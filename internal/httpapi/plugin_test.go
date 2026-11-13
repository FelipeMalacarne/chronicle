package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// pluginProgress answers the panel's reads with a real board of panelGuide
// (cursor on step 2) for the accounts the fake ingest principals use.
type pluginProgress struct {
	fakeProgress
	guide   *content.Guide
	account string
	marks   map[string]coreprogress.Mark
}

func panelGuide() *content.Guide {
	g := &content.Guide{
		ID: "oziris-main", Title: "Oziris", Source: &content.Source{Author: "OzirisLoL", Origin: "ironman.guide"},
		Sections: []*content.Section{{ID: "s", Slug: "s", Title: "Early game", Steps: []*content.Step{
			{ID: "1", Text: "first"},
			{ID: "2", Text: "second", Objectives: []content.Objective{{Key: entities.MustParseObjectiveKey("skill:agility>=10"), Role: content.RoleCompletes}},
				Tasks: []content.Task{{Type: content.TaskManual, Note: "Do it by hand.", Until: &content.TaskUntil{State: content.UntilOwnerMark}}}},
			{ID: "3", Text: "third"},
		}}},
	}
	g.Index()
	return g
}

func (f *pluginProgress) ActiveGuide(_ context.Context, accountID string) (string, error) {
	f.account = accountID
	if accountID == "missing" {
		return "", storeprogress.ErrAccountNotFound
	}
	return f.guide.ID, nil
}

func (f *pluginProgress) TaskFacts(_ context.Context, accountID string) (panel.Facts, error) {
	if accountID == "missing" {
		return panel.Facts{}, storeprogress.ErrAccountNotFound
	}
	return panel.Facts{Mode: "ironman"}, nil
}

func (f *pluginProgress) Board(_ context.Context, accountID, guideID string) (storeprogress.BoardView, error) {
	f.call = "board"
	b := coreprogress.BuildBoard(coreprogress.GuideInput{Guide: f.guide, Verdicts: coreprogress.Verdicts{}, Marks: f.marks}, nil)
	return storeprogress.BoardView{Guide: guideID, Account: accountID, Active: true, Cursor: &storeprogress.Cursor{StepID: "2", Cause: "auto"}, Board: b}, nil
}

func (f *pluginProgress) Mark(_ context.Context, accountID, guideID, stepID string, action coreprogress.MarkAction, note *string) (storeprogress.BoardView, error) {
	f.action, f.note, f.step, f.account = action, note, stepID, accountID
	if guideID != f.guide.ID {
		return storeprogress.BoardView{}, storeprogress.ErrGuideNotFound
	}
	if action == coreprogress.ActionManualDone || action == coreprogress.ActionSkipped {
		f.marks[stepID] = coreprogress.Mark{Kind: coreprogress.MarkKind(action)}
	}
	return storeprogress.BoardView{}, nil
}

func (f *pluginProgress) RestoreMarks(_ context.Context, accountID, _ string, rs []storeprogress.MarkRestore) (storeprogress.BoardView, error) {
	f.call, f.account = "restore", accountID
	if len(rs) > 0 {
		f.step = rs[0].StepID
	}
	return storeprogress.BoardView{}, nil
}

