// Package panel assembles the route window of the plugin panel (plugin
// 1.2.0, docs/01 decision 27): the cursor step and the next steps of the
// active route, with verdicts, labels, blocked requirements and the items to
// carry for the cursor's leg. It is pure: the board, the holdings and the
// catalog come in, a JSON-ready window comes out. The plugin has no catalog,
// so every display name is resolved here; it never decides a verdict.
package panel

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/planning"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// DefaultLimit and MaxLimit bound how many steps follow the cursor.
const (
	DefaultLimit = 15
	MaxLimit     = 50
)

// Window is the plugin panel's view of the account's active route.
type Window struct {
	Guide string `json:"guide"`
	Title string `json:"title"`
	// Attribution is the line every screen with guide steps shows (CLAUDE.md
	// rule 6).
	Attribution string `json:"attribution"`
	// Cursor is null when the route is finished.
	Cursor *Cursor         `json:"cursor"`
	Counts progress.Counts `json:"counts"`
	// Steps is the cursor step (Current) and the visible main-route steps
	// after it, in route order.
	Steps []Step `json:"steps"`
	// Carry is the cursor's leg: what to bring and the bank actions. Null
	// without a cursor.
	Carry *Carry `json:"carry"`
	// Section is the cursor's section with the author's description. Null
	// without a cursor.
	Section *Section `json:"section,omitempty"`
}

// Section is the cursor's section.
type Section struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// Note is one of the author's notes on a step (tip, update, hcim).
type Note struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Cursor is the route's current step.
type Cursor struct {
	StepID string `json:"step_id"`
	Cause  string `json:"cause"`
}

// Step is one step of the window.
type Step struct {
	ID           string          `json:"id"`
	Ref          string          `json:"ref"`
	SectionID    string          `json:"section_id"`
	SectionTitle string          `json:"section_title,omitempty"`
	Title        string          `json:"title"`
	Text         string          `json:"text"`
	Location     string          `json:"location,omitempty"`
	Status       progress.Status `json:"status"`
	Mark         *progress.Mark  `json:"mark,omitempty"`
	Current      bool            `json:"current"`
	// BlockedBy are the unmet requirements that make the cursor pass this
	// pending step ("falta …").
	BlockedBy  []Requirement `json:"blocked_by"`
	Objectives []Objective   `json:"objectives"`
	// Notes are the author's notes, without the BRUHsailer metadata lines
	// (GP after step, total time, items needed, skills/quests). An update
	// note can tell the player to skip the step.
	Notes []Note `json:"notes,omitempty"`
	// Tasks are the cursor step's typed tasks (docs/10 section 7: the
	// recipes' and the step's own), the checklist of the assisted mode; only
	// on the current step, absent when it has none.
	Tasks []Task `json:"tasks,omitempty"`
}

// Requirement is an objective key with its display label.
type Requirement struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Objective is one objective of a step with the account's verdict.
type Objective struct {
	Key       string        `json:"key"`
	Role      string        `json:"role"`
	Label     string        `json:"label"`
	Verdict   rules.Verdict `json:"verdict"`
	Regressed bool          `json:"regressed,omitempty"`
	// OwnerMarked: the owner marked it as already done (no evidence).
	OwnerMarked bool `json:"owner_marked,omitempty"`
}

// Carry is the readiness of the cursor's leg, flattened for the panel.
type Carry struct {
	SegmentID     string          `json:"segment_id"`
	Title         string          `json:"title,omitempty"`
	Status        planning.Status `json:"status"`
	LoadoutStatus string          `json:"loadout_status"`
	// Derived: the guide lists no setup for the leg, so Items is the
	// preview of what its next steps use (planning.LookAhead), never a
	// loadout.
	Derived     bool        `json:"derived"`
	Items       []CarryItem `json:"items"`
	BankActions []string    `json:"bank_actions"`
}

// CarryItem is one item to bring.
type CarryItem struct {
	Name      string `json:"name"`
	Qty       int64  `json:"qty"`
	QtyStated bool   `json:"qty_stated"`
	Slot      string `json:"slot"`
	Optional  bool   `json:"optional,omitempty"`
	// Verdict is about the live inventory/equipment (carried).
	Verdict rules.Verdict `json:"verdict"`
	Carried int64         `json:"carried"`
	// Owned is ownership anywhere (bank included); empty when unknown to
	// the evaluation.
	Owned rules.Verdict `json:"owned,omitempty"`
	Held  int64         `json:"held,omitempty"`
}

