package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// The text output is the owner's UI: PT-BR, game terms in English. unknown is
// never shown as unmet (docs/ui/decisoes-canvas.md): "?" is its own glyph.

const wrapWidth = 92

func glyph(v rules.Verdict) string {
	switch v {
	case rules.Met:
		return "✓"
	case rules.Unmet:
		return "✗"
	}
	return "?"
}

func statusLabel(s coreprogress.Status) string {
	switch s {
	case coreprogress.AutoDone:
		return "feito (detectado)"
	case coreprogress.ManualDone:
		return "feito"
	case coreprogress.Skipped:
		return "pulado"
	case coreprogress.PinnedOpen:
		return "fixado aberto"
	case coreprogress.Implied:
		return "implícito"
	}
	return "pendente"
}

// wrap breaks text into lines of at most wrapWidth runes after indent.
func wrap(out io.Writer, indent, text string) { wrapLead(out, indent, indent, text) }

// wrapLead is wrap with a different start on the first line ("  [tip] "); the
// next lines are indented to the same width.
func wrapLead(out io.Writer, lead, indent, text string) {
	first := true
	for _, para := range strings.Split(strings.TrimSpace(text), "\n") {
		start := indent
		if first {
			start = lead
		}
		line := start
		for _, word := range strings.Fields(para) {
			if len([]rune(line))+len([]rune(word)) > wrapWidth && len([]rune(line)) > len([]rune(start)) {
				fmt.Fprintln(out, strings.TrimRight(line, " "))
				line, start = indent, indent
			}
			line += word + " "
		}
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(out, strings.TrimRight(line, " "))
			first = false
		}
	}
}

