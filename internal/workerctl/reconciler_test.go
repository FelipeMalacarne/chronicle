package workerctl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
)

type fakeStore struct {
	mu      sync.Mutex
	desired []Desired
	tokens  int
	revoked []string
	errs    map[string]string
}

func (f *fakeStore) Desired(context.Context) ([]Desired, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.desired), nil
}

func (f *fakeStore) Session(_ context.Context, account string) (agent.GameSession, error) {
	return agent.GameSession{Character: "Char " + account, SessionID: "sid", CharacterID: "cid"}, nil
}

func (f *fakeStore) MintPluginToken(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens++
	return "ipl_new", nil
}

func (f *fakeStore) RevokePluginToken(_ context.Context, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, account)
	return nil
}

func (f *fakeStore) SetError(_ context.Context, account, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errs == nil {
		f.errs = map[string]string{}
	}
	f.errs[account] = msg
	return nil
}

type fakeCluster struct {
	mu          sync.Mutex
	deps        map[string]kube.Deployment
	applied     []string
	deleted     []string
	deletedPods []string
	fail        error
}

func (f *fakeCluster) ApplyDeployment(_ context.Context, d kube.Deployment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.deps[d.Metadata.Name] = d
	f.applied = append(f.applied, d.Metadata.Name)
	return nil
}

func (f *fakeCluster) DeleteDeployment(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.deps, name)
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeCluster) ListDeployments(context.Context, string) ([]kube.Deployment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []kube.Deployment
	for _, d := range f.deps {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeCluster) ListPods(context.Context, string) ([]kube.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []kube.Pod
	for _, d := range f.deps {
		p := kube.Pod{Metadata: kube.ObjectMeta{Name: d.Metadata.Name + "-abc", Labels: d.Spec.Template.Metadata.Labels}}
		p.Status.ContainerStatuses = []kube.ContainerStatus{{Ready: true}}
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeCluster) DeletePod(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedPods = append(f.deletedPods, name)
	return nil
}

type fakeHub struct {
	mu      sync.Mutex
	running bool
	calls   []string
	boot    agent.BootstrapParams
}

func (h *fakeHub) CallAgent(_ context.Context, _ string, command string, params, into any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, command)
	var out any
	switch command {
	case agent.CmdStatus:
		out = agent.Status{Running: h.running}
	case agent.CmdWorkerBootstrap:
		h.boot = params.(agent.BootstrapParams)
		out = agent.BootstrapResult{Started: true}
	default:
		return errors.New("unexpected " + command)
	}
	b, _ := json.Marshal(out)
	return json.Unmarshal(b, into)
}

func newRec(store *fakeStore, cl *fakeCluster, hub *fakeHub) *Reconciler {
	return &Reconciler{
		Store: store, Cluster: cl, Hub: hub, Namespace: "chronicle",
		Image: "ghcr.io/felipemalacarne/chronicle-worker:new", ServerURL: "http://chronicle.chronicle.svc", Version: "v1",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestReconcileAppliesAndDeletes(t *testing.T) {
	store := &fakeStore{desired: []Desired{
		{Account: "acc_a", Running: true, Image: "ghcr.io/felipemalacarne/chronicle-worker:old"},
		{Account: "acc_b", Running: true},
		{Account: "acc_c", Running: false},
	}}
	stale := kube.WorkerDeployment("chronicle", kube.WorkerSpec{Account: "acc_c", Image: "x"})
	orphan := kube.WorkerDeployment("chronicle", kube.WorkerSpec{Account: "acc_gone", Image: "x"})
	cl := &fakeCluster{deps: map[string]kube.Deployment{stale.Metadata.Name: stale, orphan.Metadata.Name: orphan}}
	r := newRec(store, cl, &fakeHub{})
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := cl.deps["worker-acc-a"]; !ok {
		t.Fatal("acc_a not applied")
	}
	if img := cl.deps["worker-acc-a"].Spec.Template.Spec.Containers[0].Image; img != "ghcr.io/felipemalacarne/chronicle-worker:old" {
		t.Fatalf("a running worker keeps the image it started with, got %s", img)
	}
	if img := cl.deps["worker-acc-b"].Spec.Template.Spec.Containers[0].Image; img != r.Image {
		t.Fatalf("a worker without an image takes the current one, got %s", img)
	}
	if _, ok := cl.deps["worker-acc-c"]; ok {
		t.Fatal("stopped worker not deleted")
	}
	if _, ok := cl.deps["worker-acc-gone"]; ok {
		t.Fatal("orphan worker not deleted")
	}
	if !slices.Contains(store.revoked, "acc_c") || !slices.Contains(store.revoked, "acc_gone") {
		t.Fatalf("tokens of deleted workers not revoked: %v", store.revoked)
	}
	pods := r.Pods()
	if pods["acc_a"].State != kube.PodRunning || pods["acc_a"].Version != "v1" {
		t.Fatalf("pod view: %+v", pods)
	}
}

func TestReconcileRecordsApplyError(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{}, fail: errors.New("denied by policy")}
	r := newRec(store, cl, &fakeHub{})
	if err := r.Once(context.Background()); err == nil {
		t.Fatal("apply error not returned")
	}
	if store.errs["acc_a"] != "denied by policy" {
		t.Fatalf("error not recorded: %v", store.errs)
	}
}

func TestOnConnectBootstraps(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, World: 420}}}
	hub := &fakeHub{}
	r := newRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	r.OnConnect(agent.Info{ID: "x", Account: "acc_a", Kind: agent.KindManaged})
	if hub.boot.Account != "acc_a" || hub.boot.PluginToken != "ipl_new" || hub.boot.World != 420 || hub.boot.ServerURL != r.ServerURL || hub.boot.Session.SessionID != "sid" {
		t.Fatalf("bootstrap params: %+v", hub.boot)
	}
	if store.tokens != 1 || store.errs["acc_a"] != "" {
		t.Fatalf("tokens %d errs %v", store.tokens, store.errs)
	}
}

