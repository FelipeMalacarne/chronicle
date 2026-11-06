package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// fakeAPI is a server with the routes the CLI reads, for one account.
type fakeAPI struct {
	mu       sync.Mutex
	accounts []client.Account
	marks    []string // "<step> <kind> <via> <note|->"
	cursor   string
	goals    []client.Goal
	removed  []string
}

func (f *fakeAPI) board() client.Board {
	steps := []coreprogress.StepState{
		{ID: "1.1.1", Ref: "1.1.1", Title: "Start as a female character", Status: coreprogress.ManualDone},
		{ID: "1.1.2", Ref: "1.1.2", Title: "Use every book and lamp on Herblore", Status: coreprogress.Pending},
		{ID: "bruh-a", Ref: "CH1.6", Title: "first of two", Status: coreprogress.Pending},
		{ID: "bruh-b", Ref: "CH1.6", Title: "second of two", Status: coreprogress.Pending},
	}
	b := client.Board{Guide: "oziris-main", Account: "acc_iron", Active: true, Board: coreprogress.Board{Steps: steps}}
	if f.cursor != "" {
		b.Cursor = &client.Cursor{StepID: f.cursor, Cause: "auto"}
	}
	return b
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	acct := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Query().Get("account") != "acc_iron" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"account not found","status":404}`))
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, client.Me{AuthMode: "dev", Accounts: f.accounts})
	})
	mux.HandleFunc("GET /api/route", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("route limit = %q", r.URL.Query().Get("limit"))
		}
		reply(w, panel.Window{
			Guide: "oziris-main", Title: "Oziris Ironman Efficiency Guide", Attribution: "Guia por OzirisLoL · ironman.guide",
			Cursor: &panel.Cursor{StepID: "1.1.2", Cause: "auto"}, Counts: coreprogress.Counts{Total: 564, Done: 5, Blocked: 13},
			Section: &panel.Section{ID: "1.1", Title: "Early quests"},
			Steps: []panel.Step{
				{ID: "1.1.2", Ref: "1.1.2", Text: "Use every book and lamp on Herblore UNTIL 77 herb", Current: true, Status: coreprogress.Pending,
					Objectives: []panel.Objective{{Key: "skill:herblore>=3", Label: "3 Herblore", Verdict: rules.Met}, {Key: "quest:the_grand_tree@finished", Label: "The Grand Tree", Verdict: rules.Unknown}},
					Notes:      []panel.Note{{Kind: "tip", Text: "HCIM only for prayer"}}},
				{ID: "1.1.5", Ref: "1.1.5", Title: "Do Druidic Ritual", Status: coreprogress.Pending,
					BlockedBy: []panel.Requirement{{Key: "skill:herblore>=3", Label: "3 Herblore"}}},
			},
			Carry: &panel.Carry{Title: "Lumbridge", Items: []panel.CarryItem{{Name: "Jug", Qty: 1, QtyStated: true, Verdict: rules.Unmet, Owned: rules.Met, Held: 2}}, BankActions: []string{"deposit everything"}},
		})
	})
	mux.HandleFunc("GET /api/progress", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		reply(w, client.Progress{Account: "acc_iron", ActiveGuide: "oziris-main", Sources: []client.Source{{Source: "plugin", Facts: 13, LastObservedAt: time.Now().Add(-4 * time.Minute)}}})
	})
	mux.HandleFunc("GET /api/guides/oziris-main/board", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		reply(w, f.board())
	})
	mux.HandleFunc("PATCH /api/guides/oziris-main/steps/{id}/mark", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		var body struct {
			Kind string  `json:"kind"`
			Note *string `json:"note"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		note := "-"
		if body.Note != nil {
			note = "[" + *body.Note + "]"
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.marks = append(f.marks, r.PathValue("id")+" "+body.Kind+" "+r.Header.Get("X-Ironman-Client")+" "+note)
		f.cursor = "bruh-a"
		reply(w, f.board())
	})
	mux.HandleFunc("GET /api/goals", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		reply(w, map[string]any{"account": "acc_iron", "goals": f.goals})
	})
	mux.HandleFunc("POST /api/goals", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key   string `json:"objective_key"`
			Label string `json:"label"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(body.Key, ":") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid goal: bad key","status":400}`))
			return
		}
		g := client.Goal{ID: "goal_new", Key: body.Key, Label: body.Label, Status: "active", Verdict: "unknown"}
		f.goals = append(f.goals, g)
		w.WriteHeader(http.StatusCreated)
		reply(w, g)
	})
	mux.HandleFunc("DELETE /api/goals/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.removed = append(f.removed, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/goals/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		lvl := 31
		reply(w, client.GoalPlanResponse{
			Account: "acc_iron", Goal: client.Goal{ID: r.PathValue("id"), Key: "quest:legends_quest@finished", Name: "Legends' Quest"},
			Plan: coreprogress.GoalPlan{
				Goal: "quest:legends_quest@finished", Requirements: true, Met: 6, Total: 20,
				Bottlenecks: []coreprogress.Bottleneck{{Skill: "agility", Level: &lvl, Target: 50, Reach: 48, ReachStep: &coreprogress.StepPlace{GuideID: "oziris-main", Ref: "1.4.12"}}},
				Nodes: []coreprogress.GoalNode{
					{Key: "quest:heroes_quest@finished", Label: "Heroes' Quest", Depth: 1, Verdict: rules.Unmet, Group: coreprogress.GroupAvailable},
					{Key: "quest:family_crest@finished", Label: "Family Crest", Depth: 1, Verdict: rules.Met, Group: coreprogress.GroupDone},
					{Key: "quest:legends_quest@finished", Label: "Legends' Quest", Depth: 0, Verdict: rules.Unmet, Group: coreprogress.GroupBlocked},
				},
				Routes: []coreprogress.StepPlace{{GuideID: "oziris-main", Ref: "1.4.47", Title: "Finish Legends' Quest"}},
			},
		})
	})
	mux.HandleFunc("GET /api/world/graph", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		if r.URL.Query().Get("depth") != "2" {
			t.Errorf("graph depth = %q", r.URL.Query().Get("depth"))
		}
		have := 31
		reply(w, map[string]any{"account": "acc_iron", "focus": r.URL.Query().Get("focus"), "label": "Legends' Quest", "kind": "quest",
			"depth": 2, "max_depth": 4, "hidden": 7, "requirements": true, "met": 6, "total": 20,
			"critical": []coreprogress.PathStep{{Key: "quest:legends_quest@finished", Label: "Legends' Quest"}, {Key: "quest:underground_pass@finished", Label: "Underground Pass"}},
			"nodes": []coreprogress.GraphNode{
				{GoalNode: coreprogress.GoalNode{Key: "quest:underground_pass@finished", Label: "Underground Pass", Depth: 1, Verdict: rules.Unmet, Group: coreprogress.GroupBlocked}, Kind: "quest", Critical: true},
				{GoalNode: coreprogress.GoalNode{Key: "skill:agility>=50", Label: "Agility", Depth: 1, Verdict: rules.Unmet, Group: coreprogress.GroupBlocked}, Kind: "skill", Have: &have},
				{GoalNode: coreprogress.GoalNode{Key: "quest:family_crest@finished", Label: "Family Crest", Depth: 1, Verdict: rules.Met, Group: coreprogress.GroupDone}, Kind: "quest"},
				{GoalNode: coreprogress.GoalNode{Key: "quest:legends_quest@finished", Label: "Legends' Quest", Depth: 0, Verdict: rules.Unmet, Group: coreprogress.GroupBlocked}, Kind: "quest", Critical: true},
			},
			"unlocks": []coreprogress.GraphLink{{Key: "quest:rfd@finished", Label: "Recipe for Disaster", Kind: "quest", Verdict: rules.Unmet, Missing: 9}}, "unlocks_total": 1,
		})
	})
	mux.HandleFunc("GET /api/account/state", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		qp := int64(57)
		reply(w, client.AccountState{
			Account: client.Account{ID: "acc_iron", RSN: "Iron Felipe", Mode: "ironman", GuideID: "oziris-main"}, TotalLevel: 701, QuestPoints: &qp,
			Skills:     []client.Skill{{Skill: "agility", Level: 31}, {Skill: "attack", Level: 40}},
			Quests:     []client.Quest{{Name: "Cook's Assistant", State: "FINISHED"}, {Name: "The Grand Tree", State: "IN_PROGRESS"}},
			Diaries:    []client.Diary{{Area: "ardougne", Tier: "easy", Complete: true}},
			Containers: []client.Container{{Container: "bank", State: "live", ObservedAt: time.Now().Add(-40 * time.Second), Items: []client.Item{{ID: 1935, Name: "Jug", Qty: 2}}}},
		})
	})
	mux.HandleFunc("POST /api/sync", func(w http.ResponseWriter, r *http.Request) {
		if !acct(w, r) {
			return
		}
		reply(w, client.SyncRun{ID: "sync_1", Source: "hiscores", Status: "ok", StepsChanged: 3})
	})
	return mux
}

