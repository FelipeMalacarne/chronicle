// Package workerctl runs the managed workers (docs/11): it makes the
// Kubernetes Deployments match the desired state in the database, bootstraps
// each worker's agent when it links, and restarts an outdated worker on the
// new image at a pause (docs/11 section 7.2). It owns no state of its own: the
// database says what should run, the cluster what does, and the hub who is
// linked; it only remembers its automatic restarts, so as not to repeat them.
package workerctl

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
)

// Desired is one worker row as the reconciler needs it.
type Desired struct {
	Account string
	Running bool
	World   int
	// Image is the image the worker was started (or restarted) with; empty
	// takes the reconciler's current one.
	Image string
	// AutoLogin and WasLoggedIn: log in again after a new pod when the
	// client was logged in (docs/11 section 15.5).
	AutoLogin, WasLoggedIn bool
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
	// SetRunningImage moves a worker that is meant to run to image and
	// reports whether it did; a stopped worker, or none, answers false.
	SetRunningImage(ctx context.Context, account, image string) (bool, error)
	// RunningPlugins are the plugins the worker's client had on at the last
	// complete read; SetRunningPlugins replaces them (docs/11 section 15.4).
	RunningPlugins(ctx context.Context, account string) ([]agent.ProfilePlugin, error)
	SetRunningPlugins(ctx context.Context, account string, plugins []agent.ProfilePlugin) error
	// RunningSettings are the option values of those plugins, as last read
	// (docs/11 section 15.6).
	RunningSettings(ctx context.Context, account string) ([]agent.ProfileSetting, error)
	SetRunningSettings(ctx context.Context, account string, settings []agent.ProfileSetting) error
	// SetLoggedIn records that the client was read logged in; RecordLogin the
	// outcome of a login the reconciler made; OtherClientSince whether
	// another client of the account pushed since then (docs/11 section 6.2).
	SetLoggedIn(ctx context.Context, account string, on bool) error
	RecordLogin(ctx context.Context, account, result string) error
	OtherClientSince(ctx context.Context, account string, since time.Time) (bool, error)
}

// otherClientWindow: a push from another installation this recent means the
// account is being played elsewhere (docs/11 section 6.2).
const otherClientWindow = 5 * time.Minute

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
	// Agents lists the connected agents.
	Agents() []agent.Info
	// VNCOpen reports whether a browser has a live VNC session of the agent
	// that declares account open on this server.
	VNCOpen(account string) bool
}

// Audit appends to the Microbot audit (internal/store/microbot).
type Audit interface {
	Append(ctx context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error)
}

// ErrNotRunning: the worker is not meant to run (stopped, or none), so there
// is nothing to restart.
var ErrNotRunning = errors.New("the worker is not running; start it instead")

// The automatic restart on a new image is audited like the owner's Reiniciar
// (action worker_restart), by this actor and origin, with this reason.
const (
	AutoRestartActor  = "reconciler"
	AutoRestartOrigin = "server"
	AutoRestartReason = "versão nova, na pausa"
)

// A failed automatic restart is tried again after autoBackoff, doubling up to
// autoBackoffMax.
const (
	autoBackoff    = 2 * time.Minute
	autoBackoffMax = time.Hour
)

// Reconciler keeps the cluster's workers equal to the database's.
type Reconciler struct {
	Store   Store
	Cluster Cluster
	Hub     Hub
	// Audit records the automatic restarts; nil records nothing.
	Audit     Audit
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
	// pass serializes the reconcile pass and the restarts, so a pass that
	// read the desired state never applies over a restart that moved it.
	pass sync.Mutex
	// now is the clock of the automatic restarts' backoff (time.Now).
	now func() time.Time

	mu       sync.Mutex
	pods     map[string]PodInfo // by account, from the last pass
	usage    map[string]Usage   // by account: the metrics of the pod in pods
	capacity *Capacity          // nil: no quota, or it could not be read
	// metricsDown and quotaDown remember a failing read, so it is logged
	// when it starts failing and when it is back, not on every pass.
	metricsDown, quotaDown bool
	// autos are the automatic restarts tried, by account.
	autos map[string]autoRestart
	// settled holds the agents (by connection id) whose client came up with
	// everything the bootstrap meant to start: only their plugin list is read
	// back (capture), so a client still booting never shrinks what is kept.
	settled map[string]bool
	// settingsAt is when each account's option values were last read: they
	// cost a request per option, so a pass reads them every settingsEvery.
	settingsAt map[string]time.Time
}