func newPluginServer(t *testing.T, devMode bool) (*httptest.Server, *Server, *fakeIngestService, *pluginProgress) {
	t.Helper()
	ingest := newFakeIngestService()
	prog := &pluginProgress{guide: panelGuide(), marks: map[string]coreprogress.Mark{}}
	api := New(Options{
		Version: "test", Content: &content.Content{Guides: []*content.Guide{prog.guide}},
		Ingest: ingest, Progress: prog, DevMode: devMode, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	return server, api, ingest, prog
}

// pluginDo sends a plugin request: a token, or (empty token) the tokenless
// headers; installation "" omits the header.
func pluginDo(t *testing.T, method, url, token, installation, account, body string) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if installation != "" {
		req.Header.Set(HeaderInstallation, installation)
	}
	if account != "" {
		req.Header.Set(HeaderAccount, account)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestTokenlessIngestOnlyInDevFromLoopback(t *testing.T) {
	dev, _, ingest, _ := newPluginServer(t, true)
	if status, raw := pluginDo(t, "GET", dev.URL+"/api/ingest/manifest", "", testInstallation, "acc_main", ""); status != http.StatusOK {
		t.Fatalf("tokenless manifest from loopback in dev = %d: %s", status, raw)
	}
	push, _ := json.Marshal(fullPush(1, restoredBank(1, map[string]int64{"995": 1})))
	if status, raw := pluginDo(t, "POST", dev.URL+"/api/ingest", "", testInstallation, "", string(push)); status != http.StatusOK {
		t.Fatalf("tokenless push from loopback in dev = %d: %s", status, raw)
	}
	if got := ingest.creds; len(got) != 2 || !got[0].Tokenless() || got[0].Installation != testInstallation || got[0].Account != "acc_main" || got[1].Account != "" {
		t.Fatalf("credentials = %+v", got)
	}
	// A token still wins when present, in dev mode too, whatever the headers.
	if status, _ := pluginDo(t, "GET", dev.URL+"/api/ingest/manifest", "valid", testInstallation, "", ""); status != http.StatusOK {
		t.Fatalf("token in dev = %d", status)
	}
	if last := ingest.creds[len(ingest.creds)-1]; last.Token != "valid" || last.Installation != "" {
		t.Fatalf("token credential = %+v", last)
	}
	if status, _ := pluginDo(t, "GET", dev.URL+"/api/ingest/manifest", "revoked", testInstallation, "", ""); status != http.StatusUnauthorized {
		t.Fatalf("bad token with an installation = %d, want 401 (no tokenless fallback)", status)
	}
	if status, _ := pluginDo(t, "GET", dev.URL+"/api/ingest/manifest", "", "", "", ""); status != http.StatusBadRequest {
		t.Fatalf("tokenless without installation = %d", status)
	}
	if status, _ := pluginDo(t, "GET", dev.URL+"/api/ingest/manifest", "", "not-a-uuid", "", ""); status != http.StatusBadRequest {
		t.Fatalf("tokenless with a bad installation = %d", status)
	}

	oidc, _, _, _ := newPluginServer(t, false)
	for _, path := range []string{"/api/ingest/manifest", "/api/plugin/route"} {
		if status, _ := pluginDo(t, "GET", oidc.URL+path, "", testInstallation, "", ""); status != http.StatusUnauthorized {
			t.Fatalf("tokenless %s outside dev mode = %d", path, status)
		}
	}
	if status, _ := pluginDo(t, "POST", oidc.URL+"/api/ingest", "", testInstallation, "", string(push)); status != http.StatusUnauthorized {
		t.Fatalf("tokenless push outside dev mode = %d", status)
	}
	if status, _ := pluginDo(t, "GET", oidc.URL+"/api/ingest/manifest", "valid", "", "", ""); status != http.StatusOK {
		t.Fatalf("token outside dev mode = %d", status)
	}
}

func TestTokenlessRefusesRemotePeers(t *testing.T) {
	_, api, _, _ := newPluginServer(t, true)
	for _, tc := range []struct {
		remote string
		want   int
	}{
		{"127.0.0.1:50000", http.StatusOK},
		{"127.10.0.1:50000", http.StatusOK},
		{"[::1]:50000", http.StatusOK},
		{"192.168.1.20:50000", http.StatusUnauthorized},
		{"172.17.0.1:50000", http.StatusUnauthorized},
		{"[2001:db8::1]:50000", http.StatusUnauthorized},
		{"garbage", http.StatusUnauthorized},
	} {
		req := httptest.NewRequest("GET", "/api/ingest/manifest", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set(HeaderInstallation, testInstallation)
		// a proxy header never makes a remote peer local
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Set("X-Real-IP", "127.0.0.1")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("peer %s = %d, want %d", tc.remote, rec.Code, tc.want)
		}
	}
}

func TestPluginRouteWindow(t *testing.T) {
	server, _, _, prog := newPluginServer(t, true)
	status, raw := pluginDo(t, "GET", server.URL+"/api/plugin/route?limit=5", "", testInstallation, "", "")
	if status != http.StatusOK {
		t.Fatalf("route = %d: %s", status, raw)
	}
	var w panel.Window
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	if prog.account != "acc_dev" || w.Guide != "oziris-main" || w.Attribution != "Guia por OzirisLoL · ironman.guide" {
		t.Fatalf("window header = %+v (account %q)", w, prog.account)
	}
	if w.Cursor == nil || w.Cursor.StepID != "2" || len(w.Steps) != 2 || !w.Steps[0].Current || w.Steps[1].ID != "3" {
		t.Fatalf("window steps = %+v", w.Steps)
	}
	if o := w.Steps[0].Objectives; len(o) != 1 || o[0].Label != "10 Agility" || o[0].Verdict != "unknown" {
		t.Fatalf("objectives without a catalog = %+v", o)
	}
	// The assisted mode: the cursor step's tasks, with what ends them.
	if tasks := w.Steps[0].Tasks; len(tasks) != 1 || tasks[0].Label != "Do it by hand." || tasks[0].Until != "sua marca no passo" || tasks[0].State != "unmet" || w.Steps[1].Tasks != nil {
		t.Fatalf("tasks = %+v", w.Steps)
	}
	if status, _ := pluginDo(t, "GET", server.URL+"/api/plugin/route?limit=0", "", testInstallation, "", ""); status != http.StatusBadRequest {
		t.Fatalf("limit 0 = %d", status)
	}
	if status, _ := pluginDo(t, "GET", server.URL+"/api/plugin/route", "valid", "", "", ""); status != http.StatusOK || prog.account != "account-1" {
		t.Fatalf("route with a read-only token = %d (account %q)", status, prog.account)
	}
	if status, _ := pluginDo(t, "GET", server.URL+"/api/plugin/route", "revoked", "", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("route with a bad token = %d", status)
	}
	if status, raw := pluginDo(t, "GET", server.URL+"/api/plugin/route", "", testInstallation, "ambiguous", ""); status != http.StatusUnprocessableEntity {
		t.Fatalf("ambiguous account = %d: %s", status, raw)
	}
	if status, _ := pluginDo(t, "GET", server.URL+"/api/plugin/route", "", testInstallation, "missing", ""); status != http.StatusNotFound {
		t.Fatalf("missing account = %d", status)
	}
}

func TestPluginMarks(t *testing.T) {
	server, _, _, prog := newPluginServer(t, true)
	mark := server.URL + "/api/plugin/guides/oziris-main/steps/2/mark"
	if status, _ := pluginDo(t, "PATCH", mark, "valid", "", "", `{"kind":"manual_done"}`); status != http.StatusForbidden {
		t.Fatalf("mark with a token without marks:write = %d", status)
	}
	if prog.step != "" {
		t.Fatalf("a refused mark reached the service: %q", prog.step)
	}
	status, raw := pluginDo(t, "PATCH", mark, "marks", "", "", `{"kind":"manual_done"}`)
	if status != http.StatusOK || prog.action != coreprogress.ActionManualDone || prog.step != "2" || prog.account != "account-1" {
		t.Fatalf("mark with marks:write = %d %s (%s %s %s)", status, raw, prog.action, prog.step, prog.account)
	}
	var w panel.Window
	if err := json.Unmarshal(raw, &w); err != nil || len(w.Steps) == 0 || w.Steps[0].Status != coreprogress.ManualDone {
		t.Fatalf("mark answers with the new window: %+v %v", w.Steps, err)
	}
	if status, _ := pluginDo(t, "PATCH", server.URL+"/api/plugin/guides/oziris-main/steps/3/mark", "", testInstallation, "", `{"kind":"skipped"}`); status != http.StatusOK || prog.step != "3" || prog.account != "acc_dev" {
		t.Fatalf("tokenless mark = %d (%s %s)", status, prog.step, prog.account)
	}
	if status, _ := pluginDo(t, "PATCH", server.URL+"/api/plugin/guides/other/steps/3/mark", "", testInstallation, "", `{"kind":"clear"}`); status != http.StatusNotFound {
		t.Fatalf("unknown guide = %d", status)
	}
	if status, _ := pluginDo(t, "PATCH", mark, "", testInstallation, "", `{"kind":"manual_done","extra":1}`); status != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", status)
	}

	restore := server.URL + "/api/plugin/guides/oziris-main/marks"
	body := `{"restore":[{"step_id":"1","mark":{"kind":"implied","implied_by_step_id":"2"}}]}`
	if status, raw := pluginDo(t, "POST", restore, "", testInstallation, "", body); status != http.StatusOK || prog.step != "1" {
		t.Fatalf("restore = %d %s (%s %s)", status, raw, prog.call, prog.step)
	}
	if status, _ := pluginDo(t, "POST", restore, "", testInstallation, "", `{"before_step_id":"3"}`); status != http.StatusBadRequest {
		t.Fatalf("mark-before from the plugin = %d, want 400", status)
	}

	oidc, _, _, _ := newPluginServer(t, false)
	if status, _ := pluginDo(t, "PATCH", oidc.URL+"/api/plugin/guides/oziris-main/steps/2/mark", "", testInstallation, "", `{"kind":"manual_done"}`); status != http.StatusUnauthorized {
		t.Fatalf("tokenless mark outside dev mode = %d", status)
	}
	if status, _ := pluginDo(t, "PATCH", oidc.URL+"/api/plugin/guides/oziris-main/steps/2/mark", "marks", "", "", `{"kind":"manual_done"}`); status != http.StatusOK {
		t.Fatalf("token mark outside dev mode = %d", status)
	}
}

// GET /api/route is the same window addressed by ?account= (the CLI's
// `now`): it needs the account and checks the limit.
func TestAccountRouteWindow(t *testing.T) {
	server, _, _, prog := newPluginServer(t, true)
	resp, err := http.Get(server.URL + "/api/route?account=acc_dev&limit=5")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var w panel.Window
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("route = %d %v", resp.StatusCode, err)
	}
	if prog.account != "acc_dev" || w.Cursor == nil || w.Cursor.StepID != "2" || w.Attribution == "" {
		t.Fatalf("window = %+v (account %q)", w, prog.account)
	}
	for path, want := range map[string]int{"/api/route": http.StatusBadRequest, "/api/route?account=acc_dev&limit=0": http.StatusBadRequest} {
		r, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("%s = %d, want %d", path, r.StatusCode, want)
		}
	}
}

// The panel reads the account's facts only for a cursor step with tasks.
func TestNeedsTaskFacts(t *testing.T) {
	g := panelGuide()
	agility := entities.MustParseObjectiveKey("skill:agility>=10")
	plain := &content.Content{Guides: []*content.Guide{g}}
	withRecipe := &content.Content{Guides: []*content.Guide{g}, Recipes: map[string]*content.Recipe{agility.String(): {Objective: agility}}}
	st, _ := g.Step("2")
	st.Tasks = nil
	if needsTaskFacts(plain, g, "2") || needsTaskFacts(plain, g, "1") || needsTaskFacts(plain, g, "nope") || needsTaskFacts(nil, g, "2") {
		t.Fatal("no tasks, no facts")
	}
	if !needsTaskFacts(withRecipe, g, "2") {
		t.Fatal("a recipe of an objective the step completes has tasks")
	}
	st.Tasks = []content.Task{{Type: content.TaskManual}}
	if !needsTaskFacts(plain, g, "2") {
		t.Fatal("the step's own tasks")
	}
}
