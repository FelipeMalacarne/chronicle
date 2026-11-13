package workerctl

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
)

const oldImage = "ghcr.io/felipemalacarne/chronicle-worker:old"

type memAudit struct {
	mu      sync.Mutex
	entries []storemicrobot.Entry
}

func (a *memAudit) Append(_ context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return e, nil
}

func (a *memAudit) all() []storemicrobot.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.entries)
}

// managed is a managed worker's agent; an old agent declares no run_status.
func managed(id, account string, commands ...string) agent.Info {
	if commands == nil {
		commands = []string{agent.CmdStatus, agent.CmdRunStatus}
	}
	return agent.Info{ID: id, Account: account, Kind: agent.KindManaged, Commands: commands}
}

func game(loggedIn bool, brk *agent.BreakState) agent.Status {
	state := `{"loggedIn":false,"gameState":"LOGIN_SCREEN"}`
	if loggedIn {
		state = `{"loggedIn":true,"gameState":"LOGGED_IN"}`
	}
	return agent.Status{Running: true, State: json.RawMessage(state), Break: brk}
}

// onOld is the Deployment of a worker that already runs the old image.
func onOld(account string) kube.Deployment {
	return kube.WorkerDeployment("chronicle", kube.WorkerSpec{Account: account, Image: oldImage})
}

func deployedImage(cl *fakeCluster, account string) (string, string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	d, ok := cl.deps[kube.WorkerName(account)]
	if !ok {
		return "", ""
	}
	return d.Spec.Template.Spec.Containers[0].Image, d.Spec.Template.Metadata.Annotations[kube.AnnotationRestartedAt]
}

// An outdated worker restarts on the new image by itself only at a pause:
// logged out, or on a Break Handler break, with no run. It is audited like
// the owner's Reiniciar, by the reconciler, and happens once.
func TestAutoRestartAtPause(t *testing.T) {
	store := &fakeStore{desired: []Desired{
		{Account: "acc_out", Running: true, Image: oldImage},
		{Account: "acc_game", Running: true, Image: oldImage},
		{Account: "acc_brk", Running: true, Image: oldImage},
		{Account: "acc_old", Running: true, Image: oldImage},
		{Account: "acc_off", Running: false, Image: oldImage},
	}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{}}
	for _, a := range []string{"acc_out", "acc_game", "acc_brk", "acc_old"} {
		cl.deps[kube.WorkerName(a)] = onOld(a)
	}
	hub := &fakeHub{
		agents: []agent.Info{
			managed("c-out", "acc_out"), managed("c-game", "acc_game"), managed("c-brk", "acc_brk"),
			// An agent from before the break and the runs: logged out is its pause.
			managed("c-old", "acc_old", agent.CmdStatus),
			{ID: "zaros", Account: "acc_off", Kind: agent.KindAttached, Commands: []string{agent.CmdStatus, agent.CmdRunStatus}},
		},
		status: map[string]agent.Status{
			"c-out":  game(false, &agent.BreakState{}),
			"c-game": game(true, &agent.BreakState{}),
			"c-brk":  game(true, &agent.BreakState{Active: true}),
			"c-old":  game(false, nil),
			"zaros":  game(false, nil),
		},
		runs: map[string]agent.RunStatus{"c-out": {Run: &agent.Run{ID: "run-1", State: agent.RunDone}}},
	}
	audit := &memAudit{}
	r := newRec(store, cl, hub)
	r.Audit = audit
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	for account, want := range map[string]bool{"acc_out": true, "acc_brk": true, "acc_old": true, "acc_game": false} {
		img, restartedAt := deployedImage(cl, account)
		if moved := img == r.Image && restartedAt != "" && store.image(account) == r.Image; moved != want {
			t.Errorf("%s: restarted %v, want %v (image %s, restarted-at %q, desired %s)", account, moved, want, img, restartedAt, store.image(account))
		}
	}
	if img, _ := deployedImage(cl, "acc_off"); img != "" || store.image("acc_off") != oldImage {
		t.Fatalf("a stopped worker was started: %s %s", img, store.image("acc_off"))
	}
	entries := audit.all()
	if len(entries) != 3 {
		t.Fatalf("audit = %+v", entries)
	}
	pauses := map[string]string{}
	for _, e := range entries {
		var p map[string]string
		if err := json.Unmarshal(e.Params, &p); err != nil {
			t.Fatal(err)
		}
		if e.Actor != "reconciler" || e.Origin != "server" || e.Action != "worker_restart" || !e.OK || e.Error != "" ||
			p["reason"] != "versão nova, na pausa" || p["from"] != oldImage || p["to"] != r.Image {
			t.Fatalf("audit entry = %+v %v", e, p)
		}
		pauses[e.AccountID+"@"+e.AgentID] = p["pause"]
	}
	if want := map[string]string{"acc_out@c-out": PauseLoggedOut, "acc_brk@c-brk": PauseBreak, "acc_old@c-old": PauseLoggedOut}; !maps(pauses, want) {
		t.Fatalf("pauses = %v", pauses)
	}

	// The next passes leave them alone: they run the new image.
	applied := len(cl.applied)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cl.applied) != applied || len(audit.all()) != 3 {
		t.Fatalf("a second pass restarted again: %v, %d entries", cl.applied, len(audit.all()))
	}
	// The one in the game restarts once it logs out.
	hub.mu.Lock()
	hub.status["c-game"] = game(false, nil)
	hub.mu.Unlock()
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if img, _ := deployedImage(cl, "acc_game"); img != r.Image || len(audit.all()) != 4 {
		t.Fatalf("logged out, not restarted: %s, %d entries", img, len(audit.all()))
	}
}