// autoRestart is what the reconciler remembers of a worker's automatic
// restarts: at most one per image, and a backoff after a failure.
type autoRestart struct {
	image    string    // the WORKER_IMAGE it was for
	done     bool      // restarted onto image: never again for it
	failures int       // failed attempts for image
	next     time.Time // no attempt before
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
		if r.now == nil {
			r.now = time.Now
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
// longer knows), refresh the pod view, and restart on the current image the
// outdated workers that are paused (restartPaused).
func (r *Reconciler) Once(ctx context.Context) error {
	r.init()
	want, err := r.reconcile(ctx)
	if want != nil {
		r.restartPaused(ctx, want)
	}
	return err
}

// reconcile is the pass of Once without the automatic restarts. It returns
// the desired state it read, nil when the pass could not run.
func (r *Reconciler) reconcile(ctx context.Context) ([]Desired, error) {
	r.pass.Lock()
	defer r.pass.Unlock()
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return nil, fmt.Errorf("desired workers: %w", err)
	}
	// Before any Deployment changes: a pod about to be replaced is read last.
	r.capture(ctx, want, false)
	have, err := r.Cluster.ListDeployments(ctx, kube.WorkerSelector)
	if err != nil {
		return nil, fmt.Errorf("list worker deployments: %w", err)
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
		// reach a worker only on a start or on a restart (the owner's, or the
		// automatic one at a pause, both through restart).
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
	return want, errors.Join(errs...)
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

// Restart is the owner's Reiniciar: the account's running worker restarts
// now, on the current image (restart), wherever the character is.
// ErrNotRunning when the worker is not meant to run.
func (r *Reconciler) Restart(ctx context.Context, account string) error {
	r.init()
	r.pass.Lock()
	_, err := r.restart(ctx, account)
	r.pass.Unlock()
	if err != nil {
		return err
	}
	r.Kick()
	return nil
}

// restart moves the account's running worker to the current image
// (WORKER_IMAGE; without one it keeps its own) and applies its Deployment with
// the current pod template and a new restart annotation, like `kubectl rollout
// restart`: the Deployment (Recreate) stops the pod, whose agent logs the
// account out (docs/11 section 6.5), and makes a new one, which links and gets
// a fresh bootstrap. The server's Role cannot delete pods (docs/11 section 7).
// The owner's Reiniciar and the automatic restart at a pause both come here,
// so they do the same. It returns the image the worker restarts on; a worker
// that is not meant to run is never started (ErrNotRunning). Under r.pass.
func (r *Reconciler) restart(ctx context.Context, account string) (string, error) {
	if r.Image != "" {
		moved, err := r.Store.SetRunningImage(ctx, account, r.Image)
		if err != nil {
			return "", err
		}
		if !moved {
			return "", ErrNotRunning
		}
	}
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return "", err
	}
	for _, w := range want {
		if w.Account != account || !w.Running {
			continue
		}
		image := cmp.Or(w.Image, r.Image)
		if image == "" {
			return "", errors.New("WORKER_IMAGE is not set on the server")
		}
		// The pod about to be replaced is read last (docs/11 sections 15.4 and 15.5).
		r.capture(ctx, []Desired{w}, true)
		return image, r.apply(ctx, account, image, r.now().UTC().Format(time.RFC3339))
	}
	return "", ErrNotRunning
}

// restartPaused is the automatic restart on a new image (docs/11 section 7.2,
// the owner's decision of 2026-11-13): every running worker whose pod runs
// another image than WORKER_IMAGE (the outdated of /api/workers) restarts on
// it when its managed agent says it is paused (Paused): no run, nobody on its
// VNC, logged out or on a Break Handler break. At most once per worker and
// image; a failed restart waits autoBackoff, doubling. A worker without a
// linked managed agent, or whose agent does not answer, waits for a later
// pass. The attached agent (zaros) has no image and is never restarted.
func (r *Reconciler) restartPaused(ctx context.Context, want []Desired) {
	if r.Image == "" || r.Hub == nil {
		return
	}
	pods := r.Pods()
	agents := r.Hub.Agents()
	now := r.now()
	running := map[string]bool{}
	for _, w := range want {
		if !w.Running {
			continue
		}
		running[w.Account] = true
		pod, ok := pods[w.Account]
		if !ok || pod.Image == "" || pod.Image == r.Image || !r.autoDue(w.Account, now) {
			continue
		}
		a, ok := managedAgent(agents, w.Account)
		if !ok {
			r.Log.Debug("workers: outdated, no agent linked", "account", w.Account)
			continue
		}
		pause, paused := Paused(r.pauseView(ctx, a))
		if !paused {
			r.Log.Debug("workers: outdated, not paused", "account", w.Account, "why", pause)
			continue
		}
		r.autoRestart(ctx, w.Account, a, pod.Image, pause)
	}
	r.mu.Lock()
	for account, st := range r.autos {
		if !running[account] || st.image != r.Image {
			delete(r.autos, account)
		}
	}
	r.mu.Unlock()
}

// managedAgent is the managed agent linked for account, the newest when the
// link still holds an older one.
func managedAgent(agents []agent.Info, account string) (agent.Info, bool) {
	var found agent.Info
	ok := false
	for _, a := range agents {
		if a.Kind == agent.KindManaged && a.Account == account {
			found, ok = a, true
		}
	}
	return found, ok
}

// pauseView asks the agent for its status and its run, the run last: it is
// what can change soonest (a routine coming due).
func (r *Reconciler) pauseView(ctx context.Context, a agent.Info) PauseView {
	v := PauseView{VNC: r.Hub.VNCOpen(a.Account)}
	if v.VNC {
		return v
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var st agent.Status
	if err := r.Hub.CallAgent(ctx, a.ID, agent.CmdStatus, nil, &st); err != nil {
		r.Log.Debug("workers: status for the restart", "account", a.Account, "agent", a.ID, "err", err)
	} else {
		v.Status = &st
	}
	if !a.Supports(agent.CmdRunStatus) {
		v.Run = &agent.RunStatus{} // an agent without runs
		return v
	}
	var run agent.RunStatus
	if err := r.Hub.CallAgent(ctx, a.ID, agent.CmdRunStatus, nil, &run); err != nil {
		r.Log.Debug("workers: run status for the restart", "account", a.Account, "agent", a.ID, "err", err)
	} else {
		v.Run = &run
	}
	return v
}

// autoDue reports whether an automatic restart of account may be tried now.
func (r *Reconciler) autoDue(account string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.autos[account]
	if !ok || st.image != r.Image {
		return true
	}
	return !st.done && !now.Before(st.next)
}

// autoRestart restarts a paused outdated worker (restart, as the owner's
// Reiniciar does) and audits it. Under r.pass it checks again what may have
// changed since the pause was read: the owner restarted or stopped the
// worker, or opened its VNC.
func (r *Reconciler) autoRestart(ctx context.Context, account string, a agent.Info, from, pause string) {
	r.pass.Lock()
	old, still, err := r.stillOutdated(ctx, account)
	if err != nil || !still || r.Hub.VNCOpen(account) {
		r.pass.Unlock()
		if err != nil {
			r.Log.Warn("workers: restart on the new image: read the desired state", "account", account, "err", err)
		}
		return
	}
	image, err := r.restart(ctx, account)
	if err != nil {
		// The worker goes back to its image: a later pass must not roll the
		// pod onto the new one outside a pause, only the next try.
		if _, rerr := r.Store.SetRunningImage(ctx, account, old); rerr != nil {
			r.Log.Warn("workers: restart on the new image: keep the old image", "account", account, "err", rerr)
		}
	}
	r.pass.Unlock()
	r.mu.Lock()
	if r.autos == nil {
		r.autos = map[string]autoRestart{}
	}
	st := r.autos[account]
	if st.image != r.Image {
		st = autoRestart{image: r.Image}
	}
	if err == nil {
		st.done = true
	} else {
		st.failures++
		st.next = r.now().Add(min(autoBackoff<<(st.failures-1), autoBackoffMax))
	}
	r.autos[account] = st
	r.mu.Unlock()
	if err != nil {
		r.Log.Warn("workers: restart on the new image failed", "account", account, "image", r.Image, "pause", pause,
			"failures", st.failures, "next", st.next, "err", err)
	} else {
		r.Log.Info("workers: restarted on the new image at a pause", "account", account, "from", from, "to", image, "pause", pause)
		r.Kick()
	}
	r.audit(ctx, account, a.ID, map[string]string{"reason": AutoRestartReason, "pause": pause, "from": from, "to": cmp.Or(image, r.Image)}, err)
}

// stillOutdated reports whether account's worker is still meant to run on
// another image than the current one in the database (a Reiniciar moves
// it), and which.
func (r *Reconciler) stillOutdated(ctx context.Context, account string) (string, bool, error) {
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return "", false, err
	}
	for _, w := range want {
		if w.Account == account {
			return w.Image, w.Running && w.Image != "" && w.Image != r.Image, nil
		}
	}
	return "", false, nil
}

// audit records an automatic restart in the Microbot audit like the owner's
// Reiniciar (action worker_restart), by the reconciler.
func (r *Reconciler) audit(ctx context.Context, account, agentID string, params map[string]string, err error) {
	if r.Audit == nil {
		return
	}
	raw, _ := json.Marshal(params)
	e := storemicrobot.Entry{
		Actor: AutoRestartActor, Origin: AutoRestartOrigin, Action: "worker_restart", Params: raw, OK: err == nil,
		AccountID: account, AgentID: agentID,
	}
	if err != nil {
		e.Error = err.Error()
	}
	if _, aerr := r.Audit.Append(context.WithoutCancel(ctx), e); aerr != nil {
		r.Log.Warn("workers: audit", "action", e.Action, "err", aerr)
	}
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
		r.settle(info.ID, true)
		return "", true, nil
	}
	want, err := r.Store.Desired(ctx)
	if err != nil {
		return "", false, err
	}
	world, desired := 0, false
	var relogin bool
	for _, w := range want {
		if w.Account == info.Account {
			world, desired = w.World, w.Running
			relogin = w.AutoLogin && w.WasLoggedIn
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
	// The plugins the owner had on (docs/11 section 15.4) start after the
	// profile's, so an entry of both starts once.
	if on, rerr := r.Store.RunningPlugins(ctx, info.Account); rerr != nil {
		r.Log.Warn("workers: running plugins", "account", info.Account, "err", rerr)
	} else {
		profile = agent.MergeProfiles(profile, agent.ClientProfile{Plugins: on})
	}
	// Their option values (docs/11 section 15.6) go over the profile's, but
	// never past what the agent takes in one profile.
	if set, rerr := r.Store.RunningSettings(ctx, info.Account); rerr != nil {
		r.Log.Warn("workers: running settings", "account", info.Account, "err", rerr)
	} else {
		merged := agent.MergeProfiles(profile, agent.ClientProfile{Settings: set})
		for len(merged.Settings) > agent.MaxProfileSettings && len(set) > 0 {
			set = set[:len(set)-1]
			merged = agent.MergeProfiles(profile, agent.ClientProfile{Settings: set})
		}
		profile = merged
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
	// A client that refused part of its profile is not read back: what it
	// failed to start would be forgotten.
	r.settle(info.ID, len(res.Problems) == 0)
	var msgs []string
	if len(res.Problems) > 0 {
		msgs = append(msgs, "profile: "+strings.Join(res.Problems, "; "))
	}
	if relogin {
		if msg := r.login(ctx, info, world); msg != "" {
			msgs = append(msgs, msg)
		}
	}
	return head(strings.Join(msgs, "; "), 500), false, nil
}

// login logs the worker in again after a new pod (docs/11 section 15.5) and
// returns what to show as the worker's error ("" when it entered). It never
// logs in over another client of the account, and never retries: a failure
// waits for the owner.
func (r *Reconciler) login(ctx context.Context, info agent.Info, world int) string {
	for _, a := range r.Hub.Agents() {
		if a.Account == info.Account && a.Kind != agent.KindManaged {
			return "auto login: the account is active on the attached agent " + a.Host
		}
	}
	busy, err := r.Store.OtherClientSince(ctx, info.Account, time.Now().Add(-otherClientWindow))
	if err != nil {
		return "auto login: " + err.Error()
	}
	if busy {
		return "auto login: another client of this account pushed in the last 5 minutes; logging in would log it out"
	}
	lctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := r.Hub.CallAgent(lctx, info.ID, agent.CmdLogin, agent.LoginParams{World: world}, &json.RawMessage{}); err != nil {
		_ = r.Store.RecordLogin(ctx, info.Account, head("auto login failed: "+err.Error(), 200))
		return "auto login: " + err.Error()
	}
	_ = r.Store.RecordLogin(ctx, info.Account, "auto login ok")
	r.Log.Info("workers: logged in again", "account", info.Account, "agent", info.ID, "world", world)
	return ""
}

func (r *Reconciler) settle(agentID string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled == nil {
		r.settled = map[string]bool{}
	}
	if ok {
		r.settled[agentID] = true
	} else {
		delete(r.settled, agentID)
	}
}

// capture reads the plugins each running worker's client has on and keeps
// them (docs/11 section 15.4), so the next pod starts them again. An agent
// that is not settled, or an answer that is not complete, keeps what was
// stored; the read is the Agent Server's own, never a write to the client.
func (r *Reconciler) capture(ctx context.Context, want []Desired, force bool) {
	agents := map[string]agent.Info{}
	live := map[string]bool{}
	for _, a := range r.Hub.Agents() {
		live[a.ID] = true
		if a.Kind == agent.KindManaged && a.Account != "" {
			agents[a.Account] = a
		}
	}
	r.mu.Lock()
	for id := range r.settled {
		if !live[id] {
			delete(r.settled, id)
		}
	}
	r.mu.Unlock()
	for _, w := range want {
		a, ok := agents[w.Account]
		if !w.Running || !ok {
			continue
		}
		r.mu.Lock()
		settled := r.settled[a.ID]
		r.mu.Unlock()
		if !settled {
			continue
		}
		if !w.WasLoggedIn {
			r.noteLogin(ctx, a, w.Account)
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		var p agent.Plugins
		err := r.Hub.CallAgent(cctx, a.ID, agent.CmdPlugins, nil, &p)
		cancel()
		if err == nil {
			var on []agent.ProfilePlugin
			if on, err = agent.ObservedPlugins(p); err == nil {
				if err = r.Store.SetRunningPlugins(ctx, w.Account, on); err == nil {
					r.captureSettings(ctx, a, w.Account, on, force)
				}
			}
		}
		if err != nil {
			r.Log.Debug("workers: running plugins not kept", "account", w.Account, "err", err)
		}
	}
}

// settingsEvery is how often a pass reads a worker's option values.
const settingsEvery = 2 * time.Minute

// maxObservedSettings bounds what is kept, well under what a profile takes.
const maxObservedSettings = 300

// captureSettings keeps the explicit option values of the plugins that are
// on (docs/11 section 15.6): the plugin's options come from the Chronicle
// Link's schema export, their values from the Agent Server. A read that
// fails anywhere keeps what was stored.
func (r *Reconciler) captureSettings(ctx context.Context, a agent.Info, account string, on []agent.ProfilePlugin, force bool) {
	r.mu.Lock()
	if r.settingsAt == nil {
		r.settingsAt = map[string]time.Time{}
	}
	if !force && time.Since(r.settingsAt[account]) < settingsEvery {
		r.mu.Unlock()
		return
	}
	r.settingsAt[account] = time.Now()
	r.mu.Unlock()
	sctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	set, err := r.readSettings(sctx, a, on)
	if err == nil {
		err = r.Store.SetRunningSettings(ctx, account, set)
	}
	if err != nil {
		r.Log.Debug("workers: running settings not kept", "account", account, "err", err)
	}
}

// readSettings reads every explicit option value of the plugins in on.
func (r *Reconciler) readSettings(ctx context.Context, a agent.Info, on []agent.ProfilePlugin) ([]agent.ProfileSetting, error) {
	out := []agent.ProfileSetting{}
	for _, pl := range on {
		var schema agent.PluginSchema
		err := r.Hub.CallAgent(ctx, a.ID, agent.CmdConfigSchema, agent.ScriptParams{ClassName: pl.Class()}, &schema)
		if err != nil {
			if strings.Contains(err.Error(), "has no config options") {
				continue
			}
			return nil, err
		}
		var keys []string
		seen := map[string]bool{}
		for _, it := range schema.Items {
			// A key the agent would refuse (a secret) fails the whole batch.
			if agent.ValidateSetting(agent.ProfileSetting{Group: schema.Group, Key: it.Key}) != nil {
				continue
			}
			if !seen[it.Key] {
				seen[it.Key] = true
				keys = append(keys, it.Key)
			}
		}
		for len(keys) > 0 {
			n := min(len(keys), 50)
			var v agent.ConfigValues
			if err := r.Hub.CallAgent(ctx, a.ID, agent.CmdConfigGet, agent.ConfigGetParams{Group: schema.Group, Keys: keys[:n]}, &v); err != nil {
				return nil, err
			}
			for k, val := range v.Values {
				if val == nil {
					continue
				}
				s := agent.ProfileSetting{Group: schema.Group, Key: k, Value: *val}
				if agent.ValidateSetting(s) == nil {
					out = append(out, s)
				}
			}
			keys = keys[n:]
		}
	}
	slices.SortFunc(out, func(x, y agent.ProfileSetting) int {
		return cmp.Or(strings.Compare(x.Group, y.Group), strings.Compare(x.Key, y.Key))
	})
	if len(out) > maxObservedSettings {
		out = out[:maxObservedSettings]
	}
	return out, nil
}

// noteLogin keeps that the client is logged in (docs/11 section 15.5).
func (r *Reconciler) noteLogin(ctx context.Context, a agent.Info, account string) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var st agent.Status
	if err := r.Hub.CallAgent(cctx, a.ID, agent.CmdStatus, nil, &st); err != nil {
		return
	}
	var v struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if (json.Unmarshal(st.State, &v) == nil && v.LoggedIn) || (json.Unmarshal(st.Login, &v) == nil && v.LoggedIn) {
		if err := r.Store.SetLoggedIn(ctx, account, true); err != nil {
			r.Log.Debug("workers: logged in not kept", "account", account, "err", err)
		}
	}
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