func newFakeAPI(t *testing.T) (*fakeAPI, string) {
	t.Helper()
	f := &fakeAPI{accounts: []client.Account{{ID: "acc_iron", RSN: "Iron Felipe"}}, cursor: "1.1.2",
		goals: []client.Goal{{ID: "goal_1", Key: "quest:legends_quest@finished", Name: "Legends' Quest", Status: "active", Verdict: "unmet"}}}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

// runCLI runs the root command with args and returns its output.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("IRONMAN_ACCOUNT", "")
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func wantAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("output lacks %q:\n%s", w, got)
		}
	}
}

func TestCLINow(t *testing.T) {
	_, url := newFakeAPI(t)
	out, err := runCLI(t, "now", "--url", url)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out,
		"Oziris Ironman Efficiency Guide · Guia por OzirisLoL · ironman.guide",
		"5 de 564 passos feitos · 13 bloqueados",
		"1.1 · Early quests",
		"▶ 1.1.2", "Use every book and lamp",
		"Objetivos: ✓ 3 Herblore · ? The Grand Tree",
		"[tip] HCIM only for prayer",
		"Levar · Lumbridge", "✗ 1× Jug  [na mão 0 · tem 2]", "banco: deposit everything",
		"1.1.5      Do Druidic Ritual  [bloqueado: falta 3 Herblore]",
	)
	out, err = runCLI(t, "now", "--url", url, "--json")
	if err != nil || !strings.Contains(out, `"attribution": "Guia por OzirisLoL · ironman.guide"`) {
		t.Fatalf("now --json = %v\n%s", err, out)
	}
	if _, err := runCLI(t, "now", "--url", url, "--account", "acc_nobody"); !client.IsNotFound(err) {
		t.Fatalf("unknown account = %v", err)
	}
}