// clip shortens s to n runes with an ellipsis.
func clip(s string, n int) string {
	r := []rune(strings.TrimRight(strings.TrimSuffix(s, "…"), " "))
	if len(r) <= n {
		if strings.HasSuffix(s, "…") {
			return string(r) + "…"
		}
		return string(r)
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

// ago is a compact age in PT-BR ("40 s", "4 min", "3 h", "2 d").
func ago(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d d", int(d.Hours()/24))
}

func printWindow(out io.Writer, w client.Window) {
	fmt.Fprintf(out, "%s · %s\n", w.Title, w.Attribution)
	fmt.Fprintf(out, "%d de %d passos feitos", w.Counts.Done, w.Counts.Total)
	if w.Counts.Blocked > 0 {
		fmt.Fprintf(out, " · %d bloqueados", w.Counts.Blocked)
	}
	fmt.Fprintln(out)
	if w.Cursor == nil {
		fmt.Fprintln(out, "\nRota concluída: nenhum passo pendente.")
		return
	}
	if w.Section != nil {
		fmt.Fprintf(out, "\n%s · %s\n", w.Section.ID, w.Section.Title)
	}
	var next []panel.Step
	for _, s := range w.Steps {
		if !s.Current {
			next = append(next, s)
			continue
		}
		fmt.Fprintf(out, "\n▶ %s\n", s.Ref)
		wrap(out, "  ", s.Text)
		if s.Location != "" {
			fmt.Fprintf(out, "  Local: %s\n", s.Location)
		}
		if len(s.Objectives) > 0 {
			parts := make([]string, 0, len(s.Objectives))
			for _, o := range s.Objectives {
				parts = append(parts, glyph(o.Verdict)+" "+o.Label)
			}
			fmt.Fprintf(out, "  Objetivos: %s\n", strings.Join(parts, " · "))
		}
		for _, n := range s.Notes {
			lead := "  [" + n.Kind + "] "
			wrapLead(out, lead, strings.Repeat(" ", len([]rune(lead))), n.Text)
		}
	}
	if c := w.Carry; c != nil && (len(c.Items) > 0 || len(c.BankActions) > 0) {
		title := "Levar"
		if c.Derived {
			title = "Itens dos próximos passos"
		}
		if c.Title != "" {
			title += " · " + c.Title
		}
		fmt.Fprintf(out, "\n%s\n", title)
		for _, it := range c.Items {
			qty := ""
			if it.QtyStated || it.Qty > 1 {
				qty = fmt.Sprintf("%d× ", it.Qty)
			}
			where := fmt.Sprintf("na mão %d", it.Carried)
			if it.Owned != "" {
				where += fmt.Sprintf(" · tem %d", it.Held)
			}
			opt := ""
			if it.Optional {
				opt = " (opcional)"
			}
			fmt.Fprintf(out, "  %s %s%s%s  [%s]\n", glyph(it.Verdict), qty, it.Name, opt, where)
		}
		for _, a := range c.BankActions {
			fmt.Fprintf(out, "  banco: %s\n", a)
		}
	}
	if len(next) > 0 {
		fmt.Fprintln(out, "\nPróximos")
		for _, s := range next {
			line := fmt.Sprintf("  %-10s %s", s.Ref, clip(s.Title, 64))
			if len(s.BlockedBy) > 0 {
				labels := make([]string, 0, len(s.BlockedBy))
				for _, b := range s.BlockedBy {
					labels = append(labels, b.Label)
				}
				line += "  [bloqueado: falta " + strings.Join(labels, ", ") + "]"
			} else if s.Status != coreprogress.Pending {
				line += "  [" + statusLabel(s.Status) + "]"
			}
			fmt.Fprintln(out, line)
		}
	}
}

func printMarked(out io.Writer, step stepRow, action client.MarkAction, after client.Board) {
	verb := map[client.MarkAction]string{
		client.ActionManualDone: "feito", client.ActionSkipped: "pulado",
		client.ActionPinnedOpen: "fixado aberto", client.ActionClear: "marca desfeita",
	}[action]
	fmt.Fprintf(out, "%s %s: %s\n", step.Ref, verb, clip(step.Title, 72))
	if after.Cursor == nil {
		fmt.Fprintln(out, "Rota concluída.")
		return
	}
	for _, s := range after.Steps {
		if s.ID == after.Cursor.StepID {
			fmt.Fprintf(out, "Agora: %s %s\n", s.Ref, clip(s.Title, 72))
			return
		}
	}
}

func goalName(g client.Goal) string {
	if g.Label != "" {
		return g.Label
	}
	if g.Name != "" {
		return g.Name
	}
	return g.Key
}

func printGoals(out io.Writer, goals []client.Goal) {
	if len(goals) == 0 {
		fmt.Fprintln(out, "Nenhuma meta fixada. Fixe uma com: ironman plan --add <chave do objetivo>")
		return
	}
	for _, g := range goals {
		state := ""
		if g.Status != "active" {
			state = "  [" + g.Status + "]"
		}
		fmt.Fprintf(out, "%s %-32s %-40s %s%s\n", glyph(rules.Verdict(g.Verdict)), goalName(g), g.Key, g.ID, state)
	}
}

// keyLabel names a level or quest point key by its target ("50 Agility",
// "175 quest points"); other keys keep their label.
func keyLabel(key, label string) string {
	k, err := entities.ParseObjectiveKey(key)
	if err != nil || (k.Ref.Kind != entities.KindSkill && !k.IsQuestPoints()) {
		return label
	}
	if k.IsQuestPoints() {
		return fmt.Sprintf("%d quest points", k.N)
	}
	return fmt.Sprintf("%d %s", k.N, label)
}

func place(p *coreprogress.StepPlace) string {
	if p == nil {
		return ""
	}
	return p.GuideID + " " + p.Ref
}

func groupLabel(g coreprogress.GoalGroup) string {
	switch g {
	case coreprogress.GroupDone:
		return "feito"
	case coreprogress.GroupPresumed:
		return "marcado"
	case coreprogress.GroupAvailable:
		return "disponível"
	case coreprogress.GroupBlocked:
		return "bloqueado"
	}
	return "não sabemos"
}

func printPlan(out io.Writer, r client.GoalPlanResponse) {
	p := r.Plan
	fmt.Fprintf(out, "%s · %s\n", goalName(r.Goal), r.Goal.Key)
	if !p.Requirements {
		fmt.Fprintln(out, "O catálogo não conhece requisitos para esta meta.")
	} else {
		fmt.Fprintf(out, "%d de %d requisitos feitos", p.Met, p.Total)
		if p.Presumed > 0 {
			fmt.Fprintf(out, " · %d marcados à mão", p.Presumed)
		}
		fmt.Fprintln(out)
	}
	if len(p.Bottlenecks) > 0 {
		fmt.Fprintln(out, "\nGargalos")
		for _, b := range p.Bottlenecks {
			if b.Met {
				continue
			}
			have := "?"
			if b.Level != nil {
				have = fmt.Sprint(*b.Level)
			}
			line := fmt.Sprintf("  %-12s %3s → %d", b.Skill, have, b.Target)
			switch {
			case b.Step != nil:
				line += "  · " + place(b.Step)
			case b.ReachStep != nil:
				line += fmt.Sprintf("  · a rota chega a %d em %s", b.Reach, place(b.ReachStep))
			}
			if b.Boostable {
				line += " (boost)"
			}
			fmt.Fprintln(out, line)
		}
	}
	open := 0
	for _, n := range p.Nodes {
		if n.Depth > 0 && n.Group != coreprogress.GroupDone {
			open++
		}
	}
	if open > 0 {
		fmt.Fprintln(out, "\nOrdem sugerida")
		for _, n := range p.Nodes {
			if n.Depth == 0 || n.Group == coreprogress.GroupDone {
				continue
			}
			line := fmt.Sprintf("  %s %-34s %-12s", glyph(n.Verdict), clip(keyLabel(n.Key, n.Label), 34), groupLabel(n.Group))
			if n.Step != nil {
				line += " " + place(n.Step)
			}
			fmt.Fprintln(out, strings.TrimRight(line, " "))
		}
	}
	if len(p.Routes) > 0 {
		fmt.Fprintln(out, "\nOnde cada rota faz isso")
		for _, s := range p.Routes {
			done := ""
			if s.Done {
				done = "  ✓"
			}
			fmt.Fprintf(out, "  %-16s %-10s %s%s\n", s.GuideID, s.Ref, clip(s.Title, 60), done)
		}
	}
}

func printGraph(out io.Writer, g client.Graph, all bool) {
	fmt.Fprintf(out, "%s · %s\n", g.Label, g.Focus)
	if g.Requirements {
		fmt.Fprintf(out, "%d de %d requisitos feitos · profundidade %d de %d", g.Met, g.Total, g.Depth, g.MaxDepth)
		if g.Hidden > 0 {
			fmt.Fprintf(out, " · %d mais fundos ocultos", g.Hidden)
		}
		fmt.Fprintln(out)
	} else {
		fmt.Fprintln(out, "Sem requisitos no catálogo.")
	}
	if len(g.Critical) > 1 {
		path := make([]string, 0, len(g.Critical))
		for _, s := range g.Critical {
			path = append(path, keyLabel(s.Key, s.Label))
		}
		fmt.Fprintf(out, "Caminho crítico: %s\n", strings.Join(path, " ← "))
	}
	for depth := g.Depth; depth >= 1; depth-- {
		var rows []string
		done := 0
		for _, n := range g.Nodes {
			if n.Depth != depth {
				continue
			}
			if n.Group == coreprogress.GroupDone && !all {
				done++
				continue
			}
			mark := " "
			if n.Critical {
				mark = "▌"
			}
			line := fmt.Sprintf("%s %s %-34s %s", mark, glyph(n.Verdict), clip(keyLabel(n.Key, n.Label), 34), groupLabel(n.Group))
			if n.Have != nil {
				line += fmt.Sprintf(" · tem %d", *n.Have)
			}
			if n.Step != nil {
				line += " · " + place(n.Step)
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 && done == 0 {
			continue
		}
		fmt.Fprintf(out, "\nRequer · nível %d\n", depth)
		for _, r := range rows {
			fmt.Fprintln(out, " "+r)
		}
		if done > 0 {
			fmt.Fprintf(out, "   ✓ +%d feitos\n", done)
		}
	}
	if len(g.Unlocks) > 0 {
		fmt.Fprintf(out, "\nDestrava (%d)\n", g.UnlocksTotal)
		for _, u := range g.Unlocks {
			state := "disponível"
			switch {
			case u.Verdict == rules.Met:
				state = "feito"
			case u.Missing > 0:
				state = fmt.Sprintf("faltam %d", u.Missing)
			}
			fmt.Fprintf(out, "   %s %-34s %s\n", glyph(u.Verdict), clip(u.Label, 34), state)
		}
	}
}

func printState(out io.Writer, st client.AccountState, p client.Progress, quests bool, now time.Time) {
	a := st.Account
	fmt.Fprintf(out, "%s · %s · rota %s\n", a.RSN, a.Mode, a.GuideID)
	line := fmt.Sprintf("Total %d", st.TotalLevel)
	if st.QuestPoints != nil {
		line += fmt.Sprintf(" · %d quest points", *st.QuestPoints)
	}
	fmt.Fprintln(out, line)
	if len(st.Skills) > 0 {
		fmt.Fprintln(out, "\nSkills")
		for i := 0; i < len(st.Skills); i += 3 {
			var cells []string
			for _, s := range st.Skills[i:min(i+3, len(st.Skills))] {
				cells = append(cells, fmt.Sprintf("%-13s %2d", s.Skill, s.Level))
			}
			fmt.Fprintln(out, "  "+strings.Join(cells, "    "))
		}
	}
	byState := map[string]int{}
	for _, q := range st.Quests {
		byState[q.State]++
	}
	if len(st.Quests) > 0 {
		fmt.Fprintf(out, "\nQuests: %d concluídas · %d em andamento · %d não iniciadas\n",
			byState["FINISHED"], byState["IN_PROGRESS"], byState["NOT_STARTED"])
		if quests {
			for _, q := range st.Quests {
				fmt.Fprintf(out, "  %-12s %s\n", q.State, q.Name)
			}
		}
	}
	var diaries []string
	for _, d := range st.Diaries {
		if d.Complete {
			diaries = append(diaries, d.Area+" "+d.Tier)
		}
	}
	if len(st.Diaries) > 0 {
		fmt.Fprintf(out, "\nDiaries completos (%d): %s\n", len(diaries), strings.Join(diaries, ", "))
	}
	if len(st.Containers) > 0 {
		fmt.Fprintln(out, "\nContainers")
		for _, c := range st.Containers {
			fmt.Fprintf(out, "  %-10s %-12s %3d itens · há %s\n", c.Container, c.State, len(c.Items), ago(c.ObservedAt, now))
		}
	}
	if len(p.Sources) > 0 {
		srcs := append([]client.Source(nil), p.Sources...)
		sort.Slice(srcs, func(i, j int) bool { return srcs[i].Source < srcs[j].Source })
		fmt.Fprintln(out, "\nFontes")
		for _, s := range srcs {
			fmt.Fprintf(out, "  %-12s há %s · %d fatos\n", s.Source, ago(s.LastObservedAt, now), s.Facts)
		}
	}
}

func printSync(out io.Writer, run client.SyncRun, now time.Time) {
	switch run.Status {
	case "ok":
		fmt.Fprintf(out, "Hiscores sincronizados: %d passos mudaram.\n", run.StepsChanged)
	case "unchanged":
		fmt.Fprintln(out, "Hiscores sem mudança.")
	default:
		fmt.Fprintf(out, "Sync %s", run.Status)
		if run.Error != "" {
			fmt.Fprintf(out, ": %s", run.Error)
		}
		fmt.Fprintln(out)
	}
	if run.NextAllowedAt != nil && run.NextAllowedAt.After(now) {
		fmt.Fprintf(out, "Próximo sync permitido em %s.\n", run.NextAllowedAt.Sub(now).Round(time.Second))
	}
}
