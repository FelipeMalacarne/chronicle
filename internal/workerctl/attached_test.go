package workerctl

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
)

const publicURL = "https://chronicle.saradomin.example"

func zarosPool(used int) agent.Info {
	return agent.Info{ID: "pool", Host: "zaros", Kind: agent.KindAttached, Slots: 3, Used: used, Connected: true}
}

func slotClient(account string) agent.Info {
	return agent.Info{ID: "slot-" + account, Account: account, Host: "zaros", Kind: agent.KindAttached, Slot: true, Connected: true}
}

func newAttachedRec(store *fakeStore, cl *fakeCluster, hub *fakeHub) *Reconciler {
	r := newRec(store, cl, hub)
	r.PublicURL = publicURL
	return r
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// waitIdle waits for every attached bootstrap in flight to end, which makes
// what it wrote safe to read.
func waitIdle(t *testing.T, r *Reconciler) {
	t.Helper()
	waitFor(t, "the bootstraps to end", func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.booting) == 0 })
}

func (h *fakeHub) bootstraps() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.boots)
}

func (h *fakeHub) releases() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.released)
}

// An account placed on zaros gets its bootstrap through the agent with slots,
// with the public URL (the pod's in-cluster Service is not reachable there),
// and no Deployment; once its client is linked, later passes leave it alone.
func TestAttachedPlacementBootstrapsIntoASlot(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros", World: 420}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}}
	r := newAttachedRec(store, cl, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the bootstrap", func() bool { return len(hub.bootstraps()) == 1 })
	if got := hub.bootstraps(); got[0] != "pool" {
		t.Fatalf("bootstrap went to %v, want the pool", got)
	}
	waitIdle(t, r)
	hub.mu.Lock()
	boot := hub.boot
	hub.mu.Unlock()
	if boot.Account != "acc_a" || boot.ServerURL != publicURL || boot.PluginToken != "ipl_new" || boot.World != 420 || boot.Session.SessionID != "sid" {
		t.Fatalf("bootstrap params: %+v", boot)
	}
	if len(cl.applied) != 0 {
		t.Fatalf("a Deployment was applied for an attached placement: %v", cl.applied)
	}
	if store.errs["acc_a"] != "" {
		t.Fatalf("last error: %q", store.errs["acc_a"])
	}
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := hub.bootN.Load(); n != 1 {
		t.Fatalf("a worker whose client is in a slot was bootstrapped again: %d", n)
	}
}

// The agent not being linked is not an error: the account waits for it.
func TestAttachedPlacementWaitsForItsAgent(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{{ID: "other", Host: "elsewhere", Kind: agent.KindAttached, Slots: 2, Connected: true}}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.bootN.Load() != 0 || len(store.errs) != 0 || store.tokens != 0 {
		t.Fatalf("called %d, errs %v, tokens %d", hub.bootN.Load(), store.errs, store.tokens)
	}
}

func TestAttachedPlacementReportsWhatBlocksIt(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(3)}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.bootN.Load() != 0 || store.errs["acc_a"] != "no free slot on zaros (3 of 3 in use)" {
		t.Fatalf("full pool: called %d, errs %v", hub.bootN.Load(), store.errs)
	}
	hub.agents = []agent.Info{zarosPool(0)}
	r.PublicURL = ""
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.bootN.Load() != 0 || store.errs["acc_a"] != "PUBLIC_BASE_URL is not set on the server" {
		t.Fatalf("no public URL: called %d, errs %v", hub.bootN.Load(), store.errs)
	}
}

