// Package workerctl runs the managed workers (docs/11): it makes the
// Kubernetes Deployments match the desired state in the database, and
// bootstraps each worker's agent when it links. It owns no state of its own:
// the database says what should run, the cluster what does, and the hub who
// is linked.
package workerctl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/kube"
)

// Desired is one worker row as the reconciler needs it.
type Desired struct {
	Account string
	Running bool
	World   int
	// Image is the image the worker was started (or restarted) with; empty
	// takes the reconciler's current one.
	Image string
}

// Store is the reconciler's view of the database (internal/store/workers).
type Store interface {
	Desired(ctx context.Context) ([]Desired, error)
	Session(ctx context.Context, account string) (agent.GameSession, error)
	MintPluginToken(ctx context.Context, account string) (string, error)
	RevokePluginToken(ctx context.Context, account string) error
	SetError(ctx context.Context, account, msg string) error
}

// Cluster is the reconciler's view of the API server (internal/kube).
type Cluster interface {
	ApplyDeployment(ctx context.Context, d kube.Deployment) error
	DeleteDeployment(ctx context.Context, name string) error
	ListDeployments(ctx context.Context, selector string) ([]kube.Deployment, error)
	ListPods(ctx context.Context, selector string) ([]kube.Pod, error)
	DeletePod(ctx context.Context, name string) error
}

// Hub is the reconciler's view of the agents' link (internal/agent).
type Hub interface {
	CallAgent(ctx context.Context, agentID, command string, params, into any) error
}

// Reconciler keeps the cluster's workers equal to the database's.
type Reconciler struct {
	Store     Store
	Cluster   Cluster
	Hub       Hub
	Log       *slog.Logger
	Namespace string
	// Image is the current chronicle-worker image (WORKER_IMAGE).
	Image string
	// AgentURL is CHRONICLE_URL of the agent in the pod and ServerURL the
	// Chronicle Link's; both are the in-cluster Service (WORKER_SERVER_URL).
	ServerURL string
	// Version is the server's build, written on every Deployment.
	Version string
	// Interval between full passes (default 30 s); Kick runs one sooner.
	Interval time.Duration

	kick chan struct{}
	once sync.Once

	mu   sync.Mutex
	pods map[string]PodInfo // by account, from the last pass
}

// PodInfo is what the last pass saw of an account's pod.
type PodInfo struct {
	Deployment string    `json:"deployment"`
	Pod        string    `json:"pod,omitempty"`
	State      string    `json:"state"`
	Reason     string    `json:"reason,omitempty"`
	Image      string    `json:"image,omitempty"`
	Version    string    `json:"version,omitempty"`
	Restarts   int32     `json:"restarts,omitempty"`
	Seen       time.Time `json:"seen"`
}

func (r *Reconciler) init() {
	r.once.Do(func() {
		r.kick = make(chan struct{}, 1)
		if r.Interval <= 0 {
			r.Interval = 30 * time.Second
		}
		if r.Log == nil {
			r.Log = slog.Default()
		}
	})
}

// Kick asks for a pass now (after a start or stop).
func (r *Reconciler) Kick() {
	r.init()
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// Run passes until ctx ends.
func (r *Reconciler) Run(ctx context.Context) {
	r.init()
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		if err := r.Once(ctx); err != nil && ctx.Err() == nil {
			r.Log.Warn("workers: reconcile", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.kick:
		}
	}
}

// Once is one pass: apply the Deployment of every running worker, delete the
// rest of the worker Deployments (stopped, or of an account the database no
// longer knows), and refresh the pod view.
func (r *Reconciler) Once(ctx context.Context) error {
	r.init()
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return fmt.Errorf("desired workers: %w", err)
	}
	have, err := r.Cluster.ListDeployments(ctx, kube.WorkerSelector)
	if err != nil {
		return fmt.Errorf("list worker deployments: %w", err)
	}
	running := map[string]bool{}
	var errs []error
	for _, w := range want {
		if !w.Running {
			continue
		}
		running[kube.WorkerName(w.Account)] = true
		image := w.Image
		if image == "" {
			image = r.Image
		}
		if image == "" {
			_ = r.Store.SetError(ctx, w.Account, "WORKER_IMAGE is not set on the server")
			continue
		}
		d := kube.WorkerDeployment(r.Namespace, kube.WorkerSpec{Account: w.Account, Image: image, ServerURL: r.ServerURL, Version: r.Version})
		// Server-side apply of an unchanged spec changes nothing, so the pod
		// only restarts when the image (a restart) changes.
		if err := r.Cluster.ApplyDeployment(ctx, d); err != nil {
			errs = append(errs, fmt.Errorf("apply %s: %w", d.Metadata.Name, err))
			_ = r.Store.SetError(ctx, w.Account, err.Error())
			continue
		}
	}
	for _, d := range have {
		if running[d.Metadata.Name] {
			continue
		}
		account := d.Metadata.Labels[kube.LabelAccount]
		if err := r.Cluster.DeleteDeployment(ctx, d.Metadata.Name); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", d.Metadata.Name, err))
			continue
		}
		r.Log.Info("workers: deleted", "deployment", d.Metadata.Name, "account", account)
		if account != "" {
			if err := r.Store.RevokePluginToken(ctx, account); err != nil {
				errs = append(errs, fmt.Errorf("revoke token of %s: %w", account, err))
			}
		}
	}
	r.refreshPods(ctx)
	return errors.Join(errs...)
}