func maps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Never during a run, under the owner's eyes, without the agent's word, on
// the attached agent, or for a worker that is not meant to run.
func TestAutoRestartNever(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(r *Reconciler, store *fakeStore, hub *fakeHub)
	}{
		{"a run is going", func(_ *Reconciler, _ *fakeStore, h *fakeHub) {
			h.runs["c1"] = agent.RunStatus{Run: &agent.Run{ID: "run-7", State: agent.RunRunning}}
		}},
		{"a recurring run waits for its safe point", func(_ *Reconciler, _ *fakeStore, h *fakeHub) {
			h.runs["c1"] = agent.RunStatus{Run: &agent.Run{ID: "run-8", State: agent.RunWaiting}}
		}},
		{"the VNC is open", func(_ *Reconciler, _ *fakeStore, h *fakeHub) { h.vnc["acc_a"] = true }},
		{"the agent does not answer", func(_ *Reconciler, _ *fakeStore, h *fakeHub) { h.down["c1"] = true }},
		{"in the game, no break", func(_ *Reconciler, _ *fakeStore, h *fakeHub) { h.status["c1"] = game(true, &agent.BreakState{}) }},
		{"no agent linked", func(_ *Reconciler, _ *fakeStore, h *fakeHub) { h.agents = nil }},
		{"only the attached agent", func(_ *Reconciler, _ *fakeStore, h *fakeHub) {
			h.agents = []agent.Info{{ID: "c1", Account: "acc_a", Kind: agent.KindAttached, Commands: []string{agent.CmdStatus, agent.CmdRunStatus}}}
		}},
		{"stopped", func(_ *Reconciler, s *fakeStore, _ *fakeHub) { s.desired[0].Running = false }},
		{"no WORKER_IMAGE", func(r *Reconciler, _ *fakeStore, _ *fakeHub) { r.Image = "" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Image: oldImage}}}
			cl := &fakeCluster{deps: map[string]kube.Deployment{kube.WorkerName("acc_a"): onOld("acc_a")}}
			hub := &fakeHub{
				agents: []agent.Info{managed("c1", "acc_a")},
				status: map[string]agent.Status{"c1": game(false, &agent.BreakState{Active: true})},
				runs:   map[string]agent.RunStatus{}, down: map[string]bool{}, vnc: map[string]bool{},
			}
			audit := &memAudit{}
			r := newRec(store, cl, hub)
			r.Audit = audit
			c.setup(r, store, hub)
			if err := r.Once(context.Background()); err != nil {
				t.Fatal(err)
			}
			if img, restartedAt := deployedImage(cl, "acc_a"); restartedAt != "" || (img != "" && img != oldImage) {
				t.Fatalf("restarted: image %s, restarted-at %q", img, restartedAt)
			}
			if store.image("acc_a") != oldImage || len(audit.all()) != 0 {
				t.Fatalf("desired image %s, audit %+v", store.image("acc_a"), audit.all())
			}
		})
	}
}