// A pass does not wait for a bootstrap, which starts a client and may take
// minutes, and does not send a second one for the same account meanwhile.
func TestAttachedBootstrapInFlightIsNotDoubled(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}, gate: make(chan struct{})}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	for range 3 {
		if err := r.Once(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the bootstrap to start", func() bool { return hub.bootN.Load() >= 1 })
	if n := hub.bootN.Load(); n != 1 {
		t.Fatalf("bootstraps in flight: %d", n)
	}
	close(hub.gate)
	waitIdle(t, r)
}

// A stopped account, and one moved elsewhere, lose their slot and their
// token; a client of a legacy attached agent, which only declares its
// account, is never touched.
func TestSlotClientsOfOtherPlacementsAreReleased(t *testing.T) {
	store := &fakeStore{desired: []Desired{
		{Account: "acc_stopped", Running: false, Placement: "zaros"},
		{Account: "acc_moved", Running: true, Placement: "saradomin-two"},
		{Account: "acc_cluster", Running: true},
		{Account: "acc_kept", Running: true, Placement: "zaros"},
		{Account: "acc_legacy", Running: false, Placement: "zaros"},
	}}
	legacy := agent.Info{ID: "legacy", Account: "acc_legacy", Host: "zaros", Kind: agent.KindAttached, Connected: true}
	hub := &fakeHub{agents: []agent.Info{
		zarosPool(4), slotClient("acc_stopped"), slotClient("acc_moved"), slotClient("acc_cluster"), slotClient("acc_gone"), slotClient("acc_kept"), legacy,
	}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	released := hub.releases()
	slices.Sort(released)
	if want := []string{"acc_cluster", "acc_gone", "acc_moved", "acc_stopped"}; !slices.Equal(released, want) {
		t.Fatalf("released %v, want %v", released, want)
	}
	for _, a := range released {
		if !slices.Contains(store.revoked, a) {
			t.Fatalf("token of %s not revoked: %v", a, store.revoked)
		}
	}
	if slices.Contains(released, "acc_legacy") || slices.Contains(released, "acc_kept") {
		t.Fatalf("released a client that stays: %v", released)
	}
}

// Moving a running worker from the cluster to zaros takes one pass: the pod
// goes away and the slot is filled. It does not log in (a bootstrap never does).
func TestMoveFromClusterToAttached(t *testing.T) {
	old := kube.WorkerDeployment("chronicle", kube.WorkerSpec{Account: "acc_a", Image: "img:1"})
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros", Image: "img:1"}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{old.Metadata.Name: old}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}}
	r := newAttachedRec(store, cl, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cl.deleted, []string{"worker-acc-a"}) || !slices.Contains(store.revoked, "acc_a") {
		t.Fatalf("the pod stays: deleted %v, revoked %v", cl.deleted, store.revoked)
	}
	waitFor(t, "the bootstrap", func() bool { return len(hub.bootstraps()) == 1 })
	waitIdle(t, r)
	if slices.Contains(hub.calls, agent.CmdLogin) {
		t.Fatalf("the move logged in: %v", hub.calls)
	}
}

// And back: the slot is released and the Deployment applied on the same pass.
func TestMoveFromAttachedToCluster(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(1), slotClient("acc_a")}}
	r := newAttachedRec(store, cl, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hub.releases(), []string{"acc_a"}) || !slices.Equal(cl.applied, []string{"worker-acc-a"}) {
		t.Fatalf("released %v, applied %v", hub.releases(), cl.applied)
	}
}

// With WORKERS=off the reconciler has no cluster and still manages the slots.
func TestNoClusterManagesOnlyAttachedPlacements(t *testing.T) {
	store := &fakeStore{desired: []Desired{
		{Account: "acc_a", Running: true, Placement: "zaros"},
		{Account: "acc_b", Running: true},
	}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}}
	r := newAttachedRec(store, nil, hub)
	r.Cluster = nil // newRec wraps the nil *fakeCluster in a non-nil interface
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the bootstrap", func() bool { return len(hub.bootstraps()) == 1 })
	waitIdle(t, r)
	if len(r.Pods()) != 0 {
		t.Fatalf("pods without a cluster: %v", r.Pods())
	}
	// Reiniciar of a cluster worker says why it cannot.
	if err := r.Restart(context.Background(), "acc_b"); err == nil {
		t.Fatal("restart of a cluster worker without a cluster")
	}
}

