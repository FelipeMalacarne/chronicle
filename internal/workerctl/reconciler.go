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
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
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
	// Profile is the account's effective client profile (docs/11 section 4.3).
	Profile(ctx context.Context, account string) (agent.ClientProfile, error)
}

// Cluster is the reconciler's view of the API server (internal/kube).
type Cluster interface {
	ApplyDeployment(ctx context.Context, d kube.Deployment) error
	DeleteDeployment(ctx context.Context, name string) error
	ListDeployments(ctx context.Context, selector string) ([]kube.Deployment, error)
	ListPods(ctx context.Context, selector string) ([]kube.Pod, error)
	// ListPodMetrics and ListResourceQuotas feed the usage and the capacity
	// (docs/11, F7); either may fail (no metrics-server, no Role rule)
	// without failing the pass.
	ListPodMetrics(ctx context.Context, selector string) ([]kube.PodMetrics, error)
	ListResourceQuotas(ctx context.Context) ([]kube.ResourceQuota, error)
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

	mu       sync.Mutex
	pods     map[string]PodInfo // by account, from the last pass
	usage    map[string]Usage   // by account: the metrics of the pod in pods
	capacity *Capacity          // nil: no quota, or it could not be read
	// metricsDown and quotaDown remember a failing read, so it is logged
	// when it starts failing and when it is back, not on every pass.
	metricsDown, quotaDown bool
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
	deployed := map[string]string{} // name -> image of the worker container
	for _, d := range have {
		if cs := d.Spec.Template.Spec.Containers; len(cs) > 0 {
			deployed[d.Metadata.Name] = cs[0].Image
		}
	}
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
		name := kube.WorkerName(w.Account)
		// A worker that exists on the image it should run is left alone: a
		// new server version may build another pod template (resources,
		// variables, mounts), and applying it would restart the pod, logging
		// the account out in the middle of whatever it does. Template changes
		// reach a worker only on a start or on the owner's restart (Restart,
		// or a new image).
		if img, ok := deployed[name]; ok && img == image {
			continue
		}
		if err := r.apply(ctx, w.Account, image, ""); err != nil {
			errs = append(errs, err)
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
		if why, ok := kube.DeploymentBlocked(d); ok {
			info.State, info.Reason = kube.PodBlocked, why
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
	r.refreshUsage(ctx, view, now)
}

// refreshUsage reads the metrics of the pods in view and the namespace's
// quotas, on every pass (two GETs; metrics-server samples every 15 s or so).
// Neither is needed to run workers: when one cannot be read (no
// metrics-server, no Role rule), its part is left out of the answer and the
// pass goes on.
func (r *Reconciler) refreshUsage(ctx context.Context, view map[string]PodInfo, now time.Time) {
	var usage map[string]Usage
	metrics, merr := r.Cluster.ListPodMetrics(ctx, kube.WorkerSelector)
	if merr == nil {
		byPod := make(map[string]kube.PodMetrics, len(metrics))
		for _, m := range metrics {
			byPod[m.Metadata.Name] = m
		}
		usage = map[string]Usage{}
		for account, info := range view {
			m, ok := byPod[info.Pod]
			if info.Pod == "" || !ok {
				continue
			}
			u, err := usageOf(m)
			if err != nil {
				r.Log.Warn("workers: pod metrics", "pod", info.Pod, "err", err)
				continue
			}
			usage[account] = u
		}
	}
	var capacity *Capacity
	quotas, qerr := r.Cluster.ListResourceQuotas(ctx)
	if qerr == nil {
		if c, ok := capacityOf(quotas, now); ok {
			capacity = &c
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usage, r.capacity = usage, capacity
	r.metricsDown = r.noteRead(r.metricsDown, "pod metrics (metrics.k8s.io)", merr)
	r.quotaDown = r.noteRead(r.quotaDown, "resource quotas", qerr)
}

// noteRead logs an optional read that starts failing or works again, and
// returns whether it is failing now.
func (r *Reconciler) noteRead(wasDown bool, what string, err error) bool {
	switch {
	case err != nil && !wasDown:
		r.Log.Warn("workers: cannot read "+what+"; left out of /api/workers until it works", "err", err)
	case err == nil && wasDown:
		r.Log.Info("workers: " + what + " readable again")
	}
	return err != nil
}

// apply writes the worker Deployment of the current pod template; a new
// restartedAt rolls a new pod.
func (r *Reconciler) apply(ctx context.Context, account, image, restartedAt string) error {
	d := kube.WorkerDeployment(r.Namespace, kube.WorkerSpec{
		Account: account, Image: image, ServerURL: r.ServerURL, Version: r.Version, RestartedAt: restartedAt,
	})
	if err := r.Cluster.ApplyDeployment(ctx, d); err != nil {
		_ = r.Store.SetError(ctx, account, err.Error())
		return fmt.Errorf("apply %s: %w", d.Metadata.Name, err)
	}
	return nil
}

// Restart applies the account's worker Deployment with the current pod
// template and a new restart annotation, like `kubectl rollout restart`: the
// Deployment (Recreate) stops the pod and makes a new one, which links and
// gets a fresh bootstrap. The server's Role cannot delete pods (docs/11
// section 7). It logs the account out, so only the owner asks for it.
func (r *Reconciler) Restart(ctx context.Context, account string) error {
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return err
	}
	for _, w := range want {
		if w.Account != account || !w.Running {
			continue
		}
		image := w.Image
		if image == "" {
			image = r.Image
		}
		if err := r.apply(ctx, account, image, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		r.Kick()
		return nil
	}
	return fmt.Errorf("the worker of %s is not meant to run; start it instead", account)
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

// Usage is what the metrics API last measured of each account's pod; an
// account is missing when its pod has no metrics yet or they cannot be read.
func (r *Reconciler) Usage() map[string]Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Usage, len(r.usage))
	for k, v := range r.usage {
		out[k] = v
	}
	return out
}

// Capacity is how many more workers the namespace's quotas admit, from the
// last pass; false without a quota or when it could not be read.
func (r *Reconciler) Capacity() (Capacity, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capacity == nil {
		return Capacity{}, false
	}
	return *r.capacity, true
}

// OnConnect bootstraps a managed agent that just linked: unless its client
// already runs (the link dropped, the pod did not), it gets the account's
// session, a fresh Chronicle Link token, the world and the client profile.
// The worker's last error becomes what of the profile the client refused
// (or nothing); a relink with the client up leaves it as it was. Attached
// agents are left alone.
func (r *Reconciler) OnConnect(info agent.Info) {
	r.init()
	if info.Kind != agent.KindManaged || info.Account == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	lastError, keep, err := r.bootstrap(ctx, info)
	switch {
	case err != nil:
		r.Log.Warn("workers: bootstrap", "account", info.Account, "agent", info.ID, "err", err)
		_ = r.Store.SetError(ctx, info.Account, "bootstrap: "+err.Error())
	case !keep:
		_ = r.Store.SetError(ctx, info.Account, lastError)
	}
}

// bootstrap returns the worker's last error after it ("" or the profile's
// problems) and true when the client was already up, which keeps the last
// error as it was.
func (r *Reconciler) bootstrap(ctx context.Context, info agent.Info) (string, bool, error) {
	var st agent.Status
	if err := r.Hub.CallAgent(ctx, info.ID, agent.CmdStatus, nil, &st); err != nil {
		return "", false, fmt.Errorf("status: %w", err)
	}
	if st.Running {
		r.Log.Info("workers: agent relinked with its client up", "account", info.Account, "agent", info.ID)
		return "", true, nil
	}
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return "", false, err
	}
	world, desired := 0, false
	for _, w := range want {
		if w.Account == info.Account {
			world, desired = w.World, w.Running
		}
	}
	if !desired {
		return "", false, errors.New("the worker is not meant to run; the reconciler deletes it")
	}
	session, err := r.Store.Session(ctx, info.Account)
	if err != nil {
		return "", false, fmt.Errorf("session: %w", err)
	}
	profile, err := r.Store.Profile(ctx, info.Account)
	if err != nil {
		return "", false, fmt.Errorf("client profile: %w", err)
	}
	token, err := r.Store.MintPluginToken(ctx, info.Account)
	if err != nil {
		return "", false, fmt.Errorf("plugin token: %w", err)
	}
	params := agent.BootstrapParams{
		Account: info.Account, Session: session, PluginToken: token, ServerURL: r.ServerURL, World: world, Profile: &profile,
	}
	var res agent.BootstrapResult
	if err := r.Hub.CallAgent(ctx, info.ID, agent.CmdWorkerBootstrap, params, &res); err != nil {
		return "", false, err
	}
	if !res.Started {
		return "", false, errors.New("the client did not come up: " + truncate(res.Output, 300))
	}
	r.Log.Info("workers: bootstrapped", "account", info.Account, "agent", info.ID, "profile_problems", len(res.Problems))
	if len(res.Problems) > 0 {
		return head("profile: "+strings.Join(res.Problems, "; "), 500), false, nil
	}
	return "", false, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// head keeps the first n bytes of s (whole runes).
func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}
