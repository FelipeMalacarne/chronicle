package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// fakeProgress records the last call and answers with err when set.
type fakeProgress struct {
	err    error
	call   string
	action coreprogress.MarkAction
	note   *string
	step   string
}

func (f *fakeProgress) board(call, accountID, guideID string) (storeprogress.BoardView, error) {
	f.call = call
	if f.err != nil {
		return storeprogress.BoardView{}, f.err
	}
	if accountID != "acc-1" {
		return storeprogress.BoardView{}, storeprogress.ErrAccountNotFound
	}
	return storeprogress.BoardView{Guide: guideID, Account: accountID, Orphans: []storeprogress.Orphan{}}, nil
}

func (f *fakeProgress) Board(_ context.Context, accountID, guideID string) (storeprogress.BoardView, error) {
	return f.board("board", accountID, guideID)
}

func (f *fakeProgress) ActiveGuide(_ context.Context, accountID string) (string, error) {
	if accountID != "acc-1" {
		return "", storeprogress.ErrAccountNotFound
	}
	return "oziris-main", nil
}

func (f *fakeProgress) Mark(_ context.Context, accountID, guideID, stepID string, action coreprogress.MarkAction, note *string) (storeprogress.BoardView, error) {
	f.action, f.note, f.step = action, note, stepID
	return f.board("mark", accountID, guideID)
}

func (f *fakeProgress) ConfirmImplied(_ context.Context, accountID, guideID string) (storeprogress.BoardView, error) {
	return f.board("confirm", accountID, guideID)
}

func (f *fakeProgress) PinCursor(_ context.Context, accountID, guideID, stepID string) (storeprogress.BoardView, error) {
	f.step = stepID
	return f.board("pin", accountID, guideID)
}

func (f *fakeProgress) MarkBefore(_ context.Context, accountID, guideID, stepID string) (storeprogress.BoardView, error) {
	f.step = stepID
	b, err := f.board("before", accountID, guideID)
	b.Marked = []string{"0", "1"}
	return b, err
}

func (f *fakeProgress) RestoreMarks(_ context.Context, accountID, guideID string, rs []storeprogress.MarkRestore) (storeprogress.BoardView, error) {
	var ids []string
	for _, r := range rs {
		id := r.StepID
		if r.Mark != nil {
			id += "<" + string(r.Mark.Kind)
		}
		ids = append(ids, id)
	}
	f.step = strings.Join(ids, ",")
	return f.board("restore", accountID, guideID)
}

func (f *fakeProgress) Objectives(_ context.Context, accountID string) ([]storeprogress.ObjectiveView, error) {
	f.call = "objectives"
	if accountID != "acc-1" {
		return nil, storeprogress.ErrAccountNotFound
	}
	return []storeprogress.ObjectiveView{{Key: "skill:agility>=10", Verdict: "met"}, {Key: "quest:monks_friend@finished", Verdict: "met"}}, nil
}

func (f *fakeProgress) Events(_ context.Context, accountID, guideID string, limit int) (storeprogress.EventsView, error) {
	f.call = "events"
	if accountID != "acc-1" {
		return storeprogress.EventsView{}, storeprogress.ErrAccountNotFound
	}
	return storeprogress.EventsView{Guide: guideID, Account: accountID, Steps: make([]storeprogress.StepEventView, limit), Objectives: []storeprogress.ObjectiveEventView{}}, nil
}

func (f *fakeProgress) Sources(_ context.Context, accountID string) ([]storeprogress.SourceView, error) {
	if accountID != "acc-1" {
		return nil, storeprogress.ErrAccountNotFound
	}
	return []storeprogress.SourceView{{Source: "plugin", Facts: 3}}, nil
}

func (f *fakeProgress) AccountState(_ context.Context, accountID string) (storeprogress.AccountState, error) {
	f.call = "state"
	if accountID != "acc-1" {
		return storeprogress.AccountState{}, storeprogress.ErrAccountNotFound
	}
	return storeprogress.AccountState{
		Account:    storeprogress.AccountView{ID: "acc-1", RSN: "Iron One", Mode: "ironman", GuideID: "oziris-main", Layer: "enhanced"},
		TotalLevel: 701,
		Skills:     []storeprogress.SkillView{{Skill: "magic", Level: 41, XP: 41000, FactSource: storeprogress.FactSource{Source: "hiscores"}}},
		Quests:     []storeprogress.QuestView{},
		Diaries:    []storeprogress.DiaryView{},
		Containers: []storeprogress.ContainerView{},
	}, nil
}

func (f *fakeProgress) SetActiveGuide(_ context.Context, accountID, guideID string) error {
	f.call, f.step = "active", guideID
	if guideID != "oziris-main" {
		return storeprogress.ErrGuideNotFound
	}
	return nil
}

func (f *fakeProgress) Goals(_ context.Context, accountID string) ([]storeprogress.GoalView, error) {
	if accountID != "acc-1" {
		return nil, storeprogress.ErrAccountNotFound
	}
	return []storeprogress.GoalView{{ID: "goal_1", Key: "quest:monks_friend@finished", Status: "active", Verdict: "unknown"}}, nil
}