// refreshPods reads the Deployments again (the pass changed them) and their
// pods, for the web.
func (r *Reconciler) refreshPods(ctx context.Context) {
	deployments, err := r.Cluster.ListDeployments(ctx, kube.WorkerSelector)
	if err != nil {
		r.Log.Warn("workers: list deployments", "err", err)
		return
	}
	pods, err := r.Cluster.ListPods(ctx, kube.WorkerSelector)
	if err != nil {
		r.Log.Warn("workers: list pods", "err", err)
		return
	}
	now := time.Now()
	view := map[string]PodInfo{}
	for _, d := range deployments {
		account := d.Metadata.Labels[kube.LabelAccount]
		info := PodInfo{Deployment: d.Metadata.Name, State: kube.PodPending, Version: d.Metadata.Annotations["chronicle.ftm.dev.br/version"], Seen: now}
		if cs := d.Spec.Template.Spec.Containers; len(cs) > 0 {
			info.Image = cs[0].Image
		}
		view[account] = info
	}
	for _, p := range pods {
		account := p.Metadata.Labels[kube.LabelAccount]
		info := view[account]
		info.Deployment = kube.WorkerName(account)
		info.Pod = p.Metadata.Name
		info.State, info.Reason = kube.PodState(p)
		for _, cs := range p.Status.ContainerStatuses {
			info.Restarts += cs.RestartCount
		}
		info.Seen = now
		view[account] = info
	}
	r.mu.Lock()
	r.pods = view
	r.mu.Unlock()
}

// Restart deletes the account's worker pods; the Deployment makes a new one,
// which links and gets a fresh bootstrap. It logs the account out.
func (r *Reconciler) Restart(ctx context.Context, account string) error {
	pods, err := r.Cluster.ListPods(ctx, kube.WorkerSelector)
	if err != nil {
		return err
	}
	for _, p := range pods {
		if p.Metadata.Labels[kube.LabelAccount] != account {
			continue
		}
		if err := r.Cluster.DeletePod(ctx, p.Metadata.Name); err != nil {
			return err
		}
	}
	r.Kick()
	return nil
}

// Pods is the pod view of the last pass, by account.
func (r *Reconciler) Pods() map[string]PodInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]PodInfo, len(r.pods))
	for k, v := range r.pods {
		out[k] = v
	}
	return out
}

// OnConnect bootstraps a managed agent that just linked: unless its client
// already runs (the link dropped, the pod did not), it gets the account's
// session, a fresh Chronicle Link token and the world. Attached agents are
// left alone.
func (r *Reconciler) OnConnect(info agent.Info) {
	r.init()
	if info.Kind != agent.KindManaged || info.Account == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if err := r.bootstrap(ctx, info); err != nil {
		r.Log.Warn("workers: bootstrap", "account", info.Account, "agent", info.ID, "err", err)
		_ = r.Store.SetError(ctx, info.Account, "bootstrap: "+err.Error())
		return
	}
	_ = r.Store.SetError(ctx, info.Account, "")
}

func (r *Reconciler) bootstrap(ctx context.Context, info agent.Info) error {
	var st agent.Status
	if err := r.Hub.CallAgent(ctx, info.ID, agent.CmdStatus, nil, &st); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	if st.Running {
		r.Log.Info("workers: agent relinked with its client up", "account", info.Account, "agent", info.ID)
		return nil
	}
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return err
	}
	world, desired := 0, false
	for _, w := range want {
		if w.Account == info.Account {
			world, desired = w.World, w.Running
		}
	}
	if !desired {
		return errors.New("the worker is not meant to run; the reconciler deletes it")
	}
	session, err := r.Store.Session(ctx, info.Account)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	token, err := r.Store.MintPluginToken(ctx, info.Account)
	if err != nil {
		return fmt.Errorf("plugin token: %w", err)
	}
	params := agent.BootstrapParams{Account: info.Account, Session: session, PluginToken: token, ServerURL: r.ServerURL, World: world}
	var res agent.BootstrapResult
	if err := r.Hub.CallAgent(ctx, info.ID, agent.CmdWorkerBootstrap, params, &res); err != nil {
		return err
	}
	if !res.Started {
		return errors.New("the client did not come up: " + truncate(res.Output, 300))
	}
	r.Log.Info("workers: bootstrapped", "account", info.Account, "agent", info.ID)
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