// A failed restart goes back to the old image (no later pass rolls the pod
// outside a pause), is audited as failed and waits, doubling; after one that
// worked, the worker is not restarted again for the same image.
func TestAutoRestartBackoff(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Image: oldImage}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{kube.WorkerName("acc_a"): onOld("acc_a")}, fail: errors.New("denied by policy")}
	hub := &fakeHub{agents: []agent.Info{managed("c1", "acc_a")}, status: map[string]agent.Status{"c1": game(false, nil)}}
	audit := &memAudit{}
	r := newRec(store, cl, hub)
	r.Audit = audit
	t0 := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	clock := t0
	r.now = func() time.Time { return clock }
	pass := func(at time.Duration) {
		t.Helper()
		clock = t0.Add(at)
		_ = r.Once(context.Background())
	}

	pass(0)
	e := audit.all()
	if len(e) != 1 || e[0].OK || e[0].Error == "" || store.image("acc_a") != oldImage || len(cl.applied) != 0 {
		t.Fatalf("failed restart: audit %+v, desired %s, applied %v", e, store.image("acc_a"), cl.applied)
	}
	pass(time.Minute) // within the backoff
	if len(audit.all()) != 1 {
		t.Fatalf("tried again within the backoff: %+v", audit.all())
	}
	pass(2 * time.Minute) // 2 min: tried, fails again; next in 4 min
	pass(5 * time.Minute)
	if len(audit.all()) != 2 {
		t.Fatalf("backoff did not double: %d entries", len(audit.all()))
	}
	cl.mu.Lock()
	cl.fail = nil
	cl.mu.Unlock()
	pass(6 * time.Minute)
	if e := audit.all(); len(e) != 3 || !e[2].OK {
		t.Fatalf("restart after the backoff: %+v", e)
	}
	if img, _ := deployedImage(cl, "acc_a"); img != r.Image {
		t.Fatalf("image after the restart: %s", img)
	}

	// Once per image: a worker that ends up on the old image again (here by
	// hand) is not restarted again for the same one, only for a newer.
	store.mu.Lock()
	store.desired[0].Image = oldImage
	store.mu.Unlock()
	pass(10 * time.Minute)
	if img, _ := deployedImage(cl, "acc_a"); img != oldImage || len(audit.all()) != 3 {
		t.Fatalf("restarted twice for one image: %s, %d entries", img, len(audit.all()))
	}
	r.Image = "ghcr.io/felipemalacarne/chronicle-worker:newer"
	pass(11 * time.Minute)
	if img, _ := deployedImage(cl, "acc_a"); img != r.Image || len(audit.all()) != 4 {
		t.Fatalf("a newer image: %s, %d entries", img, len(audit.all()))
	}
}

// The owner's Reiniciar and the automatic restart do not both roll the pod:
// the automatic one checks again under the pass lock, as it does for the VNC.
func TestAutoRestartAfterTheOwner(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Image: oldImage}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{kube.WorkerName("acc_a"): onOld("acc_a")}}
	hub := &fakeHub{vnc: map[string]bool{}}
	audit := &memAudit{}
	r := newRec(store, cl, hub)
	r.Audit = audit
	if err := r.Restart(context.Background(), "acc_a"); err != nil {
		t.Fatal(err)
	}
	r.autoRestart(context.Background(), "acc_a", managed("c1", "acc_a"), oldImage, PauseLoggedOut)
	if len(cl.applied) != 1 || len(audit.all()) != 0 {
		t.Fatalf("restarted twice: %v, audit %+v", cl.applied, audit.all())
	}
	store.mu.Lock()
	store.desired[0].Image = oldImage
	store.mu.Unlock()
	hub.vnc["acc_a"] = true // the owner opened the VNC after the pause was read
	r.autoRestart(context.Background(), "acc_a", managed("c1", "acc_a"), oldImage, PauseLoggedOut)
	if len(cl.applied) != 1 || len(audit.all()) != 0 || store.image("acc_a") != oldImage {
		t.Fatalf("restarted under the VNC: %v, audit %+v", cl.applied, audit.all())
	}
}