func TestCLIDone(t *testing.T) {
	f, url := newFakeAPI(t)
	out, err := runCLI(t, "done", "--url", url)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out, "1.1.2 feito: Use every book", "Agora: CH1.6 first of two")
	if _, err := runCLI(t, "done", "1.1.1", "--url", url, "--skip", "--note", "later"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "done", "bruh-b", "--url", url, "--undo", "--note", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"1.1.2 manual_done cli -", "1.1.1 skipped cli [later]", "bruh-b clear cli []"}
	if strings.Join(f.marks, "|") != strings.Join(want, "|") {
		t.Fatalf("marks = %q", f.marks)
	}
	if _, err := runCLI(t, "done", "CH1.6", "--url", url); err == nil || !strings.Contains(err.Error(), "names 2 steps") {
		t.Fatalf("ambiguous ref = %v", err)
	}
	if _, err := runCLI(t, "done", "9.9.9", "--url", url); err == nil || !strings.Contains(err.Error(), `no step "9.9.9"`) {
		t.Fatalf("missing step = %v", err)
	}
	if _, err := runCLI(t, "done", "--url", url, "--skip", "--pin"); err == nil {
		t.Fatal("two actions accepted")
	}
	f.cursor = ""
	if _, err := runCLI(t, "done", "--url", url); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("finished route = %v", err)
	}
}