// Input is what Build reads.
type Input struct {
	Guide    *content.Guide
	Board    progress.Board
	Cursor   progress.CursorRecord
	Holdings rules.ItemHoldings
	Registry *entities.Registry
	// Limit is how many steps follow the cursor; out of range means
	// DefaultLimit.
	Limit int
	// Content gives the cursor step's tasks with its recipes; nil leaves
	// the checklist out. Facts and Now evaluate their termination.
	Content *content.Content
	Facts   Facts
	Now     time.Time
}

// Build assembles the window.
func Build(in Input) Window {
	g := in.Guide
	out := Window{Guide: g.ID, Title: g.Title, Attribution: Attribution(g), Counts: in.Board.Counts, Steps: []Step{}}
	limit := in.Limit
	if limit < 1 || limit > MaxLimit {
		limit = DefaultLimit
	}
	if in.Cursor.StepID == "" {
		return out
	}
	out.Cursor = &Cursor{StepID: in.Cursor.StepID, Cause: in.Cursor.Cause}
	titles := map[string]string{}
	for _, sec := range g.Sections {
		titles[sec.ID] = sec.Title
	}
	start := -1
	for i, s := range in.Board.Steps {
		if s.ID == in.Cursor.StepID {
			start = i
			break
		}
	}
	if start < 0 {
		return out
	}
	for i, s := range in.Board.Steps[start:] {
		current := i == 0
		if !current && (!s.Main || !s.Visible) {
			continue
		}
		if !current && len(out.Steps) > limit {
			// the cursor step plus limit steps after it
			break
		}
		st, ok := g.Step(s.ID)
		if !ok {
			continue
		}
		out.Steps = append(out.Steps, step(st, s, titles[st.SectionID], current, in.Registry))
	}
	if cur, ok := g.Step(in.Cursor.StepID); ok {
		if sec, ok := g.Section(cur.SectionID); ok {
			out.Section = &Section{ID: sec.ID, Title: sec.Title, Description: strings.TrimSpace(sec.Description)}
		}
	}
	if current, _ := planning.Legs(g, in.Cursor.StepID); current != nil {
		out.Carry = carry(g, current, in.Cursor.StepID, in.Holdings, in.Registry)
	}
	if in.Content != nil && len(out.Steps) > 0 && out.Steps[0].Current {
		if st, ok := g.Step(in.Cursor.StepID); ok {
			now := in.Now
			if now.IsZero() {
				now = time.Now()
			}
			// bank tasks without items get ready for the step's own segment
			seg, _ := g.Segment(st.SegmentID)
			if tasks := stepTasks(in.Content, st, in.Board.Steps[start], seg, g, in.Holdings, in.Facts, in.Registry, now); len(tasks) > 0 {
				out.Steps[0].Tasks = tasks
			}
		}
	}
	return out
}

func step(st *content.Step, s progress.StepState, sectionTitle string, current bool, reg *entities.Registry) Step {
	out := Step{
		ID: st.ID, Ref: s.Ref, SectionID: st.SectionID, SectionTitle: sectionTitle, Title: s.Title,
		Text: strings.TrimSpace(st.Text), Location: location(st, reg), Status: s.Status, Mark: s.Mark, Current: current,
		BlockedBy: make([]Requirement, 0, len(s.BlockedBy)), Objectives: make([]Objective, 0, len(s.Objectives)),
	}
	for _, key := range s.BlockedBy {
		out.BlockedBy = append(out.BlockedBy, Requirement{Key: key, Label: Label(key, reg)})
	}
	for _, o := range s.Objectives {
		out.Objectives = append(out.Objectives, Objective{Key: o.Key, Role: o.Role, Label: Label(o.Key, reg), Verdict: o.Verdict, Regressed: o.Regressed, OwnerMarked: o.OwnerMarked})
	}
	for _, n := range st.Notes {
		if text := strings.TrimSpace(n.Text); text != "" && !metaNote(text) {
			out.Notes = append(out.Notes, Note{Kind: n.Kind, Text: text})
		}
	}
	return out
}

// metaPrefixes are the BRUHsailer metadata lines the importer keeps as notes;
// the web shows them as chips (web/src/lib/progress/view.ts META), not as notes.
var metaPrefixes = []string{"gp after step:", "total time:", "items needed:", "skills / quests:"}