func (f *fakeProgress) AddGoal(_ context.Context, _, key, label string) (storeprogress.GoalView, error) {
	f.call, f.step = "addGoal", key
	if !strings.Contains(key, "@") && !strings.Contains(key, ">=") {
		return storeprogress.GoalView{}, storeprogress.ErrInvalidGoal
	}
	return storeprogress.GoalView{ID: "goal_2", Key: key, Label: label, Status: "active", Verdict: "unknown"}, nil
}

func (f *fakeProgress) Check(_ context.Context, accountID string) (storeprogress.CheckReport, error) {
	f.call = "check"
	return storeprogress.CheckReport{Account: accountID, OK: true, Differences: []string{}}, f.err
}

func (f *fakeProgress) GoalPlan(ctx context.Context, accountID, goalID string) (storeprogress.GoalPlanView, error) {
	goals, err := f.Goals(ctx, accountID)
	if err != nil {
		return storeprogress.GoalPlanView{}, err
	}
	if goalID != "goal_1" {
		return storeprogress.GoalPlanView{}, storeprogress.ErrGoalNotFound
	}
	return storeprogress.GoalPlanView{Goal: goals[0], Plan: coreprogress.GoalPlan{Goal: goals[0].Key, Requirements: true, Total: 1}}, nil
}

func (f *fakeProgress) Graph(_ context.Context, accountID, focus string, depth int) (coreprogress.Graph, error) {
	f.call, f.step = "graph", focus
	if accountID != "acc-1" {
		return coreprogress.Graph{}, storeprogress.ErrAccountNotFound
	}
	if !strings.Contains(focus, ":") {
		return coreprogress.Graph{}, storeprogress.ErrInvalidFocus
	}
	return coreprogress.Graph{Focus: focus, Depth: depth, Nodes: []coreprogress.GraphNode{}, Critical: []coreprogress.PathStep{}}, nil
}

func (f *fakeProgress) RemoveGoal(_ context.Context, _, goalID string) error {
	f.call, f.step = "removeGoal", goalID
	if goalID != "goal_1" {
		return storeprogress.ErrGoalNotFound
	}
	return nil
}

