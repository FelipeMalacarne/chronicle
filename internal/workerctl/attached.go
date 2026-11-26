package workerctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// Host is the host of the attached agent that runs the worker's client, ""
// when it runs in the cluster.
func (d Desired) Host() string {
	if d.Placement == "" || d.Placement == storeworkers.PlacementCluster {
		return ""
	}
	return d.Placement
}

// ServingAgent is the agent that serves a worker placed at placement: its
// managed agent in the cluster, its client in a slot of the host's agent
// otherwise.
func ServingAgent(agents []agent.Info, account, placement string) (agent.Info, bool) {
	if placement == "" || placement == storeworkers.PlacementCluster {
		return managedAgent(agents, account)
	}
	return agent.SlotClientOf(agents, placement, account)
}

// reconcileAttached fills the slot of every running worker placed on an
// attached agent and frees the slots that no running worker is placed on. An
// account whose agent is not linked waits for it: nothing is an error.
func (r *Reconciler) reconcileAttached(ctx context.Context, want []Desired) []error {
	agents := r.Hub.Agents()
	for _, w := range want {
		host := w.Host()
		if !w.Running || host == "" {
			continue
		}
		if _, ok := agent.SlotClientOf(agents, host, w.Account); ok {
			continue
		}
		pool, ok := agent.PoolOf(agents, host)
		if !ok {
			continue
		}
		switch {
		case r.PublicURL == "":
			_ = r.Store.SetError(ctx, w.Account, "PUBLIC_BASE_URL is not set on the server")
		case pool.Used >= pool.Slots:
			_ = r.Store.SetError(ctx, w.Account, fmt.Sprintf("no free slot on %s (%d of %d in use)", host, pool.Used, pool.Slots))
		default:
			r.startBootstrap(ctx, pool, w.Account)
		}
	}
	var errs []error
	for _, c := range agents {
		if !c.Slot {
			continue
		}
		if slices.ContainsFunc(want, func(w Desired) bool { return w.Account == c.Account && w.Running && w.Host() == c.Host }) {
			continue
		}
		if err := r.release(ctx, c.Host, c.Account); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// startBootstrap bootstraps account in a slot of pool in the background,
// unless one is already in flight: the bootstrap starts the client and may
// take minutes.
func (r *Reconciler) startBootstrap(ctx context.Context, pool agent.Info, account string) {
	r.mu.Lock()
	if r.booting[account] {
		r.mu.Unlock()
		return
	}
	if r.booting == nil {
		r.booting = map[string]bool{}
	}
	r.booting[account] = true
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.booting, account)
			r.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		defer cancel()
		lastError, err := r.bootstrapAttached(ctx, pool, account)
		r.recordBootstrap(ctx, account, pool.ID, lastError, false, err)
	}()
}

// bootstrapAttached sends the account's bootstrap to pool, which puts it in a
// free slot, and handles the answer for the client that appeared there. The
// Chronicle Link of that client reaches r.PublicURL.
func (r *Reconciler) bootstrapAttached(ctx context.Context, pool agent.Info, account string) (string, error) {
	plan, err := r.bootstrapPlan(ctx, account, r.PublicURL)
	if err != nil {
		return "", err
	}
	var res agent.BootstrapResult
	if err := r.Hub.CallAgent(ctx, pool.ID, agent.CmdWorkerBootstrap, plan.params, &res); err != nil {
		return "", err
	}
	client, ok := agent.SlotClientOf(r.Hub.Agents(), pool.Host, account)
	if !ok {
		return "", errors.New("the client started but did not link to the server")
	}
	return r.bootstrapped(ctx, client, plan, res)
}

// release frees the account's slot on host's agent (it logs the client out
// and stops it) and revokes the client's Chronicle Link token. Nothing to do
// without a client in a slot, or while the agent is not linked.
func (r *Reconciler) release(ctx context.Context, host, account string) error {
	agents := r.Hub.Agents()
	if _, ok := agent.SlotClientOf(agents, host, account); !ok {
		return nil
	}
	pool, ok := agent.PoolOf(agents, host)
	if !ok {
		return nil
	}
	if err := r.Hub.CallAgent(ctx, pool.ID, agent.CmdWorkerRelease, agent.ReleaseParams{Account: account}, &json.RawMessage{}); err != nil {
		return fmt.Errorf("release %s on %s: %w", account, host, err)
	}
	r.Log.Info("workers: slot released", "account", account, "host", host)
	if err := r.Store.RevokePluginToken(ctx, account); err != nil {
		return fmt.Errorf("revoke token of %s: %w", account, err)
	}
	return nil
}
