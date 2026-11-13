package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/workerctl"
)

// WorkerStore is the database side of the workers (internal/store/workers):
// the accounts' game sessions and the desired state of their pods.
type WorkerStore interface {
	PutSession(ctx context.Context, account, pushedBy string, s agent.GameSession) error
	SessionInfo(ctx context.Context, account string) (storeworkers.SessionInfo, error)
	DeleteSession(ctx context.Context, account string) error
	SetDesired(ctx context.Context, account, state string, world int, image string) error
	Get(ctx context.Context, account string) (storeworkers.Worker, error)
	List(ctx context.Context) ([]storeworkers.Worker, error)
	ListForUser(ctx context.Context, userID string) ([]storeworkers.Worker, error)
	OtherClientSince(ctx context.Context, account string, since time.Time) (bool, error)
	RecordLogin(ctx context.Context, account, result string) error
}

// WorkerControl is the cluster side (internal/workerctl.Reconciler); nil
// when WORKERS=off, where sessions can still be stored.
type WorkerControl interface {
	Kick()
	Pods() map[string]workerctl.PodInfo
	Restart(ctx context.Context, account string) error
	// Usage and Capacity come from the metrics API and the namespace's
	// ResourceQuotas (docs/11, F7); both may be missing.
	Usage() map[string]workerctl.Usage
	Capacity() (workerctl.Capacity, bool)
}

// otherClientWindow: a push from another installation this recent means the
// account is being played elsewhere (docs/11 section 6.2).
const otherClientWindow = 5 * time.Minute

func (s *Server) workerRoutes() {
	m := s.opts.Microbot
	if m == nil || m.Sessions == nil {
		return
	}
	control := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(s.microbotControl(h))) }
	control("GET /api/workers", s.handleWorkers)
	control("POST /api/workers/start", s.handleWorkerStart)
	control("POST /api/workers/stop", s.handleWorkerStop)
	control("POST /api/workers/restart", s.handleWorkerRestart)
	control("GET /api/accounts/session", s.handleSessionGet)
	control("PUT /api/accounts/session", s.handleSessionPut)
	control("DELETE /api/accounts/session", s.handleSessionDelete)
}

type workerView struct {
	Account   string                    `json:"account"`
	RSN       string                    `json:"rsn"`
	Session   *storeworkers.SessionInfo `json:"session"`
	Desired   string                    `json:"desired"`
	World     int                       `json:"world,omitempty"`
	Image     string                    `json:"image,omitempty"`
	StartedAt *time.Time                `json:"started_at,omitempty"`
	StoppedAt *time.Time                `json:"stopped_at,omitempty"`
	LastError string                    `json:"last_error,omitempty"`
	Pod       *workerctl.PodInfo        `json:"pod"`
	// Usage is the pod's CPU and memory from the metrics API; absent with
	// WORKERS=off, without a pod, or without metrics-server.
	Usage *workerctl.Usage `json:"usage,omitempty"`
	Agent *agent.Info      `json:"agent"`
	// Outdated: the pod runs another image than the server's WORKER_IMAGE
	// (restarting logs the account out, so only on the owner's click).
	Outdated bool `json:"outdated"`
}