func newProgressServer(t *testing.T, devMode bool, fake *fakeProgress) *httptest.Server {
	t.Helper()
	g := &content.Guide{ID: "oziris-main", Title: "Oziris", Sections: []*content.Section{{ID: "s", Slug: "s", Steps: []*content.Step{{ID: "1", Text: "x"}}}}}
	g.Index()
	server := httptest.NewServer(New(Options{
		Version: "test", Content: &content.Content{Guides: []*content.Guide{g}}, Progress: fake, DevMode: devMode,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(server.Close)
	return server
}

func do(t *testing.T, method, url, body string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestProgressRoutesAreDevOnly(t *testing.T) {
	server := newProgressServer(t, false, &fakeProgress{})
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/guides/oziris-main/board?account=acc-1", ""},
		{"PATCH", "/api/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"manual_done"}`},
		{"POST", "/api/guides/oziris-main/implied/confirm?account=acc-1", ""},
		{"PUT", "/api/guides/oziris-main/cursor?account=acc-1", `{"step_id":"1"}`},
		{"GET", "/api/objectives?account=acc-1", ""},
		{"GET", "/api/guides/oziris-main/events?account=acc-1", ""},
		{"GET", "/api/progress?account=acc-1", ""},
		{"PUT", "/api/account/guide?account=acc-1", `{"guide_id":"oziris-main"}`},
		{"GET", "/api/account/state?account=acc-1", ""},
		{"GET", "/api/world/entity?account=acc-1&ref=quest:the_grand_tree", ""},
	} {
		if status, _ := do(t, r.method, server.URL+r.path, r.body); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s outside dev mode = %d", r.method, r.path, status)
		}
	}
}

func TestProgressRoutes(t *testing.T) {
	fake := &fakeProgress{}
	server := newProgressServer(t, true, fake)
	base := server.URL + "/api"

	if status, body := do(t, "GET", base+"/guides/oziris-main/board?account=acc-1", ""); status != http.StatusOK || body["guide"] != "oziris-main" || fake.call != "board" {
		t.Fatalf("board = %d %v", status, body)
	}
	if status, _ := do(t, "GET", base+"/guides/oziris-main/board", ""); status != http.StatusBadRequest {
		t.Fatalf("board without account = %d", status)
	}
	if status, _ := do(t, "GET", base+"/guides/oziris-main/board?account=nobody", ""); status != http.StatusNotFound {
		t.Fatalf("board of an unknown account = %d", status)
	}
	if status, body := do(t, "GET", base+"/account/state?account=acc-1", ""); status != http.StatusOK || fake.call != "state" || body["total_level"] != float64(701) || body["account"].(map[string]any)["rsn"] != "Iron One" {
		t.Fatalf("account state = %d %v", status, body)
	}
	if status, _ := do(t, "GET", base+"/account/state?account=nobody", ""); status != http.StatusNotFound {
		t.Fatalf("account state of an unknown account = %d", status)
	}

	if status, _ := do(t, "PATCH", base+"/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"skipped","note":"later"}`); status != http.StatusOK || fake.action != coreprogress.ActionSkipped || fake.note == nil || *fake.note != "later" || fake.step != "1" {
		t.Fatalf("mark = %d action=%s note=%v", status, fake.action, fake.note)
	}
	if status, _ := do(t, "PATCH", base+"/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"clear"}`); status != http.StatusOK || fake.note != nil {
		t.Fatalf("mark without note = %d note=%v", status, fake.note)
	}
	if status, _ := do(t, "PATCH", base+"/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"done","extra":1}`); status != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", status)
	}
	if status, _ := do(t, "PATCH", base+"/guides/oziris-main/steps/1/mark?account=acc-1", ""); status != http.StatusBadRequest {
		t.Fatalf("empty body = %d", status)
	}

	if status, _ := do(t, "POST", base+"/guides/oziris-main/implied/confirm?account=acc-1", ""); status != http.StatusOK || fake.call != "confirm" {
		t.Fatalf("confirm = %d", status)
	}
	if status, _ := do(t, "PUT", base+"/guides/oziris-main/cursor?account=acc-1", `{"step_id":"1"}`); status != http.StatusOK || fake.step != "1" {
		t.Fatalf("pin = %d step=%q", status, fake.step)
	}
	if status, _ := do(t, "PUT", base+"/guides/oziris-main/cursor?account=acc-1", `{"step_id":null}`); status != http.StatusOK || fake.step != "" {
		t.Fatalf("unpin = %d step=%q", status, fake.step)
	}
	if status, _ := do(t, "PUT", base+"/guides/oziris-main/cursor?account=acc-1", `{"step_id":" "}`); status != http.StatusBadRequest {
		t.Fatalf("blank step = %d", status)
	}
	if status, body := do(t, "GET", base+"/objectives?account=acc-1", ""); status != http.StatusOK || len(body["objectives"].([]any)) != 2 {
		t.Fatalf("objectives = %d %v", status, body)
	}

	if status, body := do(t, "POST", base+"/guides/oziris-main/marks?account=acc-1", `{"before_step_id":"1"}`); status != http.StatusOK || fake.call != "before" || len(body["marked"].([]any)) != 2 {
		t.Fatalf("mark before = %d %v", status, body)
	}
	if status, _ := do(t, "POST", base+"/guides/oziris-main/marks?account=acc-1", `{"restore":[{"step_id":"0","mark":null},{"step_id":"1","mark":{"kind":"implied","implied_by_step_id":"2"}}]}`); status != http.StatusOK || fake.call != "restore" || fake.step != "0,1<implied" {
		t.Fatalf("restore = %d %s", status, fake.step)
	}
	if status, _ := do(t, "POST", base+"/guides/oziris-main/marks?account=acc-1", `{"before_step_id":"1","restore":[{"step_id":"0"}]}`); status != http.StatusBadRequest {
		t.Fatalf("both = %d", status)
	}
	if status, body := do(t, "GET", base+"/progress?account=acc-1", ""); status != http.StatusOK || len(body["routes"].([]any)) != 1 {
		t.Fatalf("progress = %d %v", status, body)
	}
	if status, body := do(t, "GET", base+"/guides/oziris-main/events?account=acc-1&limit=3", ""); status != http.StatusOK || len(body["steps"].([]any)) != 3 {
		t.Fatalf("events = %d %v", status, body)
	}
	if status, _ := do(t, "GET", base+"/guides/oziris-main/events?account=acc-1&limit=0", ""); status != http.StatusBadRequest {
		t.Fatalf("events limit 0 = %d", status)
	}
	if status, _ := do(t, "PUT", base+"/account/guide?account=acc-1", `{"guide_id":"oziris-main"}`); status != http.StatusOK || fake.call != "board" {
		t.Fatalf("switch route = %d last call %s", status, fake.call)
	}
	if status, _ := do(t, "PUT", base+"/account/guide?account=acc-1", `{"guide_id":"nope"}`); status != http.StatusNotFound {
		t.Fatalf("switch to unknown route = %d", status)
	}

	for _, tc := range []struct {
		err  error
		want int
	}{
		{storeprogress.ErrGuideNotFound, http.StatusNotFound},
		{coreprogress.ErrStepNotFound, http.StatusNotFound},
		{coreprogress.ErrInvalidAction, http.StatusBadRequest},
		{coreprogress.ErrNotImplied, http.StatusConflict},
		{coreprogress.ErrStepDone, http.StatusConflict},
		{io.ErrUnexpectedEOF, http.StatusInternalServerError},
	} {
		fake.err = tc.err
		if status, _ := do(t, "PATCH", base+"/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"manual_done"}`); status != tc.want {
			t.Fatalf("%v = %d, want %d", tc.err, status, tc.want)
		}
	}
}