func TestOnConnectSkips(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true}, {Account: "acc_b", Running: false}}}
	// The client already runs (the link dropped, not the pod): no new token,
	// which would cut off the running Chronicle Link.
	hub := &fakeHub{running: true}
	r := newRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub)
	r.OnConnect(agent.Info{ID: "x", Account: "acc_a", Kind: agent.KindManaged})
	if store.tokens != 0 || slices.Contains(hub.calls, agent.CmdWorkerBootstrap) {
		t.Fatalf("bootstrapped a running client: %v", hub.calls)
	}
	// Attached agents are never bootstrapped.
	hub2 := &fakeHub{}
	newRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub2).OnConnect(agent.Info{ID: "y", Account: "acc_a", Kind: agent.KindAttached})
	if len(hub2.calls) != 0 {
		t.Fatalf("attached agent called: %v", hub2.calls)
	}
	// A pod of a stopped worker gets nothing, and the error says why.
	hub3 := &fakeHub{}
	newRec(store, &fakeCluster{deps: map[string]kube.Deployment{}}, hub3).OnConnect(agent.Info{ID: "z", Account: "acc_b", Kind: agent.KindManaged})
	if slices.Contains(hub3.calls, agent.CmdWorkerBootstrap) || store.errs["acc_b"] == "" {
		t.Fatalf("stopped worker bootstrapped: %v %v", hub3.calls, store.errs)
	}
}

// A running worker is not touched by a new server's pod template: only its
// image changing (a restart to the new version) or Restart applies it.
func TestReconcileLeavesRunningWorkersAlone(t *testing.T) {
	old := kube.WorkerDeployment("chronicle", kube.WorkerSpec{Account: "acc_a", Image: "img:1"})
	old.Spec.Template.Spec.Containers[0].Resources.Requests["cpu"] = "500m" // an older template
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true, Image: "img:1"}}}
	cl := &fakeCluster{deps: map[string]kube.Deployment{old.Metadata.Name: old}}
	r := newRec(store, cl, &fakeHub{})
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cl.applied) != 0 {
		t.Fatalf("a running worker on its image was re-applied: %v", cl.applied)
	}
	// Restart applies the current template and replaces the pod.
	if err := r.Restart(context.Background(), "acc_a"); err != nil {
		t.Fatal(err)
	}
	if len(cl.applied) != 1 || len(cl.deletedPods) != 1 {
		t.Fatalf("restart: applied %v, deleted pods %v", cl.applied, cl.deletedPods)
	}
	// A new image (restart to the new version) is applied by the pass.
	store.desired[0].Image = "img:2"
	if err := r.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cl.applied) != 2 || cl.deps["worker-acc-a"].Spec.Template.Spec.Containers[0].Image != "img:2" {
		t.Fatalf("new image not applied: %v", cl.applied)
	}
}