func TestCLIAccountResolution(t *testing.T) {
	f, url := newFakeAPI(t)
	f.accounts = append(f.accounts, client.Account{ID: "acc_alt", RSN: "Alt"})
	if _, err := runCLI(t, "state", "--url", url); err == nil || !strings.Contains(err.Error(), "acc_alt (Alt)") {
		t.Fatalf("two accounts = %v", err)
	}
	if _, err := runCLI(t, "state", "--url", url, "--account", "acc_iron"); err != nil {
		t.Fatal(err)
	}
	f.accounts = nil
	if _, err := runCLI(t, "state", "--url", url); err == nil || !strings.Contains(err.Error(), "no account yet") {
		t.Fatalf("no account = %v", err)
	}
}

func TestCLIPlan(t *testing.T) {
	f, url := newFakeAPI(t)
	out, err := runCLI(t, "plan", "--url", url)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out, "✗ Legends' Quest", "quest:legends_quest@finished", "goal_1")
	out, err = runCLI(t, "plan", "legends' quest", "--url", url)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out, "6 de 20 requisitos feitos", "agility       31 → 50  · a rota chega a 48 em oziris-main 1.4.12",
		"✗ Heroes' Quest", "disponível", "oziris-main      1.4.47     Finish Legends' Quest")
	if strings.Contains(out, "Family Crest") {
		t.Fatalf("done requirement listed:\n%s", out)
	}
	if out, err = runCLI(t, "plan", "--url", url, "--add", "skill:agility>=50", "--label", "Agility"); err != nil || !strings.Contains(out, "requisitos feitos") {
		t.Fatalf("add = %v\n%s", err, out)
	}
	if _, err = runCLI(t, "plan", "--url", url, "--add", "nonsense"); err == nil || !strings.Contains(err.Error(), "invalid goal") {
		t.Fatalf("bad key = %v", err)
	}
	if out, err = runCLI(t, "plan", "--url", url, "--remove", "goal_1"); err != nil || !strings.Contains(out, "Meta removida: Legends' Quest") || f.removed[0] != "goal_1" {
		t.Fatalf("remove = %v %v\n%s", err, f.removed, out)
	}
	if _, err = runCLI(t, "plan", "nope", "--url", url); err == nil {
		t.Fatal("unknown goal accepted")
	}
}

func TestCLIGraphStateSync(t *testing.T) {
	_, url := newFakeAPI(t)
	out, err := runCLI(t, "graph", "quest:legends_quest@finished", "--url", url, "-d", "2")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out, "6 de 20 requisitos feitos · profundidade 2 de 4 · 7 mais fundos ocultos",
		"Caminho crítico: Legends' Quest ← Underground Pass",
		"▌ ✗ Underground Pass", "✗ 50 Agility", "tem 31", "✓ +1 feitos",
		"Destrava (1)", "Recipe for Disaster", "faltam 9")
	out, err = runCLI(t, "state", "--url", url, "--quests")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, out, "Iron Felipe · ironman · rota oziris-main", "Total 701 · 57 quest points",
		"agility       31", "Quests: 1 concluídas · 1 em andamento · 0 não iniciadas", "IN_PROGRESS  The Grand Tree",
		"Diaries completos (1): ardougne easy", "bank       live           1 itens · há 40 s", "plugin       há 4 min · 13 fatos")
	out, err = runCLI(t, "sync", "--url", url)
	if err != nil || !strings.Contains(out, "Hiscores sincronizados: 3 passos mudaram.") {
		t.Fatalf("sync = %v\n%s", err, out)
	}
}