// Reiniciar of a worker in a slot frees it; the next pass fills it again.
func TestRestartOfAttachedPlacementReleasesTheSlot(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}, {Account: "acc_b", Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(1), slotClient("acc_a")}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	ctx := context.Background()
	if err := r.Restart(ctx, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hub.releases(), []string{"acc_a"}) {
		t.Fatalf("released %v", hub.releases())
	}
	if err := r.Once(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the new bootstrap", func() bool { return len(hub.bootstraps()) == 1 })
	waitIdle(t, r)
	if err := r.Restart(ctx, "acc_b"); err != ErrNotRunning {
		t.Fatalf("restart of a stopped worker = %v", err)
	}
}

// The auto login of a slot client is not refused because of the client itself.
func TestAutoLoginInASlot(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros", World: 330, AutoLogin: true, WasLoggedIn: true}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the bootstrap", func() bool { return len(hub.bootstraps()) == 1 })
	waitIdle(t, r)
	if store.logins["acc_a"] != "auto login ok" || store.errs["acc_a"] != "" || hub.world != 330 {
		t.Fatalf("login %q, error %q, world %d", store.logins["acc_a"], store.errs["acc_a"], hub.world)
	}
}

// A bootstrap that fails is not sent again on every pass: each one starts a
// client on the owner's machine. It waits attachedBackoff, doubling, and a
// stop forgets the failures.
func TestFailedAttachedBootstrapBacksOff(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}, down: map[string]bool{"pool": true}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	pass := func() {
		t.Helper()
		if err := r.Once(context.Background()); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, r)
	}
	pass()
	pass()
	if n := hub.bootN.Load(); n != 1 {
		t.Fatalf("bootstraps right after a failure: %d", n)
	}
	now = now.Add(attachedBackoff + time.Second)
	pass()
	now = now.Add(attachedBackoff + time.Second) // the second wait is twice as long
	pass()
	if n := hub.bootN.Load(); n != 2 {
		t.Fatalf("bootstraps after one backoff: %d", n)
	}
	store.desired[0].Running = false
	pass()
	store.desired[0].Running = true
	pass()
	if n := hub.bootN.Load(); n != 3 {
		t.Fatalf("a start after a stop still waited: %d bootstraps", n)
	}
}

// A worker stopped while its bootstrap is in flight is released by a later
// pass, once the bootstrap ended: a stop sent during mb-client start could
// land first and leave the client running.
func TestSlotIsNotReleasedWhileItsBootstrapIsInFlight(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Placement: "zaros"}}}
	hub := &fakeHub{agents: []agent.Info{zarosPool(0)}, gate: make(chan struct{})}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the bootstrap to start", func() bool { return hub.bootN.Load() == 1 })
	hub.mu.Lock()
	hub.agents = []agent.Info{zarosPool(1), slotClient("acc_a")}
	hub.mu.Unlock()
	store.desired[0].Running = false
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hub.releases(); len(got) != 0 {
		t.Fatalf("released during the bootstrap: %v", got)
	}
	close(hub.gate)
	waitIdle(t, r)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hub.releases(); !slices.Equal(got, []string{"acc_a"}) {
		t.Fatalf("released %v after the bootstrap", got)
	}
}

// A slot freed in a pass is filled in the same pass, and two accounts that
// wait for one free slot do not both get a bootstrap.
func TestFreedSlotIsFilledInTheSamePass(t *testing.T) {
	store := &fakeStore{desired: []Desired{
		{Account: "acc_moved", Running: true},
		{Account: "acc_b", Running: true, Placement: "zaros"},
		{Account: "acc_c", Running: true, Placement: "zaros"},
	}}
	hub := &fakeHub{agents: []agent.Info{{ID: "pool", Host: "zaros", Kind: agent.KindAttached, Slots: 1, Used: 1, Connected: true}, slotClient("acc_moved")}}
	r := newAttachedRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, r)
	if got := hub.releases(); !slices.Equal(got, []string{"acc_moved"}) {
		t.Fatalf("released %v", got)
	}
	if n := hub.bootN.Load(); n != 1 {
		t.Fatalf("bootstraps for one freed slot: %d", n)
	}
	if store.errs["acc_c"] == "" {
		t.Fatalf("the account left without a slot says nothing: %v", store.errs)
	}
}