func metaNote(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range metaPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func location(st *content.Step, reg *entities.Registry) string {
	if st.LocationRef != nil {
		if name, ok := display(reg, *st.LocationRef); ok {
			return name
		}
	}
	return strings.TrimSpace(st.Location)
}

func carry(g *content.Guide, seg *content.Segment, cursor string, h rules.ItemHoldings, reg *entities.Registry) *Carry {
	ev := planning.EvaluateSegment(g, seg, h, reg)
	out := &Carry{
		SegmentID: ev.SegmentID, Title: ev.Title, Status: ev.Status, LoadoutStatus: ev.LoadoutStatus,
		Items: []CarryItem{}, BankActions: []string{},
	}
	for _, a := range ev.BankActions {
		if text := strings.TrimSpace(a.Text); text != "" {
			out.BankActions = append(out.BankActions, text)
		}
	}
	items := ev.Items
	if ev.LoadoutStatus == content.LoadoutUnknown {
		if ahead := planning.LookAhead(seg, cursor, h, reg); ahead != nil {
			out.Derived = true
			for _, grp := range ahead.Groups {
				for _, it := range grp.Items {
					items = append(items, it.Item)
				}
			}
		}
	}
	for _, it := range items {
		ci := CarryItem{
			Name: it.Name, Qty: it.Required, QtyStated: it.QtyStated, Slot: it.Slot, Optional: it.Optional,
			Verdict: it.Verdict, Carried: it.Carried,
		}
		if it.Owned != nil {
			ci.Owned, ci.Held = it.Owned.Verdict, it.Owned.Held
		}
		out.Items = append(out.Items, ci)
	}
	return out
}

// Attribution is the credit line of a guide in the panel's language (PT-BR):
// "Guia por OzirisLoL · ironman.guide", "Guia por So Iron BRUH e
// ParasailerOSRS" (CLAUDE.md rule 6).
func Attribution(g *content.Guide) string {
	if g.Source == nil || strings.TrimSpace(g.Source.Author) == "" {
		return g.Title
	}
	line := "Guia por " + strings.ReplaceAll(strings.TrimSpace(g.Source.Author), " and ", " e ")
	if origin := strings.TrimSpace(g.Source.Origin); strings.Contains(origin, ".") {
		line += " · " + origin
	}
	return line
}

// Label is the display name of an objective's target, as the web shows it
// (web/src/lib/progress/view.ts, objectiveLabel): "35 Woodcutting", "Monk's
// Friend", "120 Feather", "107 quest points", "Graceful hood (qualquer
// variante)", "2 Mithril axe ou Adamant axe".
func Label(key string, reg *entities.Registry) string {
	k, err := entities.ParseObjectiveKey(key)
	if err != nil {
		return key
	}
	if k.IsQuestPoints() {
		return strconv.Itoa(k.N) + " quest points"
	}
	if slug, n, ok := k.ItemFamily(); ok {
		name, known := display(reg, k.Ref)
		if !known {
			name = sentenceCase(slug)
		}
		return counted(n, name) + " (qualquer variante)"
	}
	if alts, ok := k.ItemAlternatives(); ok {
		parts := make([]string, len(alts))
		for i, a := range alts {
			name, known := display(reg, entities.ItemRef(a.ID))
			if !known {
				name = "item " + strconv.Itoa(a.ID)
			}
			parts[i] = counted(a.N, name)
		}
		return strings.Join(parts, " ou ")
	}
	name, known := display(reg, k.Ref)
	if !known {
		name = titleCase(k.Ref.ID)
	}
	switch k.Ref.Kind {
	case entities.KindSkill, entities.KindBoss:
		return strconv.Itoa(k.N) + " " + name
	case entities.KindItem:
		return counted(k.N, name)
	default:
		return name
	}
}

func display(reg *entities.Registry, ref entities.Ref) (string, bool) {
	if reg == nil {
		return "", false
	}
	return reg.Display(ref)
}

func counted(n int, name string) string {
	if n > 1 {
		return strconv.Itoa(n) + " " + name
	}
	return name
}

// titleCase turns an id into words: "x_marks_the_spot" -> "X Marks The Spot".
func titleCase(id string) string {
	words := strings.Fields(strings.ReplaceAll(id, "_", " "))
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// sentenceCase turns a family slug into the catalog's spelling:
// "graceful_hood" -> "Graceful hood".
func sentenceCase(slug string) string {
	s := strings.ReplaceAll(slug, "_", " ")
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