func TestAccountMCP(t *testing.T) {
	f, url := newFakeAPI(t)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go func() {
		_ = newAccountMCPServer(client.New(url, "", client.ViaMCP), "").Serve(context.Background(), inR, outW)
		_ = outW.Close()
	}()
	c := &mcpClient{t: t, in: inW, out: bufio.NewScanner(outR)}
	c.out.Buffer(nil, 16<<20)
	_ = c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}})

	var list struct {
		Tools []struct{ Name string }
	}
	if err := json.Unmarshal(c.call("tools/list", map[string]any{}), &list); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "now,routes,mark_step,goals,goal_plan,remove_goal,graph,entity,account_state,sync" {
		t.Fatalf("tools = %v", names)
	}

	text := func(name string, args any) (string, bool) {
		t.Helper()
		var res struct {
			Content []struct{ Text string }
			IsError bool
		}
		if err := json.Unmarshal(c.call("tools/call", map[string]any{"name": name, "arguments": args}), &res); err != nil {
			t.Fatal(err)
		}
		return res.Content[0].Text, res.IsError
	}
	if out, isErr := text("now", map[string]any{}); isErr || !strings.Contains(out, "▶ 1.1.2") {
		t.Fatalf("now = %v\n%s", isErr, out)
	}
	var w panel.Window
	if !c.tool("now", map[string]any{"format": "json"}, &w) || w.Cursor.StepID != "1.1.2" {
		t.Fatalf("now json = %+v", w)
	}
	var routes struct {
		ActiveGuide string `json:"active_guide"`
	}
	if !c.tool("routes", map[string]any{}, &routes) || routes.ActiveGuide != "oziris-main" {
		t.Fatalf("routes = %+v", routes)
	}
	if out, isErr := text("mark_step", map[string]any{"kind": "skipped", "step": "1.1.1"}); isErr || !strings.Contains(out, "1.1.1 pulado") {
		t.Fatalf("mark = %v\n%s", isErr, out)
	}
	if len(f.marks) != 1 || f.marks[0] != "1.1.1 skipped mcp -" {
		t.Fatalf("marks = %q", f.marks)
	}
	if out, isErr := text("goal_plan", map[string]any{"goal": "goal_1"}); isErr || !strings.Contains(out, "6 de 20") {
		t.Fatalf("goal_plan = %v\n%s", isErr, out)
	}
	if out, isErr := text("graph", map[string]any{"focus": "quest:legends_quest@finished", "depth": 2}); isErr || !strings.Contains(out, "Caminho crítico") {
		t.Fatalf("graph = %v\n%s", isErr, out)
	}
	if out, isErr := text("account_state", map[string]any{"bogus": true}); !isErr || !strings.Contains(out, "unknown field") {
		t.Fatalf("unknown argument = %v\n%s", isErr, out)
	}
	if out, isErr := text("sync", map[string]any{}); isErr || !strings.Contains(out, "3 passos") {
		t.Fatalf("sync = %v\n%s", isErr, out)
	}
	_ = inW.Close()
}

func TestWrapAndClip(t *testing.T) {
	var out bytes.Buffer
	wrapLead(&out, "  [update] ", "           ", strings.Repeat("word ", 30))
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "  [update] word") || !strings.HasPrefix(lines[1], "           word") {
		t.Fatalf("wrapLead = %q", lines)
	}
	for _, l := range lines {
		if len([]rune(l)) > wrapWidth {
			t.Fatalf("line longer than %d: %q", wrapWidth, l)
		}
	}
	if got := clip("short", 10); got != "short" {
		t.Fatalf("clip short = %q", got)
	}
	if got := clip("Use every book and lamp on Herblore UNTIL 77 herb", 20); got != "Use every book and…" {
		t.Fatalf("clip long = %q", got)
	}
	if got := clip("already cut…", 20); got != "already cut…" {
		t.Fatalf("clip keeps the server's ellipsis = %q", got)
	}
}