type workersAnswer struct {
	// Enabled: the server runs workers (WORKERS=on); off, it only stores
	// sessions.
	Enabled bool   `json:"enabled"`
	Image   string `json:"image,omitempty"`
	Version string `json:"version,omitempty"`
	// Workers are the managed accounts; Attached the other agents the user
	// may see (the one on zaros).
	Workers  []workerView `json:"workers"`
	Attached []agent.Info `json:"attached"`
	// Capacity is how many more workers fit in the namespace's quota;
	// absent with WORKERS=off, without a quota or when it cannot be read.
	Capacity *workerctl.Capacity `json:"capacity,omitempty"`
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	m := s.opts.Microbot
	var (
		list []storeworkers.Worker
		err  error
	)
	if s.opts.DevMode {
		list, err = m.Sessions.List(r.Context())
	} else {
		list, err = m.Sessions.ListForUser(r.Context(), sessionUser(r))
	}
	if err != nil {
		s.log.Error("workers: list", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list the workers")
		return
	}
	out := workersAnswer{Enabled: m.Control != nil, Image: m.WorkerImage, Version: s.opts.Version, Workers: []workerView{}, Attached: []agent.Info{}}
	var (
		pods  map[string]workerctl.PodInfo
		usage map[string]workerctl.Usage
	)
	if m.Control != nil {
		pods, usage = m.Control.Pods(), m.Control.Usage()
		if c, ok := m.Control.Capacity(); ok {
			out.Capacity = &c
		}
	}
	agents := s.visibleAgents(r, m.Link.Agents())
	managed := map[string]bool{}
	for _, wk := range list {
		v := workerView{
			Account: wk.AccountID, RSN: wk.RSN, Session: wk.Session, Desired: wk.DesiredState, World: wk.World,
			Image: wk.Image, StartedAt: wk.StartedAt, StoppedAt: wk.StoppedAt, LastError: wk.LastError,
		}
		if p, ok := pods[wk.AccountID]; ok {
			v.Pod = &p
			v.Outdated = m.WorkerImage != "" && p.Image != "" && p.Image != m.WorkerImage
			if u, ok := usage[wk.AccountID]; ok {
				v.Usage = &u
			}
		}
		for i := range agents {
			if agents[i].Account == wk.AccountID && agents[i].Kind == agent.KindManaged {
				a := agents[i]
				v.Agent = &a
			}
		}
		managed[wk.AccountID] = true
		out.Workers = append(out.Workers, v)
	}
	for _, a := range agents {
		if a.Kind != agent.KindManaged {
			out.Attached = append(out.Attached, a)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type workerStartBody struct {
	World int `json:"world,omitempty"`
}

func (s *Server) handleWorkerStart(w http.ResponseWriter, r *http.Request) {
	m := s.opts.Microbot
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	if m.Control == nil {
		writeError(w, http.StatusServiceUnavailable, "workers are off on this server (WORKERS=off)")
		return
	}
	var body workerStartBody
	if r.ContentLength != 0 && !decodeBody(w, r, &body) {
		return
	}
	if body.World != 0 && (body.World < 301 || body.World > 999) {
		writeError(w, http.StatusBadRequest, "world must be between 301 and 999")
		return
	}
	// One account in one place (docs/11 section 6.2).
	for _, a := range m.Link.Agents() {
		if a.Account == account && a.Kind != agent.KindManaged {
			s.workerRefused(w, r, "worker_start", account, "the account is active on the attached agent "+a.Host+"; stop it there first")
			return
		}
	}
	busy, err := m.Sessions.OtherClientSince(r.Context(), account, time.Now().Add(-otherClientWindow))
	if err != nil {
		s.log.Error("workers: other client", "err", err)
		writeError(w, http.StatusInternalServerError, "could not check the account's other clients")
		return
	}
	if busy {
		s.workerRefused(w, r, "worker_start", account, "another client of this account pushed in the last 5 minutes (someone is playing it); starting would log it out")
		return
	}
	image := ""
	if cur, gerr := m.Sessions.Get(r.Context(), account); gerr != nil || cur.DesiredState != storeworkers.StateRunning {
		image = m.WorkerImage // a fresh start takes the current image
	}
	err = m.Sessions.SetDesired(r.Context(), account, storeworkers.StateRunning, body.World, image)
	s.auditWorker(r, "worker_start", account, err)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	m.Control.Kick()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleWorkerStop(w http.ResponseWriter, r *http.Request) {
	m := s.opts.Microbot
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	if m.Control == nil {
		writeError(w, http.StatusServiceUnavailable, "workers are off on this server (WORKERS=off)")
		return
	}
	err := m.Sessions.SetDesired(r.Context(), account, storeworkers.StateStopped, 0, "")
	s.auditWorker(r, "worker_stop", account, err)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	m.Control.Kick()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleWorkerRestart moves a running worker to the server's current image:
// a new image rolls the Deployment; the same image rolls it by a restart
// annotation. Either way the account is logged out, so the web asks first.
func (s *Server) handleWorkerRestart(w http.ResponseWriter, r *http.Request) {
	m := s.opts.Microbot
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	if m.Control == nil {
		writeError(w, http.StatusServiceUnavailable, "workers are off on this server (WORKERS=off)")
		return
	}
	cur, err := m.Sessions.Get(r.Context(), account)
	if err == nil && cur.DesiredState != storeworkers.StateRunning {
		err = errors.New("the worker is not running; start it instead")
		s.auditWorker(r, "worker_restart", account, err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err == nil {
		err = m.Sessions.SetDesired(r.Context(), account, storeworkers.StateRunning, cur.World, m.WorkerImage)
	}
	if err == nil && (m.WorkerImage == "" || cur.Image == m.WorkerImage) {
		err = m.Control.Restart(r.Context(), account)
	}
	s.auditWorker(r, "worker_restart", account, err)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	m.Control.Kick()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	info, err := s.opts.Microbot.Sessions.SessionInfo(r.Context(), account)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, info)
}

// handleSessionPut stores the account's game session (`chronicle accounts
// connect`). The session is write-only: no route ever answers it back.
func (s *Server) handleSessionPut(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var body agent.GameSession
	if !decodeBody(w, r, &body) {
		return
	}
	actor := sessionUser(r)
	if actor == "" {
		actor = "dev"
	}
	err := s.opts.Microbot.Sessions.PutSession(r.Context(), account, actor, body)
	s.auditWorker(r, "session_put", account, err)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	info, err := s.opts.Microbot.Sessions.SessionInfo(r.Context(), account)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	err := s.opts.Microbot.Sessions.DeleteSession(r.Context(), account)
	s.auditWorker(r, "session_delete", account, err)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// recordWorkerLogin keeps the outcome of a login on a managed worker, which
// logs in with the account's stored session (docs/11 section 5): LoginOK, or
// the game's reason when the login did not get in (agent.LoginRefusal). The
// session's login_failed flag comes from it. Other failures (no agent, a
// login in progress, the cooldown) say nothing about the session and are not
// recorded; an attached agent uses the launcher's session, not the stored one.
func (s *Server) recordWorkerLogin(r *http.Request, served agent.Info, err error) {
	m := s.opts.Microbot
	if m.Sessions == nil || served.Kind != agent.KindManaged || served.Account == "" {
		return
	}
	result := storeworkers.LoginOK
	if err != nil {
		reason, refused := agent.LoginRefusal(err)
		if !refused {
			return
		}
		result = reason
	}
	rerr := m.Sessions.RecordLogin(context.WithoutCancel(r.Context()), served.Account, result)
	if rerr != nil && !errors.Is(rerr, storeworkers.ErrNoSession) {
		s.log.Error("workers: record login", "account", served.Account, "err", rerr)
	}
}

func (s *Server) workerRefused(w http.ResponseWriter, r *http.Request, action, account, msg string) {
	s.auditWorker(r, action, account, errors.New(msg))
	writeError(w, http.StatusConflict, msg)
}

// auditWorker records a worker or session action in the Microbot audit,
// never with the session's content.
func (s *Server) auditWorker(r *http.Request, action, account string, err error) {
	actor := sessionUser(r)
	if actor == "" {
		actor = "dev"
	}
	e := entryFor(actor, action, account, err)
	if _, aerr := s.opts.Microbot.Audit.Append(context.WithoutCancel(r.Context()), e); aerr != nil {
		s.log.Error("microbot audit", "err", aerr)
	}
}

func (s *Server) writeWorkerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storeworkers.ErrNotFound):
		writeError(w, http.StatusNotFound, "no session or worker for this account (run `chronicle accounts connect`)")
	case errors.Is(err, storeworkers.ErrNoSession):
		writeError(w, http.StatusConflict, "the account has no game session yet (run `chronicle accounts connect` on the launcher's machine)")
	case errors.Is(err, storeworkers.ErrWorkerRunning):
		writeError(w, http.StatusConflict, "stop the worker first")
	case errors.Is(err, storeworkers.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, storeworkers.ErrKeyMismatch):
		writeError(w, http.StatusConflict, "the session was stored with another vault key: connect the account again")
	default:
		s.log.Error("workers", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func entryFor(actor, action, account string, err error) storemicrobot.Entry {
	e := storemicrobot.Entry{Actor: actor, Origin: "web", Action: action, OK: err == nil, AccountID: account}
	if err != nil {
		e.Error = err.Error()
	}
	return e
}
