package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"
	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// The recurring routines over the real store, wired as serve does: saved
// while the agent is away, they reach it when it links; with it linked, a
// save or a delete reaches it at once and the answer is its watcher's view;
// an agent from before them never gets them.
func TestRecurringRoutesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_farm', 'dev', 'Farm', 'farm', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_f6', 'dev', 'Old', 'old', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := storeroutines.New(pool)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	audit := &memAudit{}
	sync := &routinesync.Sync{Store: repo, Hub: link, Audit: audit, Log: quiet}
	link.OnConnect, link.OnEvent = sync.OnConnect, sync.OnEvent
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit, Routines: repo},
	}))
	t.Cleanup(srv.Close)
	api := srv.URL + "/api/microbot/recurring"
	// A clock-only routine: with no bridge in the test agent its clock is
	// never known, so it never starts.
	herb := `{"name":"Herb run","enabled":true,"clock":"HERB","priority":10,"run":{"className":"a.b.HerbrunPlugin","stop":["a.b.FisherPlugin"]}}`

	if status, _, _ := call(t, "GET", api, ""); status != http.StatusBadRequest {
		t.Fatalf("without ?account= = %d", status)
	}
	// Away: saved and read back from the database.
	status, body, _ := call(t, "POST", api+"?account=acc_farm", herb)
	if status != http.StatusOK || !strings.HasPrefix(body["id"].(string), "rtn_") ||
		!strings.Contains(body["result"].(map[string]any)["watchError"].(string), "no agent") {
		t.Fatalf("save while away = %d %v", status, body)
	}
	herbID := body["id"].(string)
	if last := audit.last(); last.Action != auditRecurringSet || !last.OK || last.AccountID != "acc_farm" || !strings.Contains(string(last.Params), herbID) {
		t.Fatalf("audit = %+v", last)
	}
	status, body, _ = call(t, "GET", api+"?account=acc_farm", "")
	if rs, _ := body["recurring"].([]any); status != http.StatusOK || len(rs) != 1 || rs[0].(map[string]any)["id"] != herbID {
		t.Fatalf("read while away = %d %v", status, body)
	}
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"name":"Nothing","enabled":true,"run":{"className":"a.b.HerbrunPlugin"}}`, http.StatusBadRequest},
		{`{"name":"Herb run","intervalMin":60,"run":{"className":"a.b.HerbrunPlugin"}}`, http.StatusConflict},
		{`{"id":"rtn_ffffffffffff","name":"Ghost","intervalMin":60,"run":{"className":"a.b.HerbrunPlugin"}}`, http.StatusNotFound},
		{`{"name":"Extra","intervalMin":60,"run":{"className":"a.b.HerbrunPlugin"},"shell":"rm"}`, http.StatusBadRequest},
	} {
		if status, _, _ = call(t, "POST", api+"?account=acc_farm", c.body); status != c.want {
			t.Errorf("%s = %d, want %d", c.body, status, c.want)
		}
	}
	if status, _, _ = call(t, "POST", api+"?account=acc_missing", herb); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}

	// The agent links and gets it; reads come from its watcher.
	startAgent(t, srv.URL, "", agent.Hello{Host: "zaros", Account: "acc_farm"}, "A")
	waitLinked(t, link, 1)
	watched := func(n int, ok func(map[string]any) bool) map[string]any {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			status, body, _ = call(t, "GET", api+"?account=acc_farm", "")
			if rs, _ := body["recurring"].([]any); status == http.StatusOK && body["checkedAt"] != nil && len(rs) == n && ok(body) {
				return body
			}
			if time.Now().After(deadline) {
				t.Fatalf("the agent's view never came: %d %v", status, body)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	watched(1, func(b map[string]any) bool {
		return b["recurring"].([]any)[0].(map[string]any)["reason"] == agent.ReasonNoClock
	})

	// A save while linked reaches the agent at once. An interval routine
	// that never ran is due, but the character has no position here: it
	// waits for a safe point and nothing starts.
	tree := `{"name":"Tree run","enabled":true,"intervalMin":300,"priority":50,"run":{"className":"a.b.TreerunPlugin"}}`
	status, body, _ = call(t, "POST", api+"?account=acc_farm", tree)
	if rs, _ := body["result"].(map[string]any)["recurring"].([]any); status != http.StatusOK || len(rs) != 2 || body["result"].(map[string]any)["watchError"] != nil {
		t.Fatalf("save while linked = %d %v", status, body)
	}
	treeID := body["id"].(string)
	view := watched(2, func(b map[string]any) bool {
		waiting, _ := b["waiting"].(string)
		return strings.Contains(waiting, "esperando: fora do jogo")
	})
	if first := view["recurring"].([]any)[0].(map[string]any); first["id"] != treeID || first["due"] != true || first["reason"] != agent.ReasonFirst {
		t.Fatalf("tree = %v", first)
	}

	// Delete: the agent drops it; a second delete is not found.
	if status, body, _ = call(t, "DELETE", api+"/"+treeID+"?account=acc_farm", ""); status != http.StatusOK {
		t.Fatalf("delete = %d %v", status, body)
	}
	if last := audit.last(); last.Action != auditRecurringDelete || !last.OK {
		t.Fatalf("audit = %+v", last)
	}
	watched(1, func(map[string]any) bool { return true })
	if status, _, _ = call(t, "DELETE", api+"/"+treeID+"?account=acc_farm", ""); status != http.StatusNotFound {
		t.Fatalf("delete again = %d", status)
	}

	// An agent from F6 (routines_sync, no recurring_get) never gets the
	// field it would refuse; reads come from the database.
	syncs := f6Agent(t, srv.URL, "acc_f6")
	waitLinked(t, link, 2)
	if status, body, _ = call(t, "POST", api+"?account=acc_f6", herb); status != http.StatusOK ||
		!strings.Contains(body["result"].(map[string]any)["watchError"].(string), "update chronicle") {
		t.Fatalf("save for an F6 agent = %d %v", status, body)
	}
	for _, raw := range syncs() {
		if strings.Contains(string(raw), "recurring") {
			t.Fatalf("an F6 agent got %s", raw)
		}
	}
	if status, body, _ = call(t, "GET", api+"?account=acc_f6", ""); status != http.StatusOK || len(body["recurring"].([]any)) != 1 {
		t.Fatalf("read for an F6 agent = %d %v", status, body)
	}
}

// f6Agent links a bare agent with the protocol of F6 (routines_sync, no
// recurring_get) and returns the routines_sync params it got.
func f6Agent(t *testing.T, url, account string) func() []json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, agent.LinkURL(url), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	commands := slices.DeleteFunc(slices.Clone(agent.Commands), func(c string) bool { return c == agent.CmdRecurringGet })
	hello, _ := json.Marshal(agent.Message{Type: agent.TypeHello, Hello: &agent.Hello{Host: "f6", Account: account, Commands: commands}})
	if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	var (
		mu  sync.Mutex
		got []json.RawMessage
	)
	go func() {
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m agent.Message
			if json.Unmarshal(raw, &m) != nil {
				continue
			}
			res := agent.Message{Type: agent.TypeResult, ID: m.ID, OK: true, Data: json.RawMessage(`{"triggers":[]}`)}
			if m.Command == agent.CmdRoutinesSync {
				mu.Lock()
				got = append(got, m.Params)
				mu.Unlock()
			}
			b, _ := json.Marshal(res)
			_ = conn.Write(ctx, websocket.MessageText, b)
		}
	}()
	return func() []json.RawMessage {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(got)
	}
}

// memRoutines is a RoutineStore that records the accounts it was asked for.
type memRoutines struct {
	mu       sync.Mutex
	accounts []string
}

func (m *memRoutines) seen(account string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts = append(m.accounts, account)
}

func (m *memRoutines) Triggers(_ context.Context, account string) ([]agent.Trigger, error) {
	m.seen(account)
	return []agent.Trigger{}, nil
}

func (m *memRoutines) PutTrigger(_ context.Context, account string, t agent.Trigger) ([]agent.Trigger, error) {
	m.seen(account)
	return []agent.Trigger{t}, nil
}

func (m *memRoutines) Runs(_ context.Context, account string, _ int) ([]agent.Run, error) {
	m.seen(account)
	return []agent.Run{}, nil
}

func (m *memRoutines) Recurring(_ context.Context, account string) ([]agent.Recurring, error) {
	m.seen(account)
	return []agent.Recurring{}, nil
}

func (m *memRoutines) PutRecurring(_ context.Context, account string, rc agent.Recurring) (string, []agent.Recurring, error) {
	m.seen(account)
	rc.ID = "rtn_000000000001"
	return rc.ID, []agent.Recurring{rc}, nil
}

func (m *memRoutines) DeleteRecurring(_ context.Context, account, _ string) ([]agent.Recurring, error) {
	m.seen(account)
	return []agent.Recurring{}, nil
}

// In oidc the recurring routes need a session, the Microbot role and an
// account of the session's user; the store never sees another's.
func TestRecurringRoutesOIDC(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &memRoutines{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) {
		o.Microbot = &MicrobotOptions{Link: &agent.Link{Log: quiet}, Audit: &memAudit{}, Routines: store}
	})
	herb := `{"name":"Herb run","enabled":true,"clock":"HERB","run":{"className":"a.b.HerbrunPlugin"}}`
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/microbot/recurring?account=acc-1", "", false); status != http.StatusUnauthorized {
		t.Fatalf("without a session = %d", status)
	}
	e.login(c, "/", authtest.User{Subject: "100", Username: "felipe", Roles: []string{"owner"}})
	if status, _ := e.do(c, "GET", "/api/microbot/recurring?account=acc-1", "", false); status != http.StatusForbidden {
		t.Fatalf("without the Microbot role = %d", status)
	}
	c = e.client()
	e.login(c, "/", authtest.User{Subject: "100", Username: "felipe", Roles: []string{"owner", MicrobotRole}})
	if status, _ := e.do(c, "POST", "/api/microbot/recurring?account=acc-1", herb, false); status != http.StatusForbidden {
		t.Fatalf("a save without the CSRF header = %d", status)
	}
	for _, req := range []struct{ method, path, body string }{
		{"GET", "/api/microbot/recurring?account=acc-1", ""},
		{"POST", "/api/microbot/recurring?account=acc-1", herb},
		{"DELETE", "/api/microbot/recurring/rtn_000000000001?account=acc-1", ""},
	} {
		if status, _ := e.do(c, req.method, req.path, req.body, req.method != "GET"); status != http.StatusOK {
			t.Fatalf("%s %s = %d", req.method, req.path, status)
		}
	}
	for _, acc := range []string{"acc-2", "acc-missing"} {
		for _, req := range []struct{ method, path, body string }{
			{"GET", "/api/microbot/recurring?account=" + acc, ""},
			{"POST", "/api/microbot/recurring?account=" + acc, herb},
			{"DELETE", "/api/microbot/recurring/rtn_000000000001?account=" + acc, ""},
		} {
			if status, _ := e.do(c, req.method, req.path, req.body, req.method != "GET"); status != http.StatusNotFound {
				t.Fatalf("%s %s = %d", req.method, req.path, status)
			}
		}
	}
	for _, acc := range store.accounts {
		if acc != "acc-1" {
			t.Fatalf("the store got account %q (%v)", acc, store.accounts)
		}
	}
}
