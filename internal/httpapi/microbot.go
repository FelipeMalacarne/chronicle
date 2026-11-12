package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	storemicrobot "github.com/FelipeMalacarne/ironman-planner/internal/store/microbot"
)

// MicrobotRole is the Zitadel project role every control route requires in
// AUTH_MODE=oidc (docs/08, Segurança item 2).
const MicrobotRole = "microbot:control"

// MicrobotAgentRole is the project role of the agent's machine user in
// AUTH_MODE=oidc; people never hold it.
const MicrobotAgentRole = "microbot:agent"

// MicrobotAudit persists the control actions (internal/store/microbot).
type MicrobotAudit interface {
	Append(ctx context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error)
	List(ctx context.Context, limit int, account string) ([]storemicrobot.Entry, error)
}

// MicrobotOptions turns on the Microbot control module (MICROBOT_CONTROL=on,
// docs/08). Without it no /api/microbot route exists.
type MicrobotOptions struct {
	Link  *agent.Link
	Audit MicrobotAudit
	// Workers verifies the token of a managed worker's pod (docs/11 section
	// 6.3); nil takes no pod tokens.
	Workers WorkerTokenVerifier
	// Sessions stores the accounts' game sessions and the workers' desired
	// state (CHRONICLE_VAULT_KEY set); nil leaves /api/workers and
	// /api/accounts/session out.
	Sessions WorkerStore
	// Control runs the workers in the cluster (WORKERS=on); nil only stores
	// sessions.
	Control WorkerControl
	// WorkerImage is WORKER_IMAGE, the image a start or restart takes.
	WorkerImage string
}

// WorkerIdentity is the pod a worker token belongs to and the account the
// pod serves.
type WorkerIdentity struct {
	Pod     string
	Account string
}

// WorkerTokenVerifier checks a worker pod's token (in the cluster, a
// TokenReview of its projected service-account token).
type WorkerTokenVerifier interface {
	VerifyWorkerToken(ctx context.Context, token string) (WorkerIdentity, error)
}

func (s *Server) microbotRoutes() {
	m := s.opts.Microbot
	if m == nil || !s.accountRoutes() {
		return
	}
	s.mux.HandleFunc("GET /api/microbot/agent", s.agentAuth(s.opts.Microbot.Link.Serve))
	s.mux.HandleFunc("GET /api/microbot/agent/vnc", s.agentAuth(s.opts.Microbot.Link.ServeVNC))
	control := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(s.microbotControl(h))) }
	control("GET /api/microbot/status", s.handleMicrobotStatus)
	control("GET /api/microbot/screenshot", s.handleMicrobotScreenshot)
	control("GET /api/microbot/audit", s.handleMicrobotAudit)
	control("GET /api/microbot/vnc", s.handleMicrobotVNC)
	control("POST /api/microbot/client/start", s.microbotAction(agent.CmdClientStart, 6*time.Minute))
	control("POST /api/microbot/client/stop", s.microbotAction(agent.CmdClientStop, 3*time.Minute))
	control("POST /api/microbot/login", s.microbotAction(agent.CmdLogin, 90*time.Second))
	control("POST /api/microbot/logout", s.microbotAction(agent.CmdLogout, 45*time.Second))
	// Plugins, scripts and config (docs/08 phase 3).
	control("GET /api/microbot/plugins", s.microbotRead(agent.CmdPlugins, func(*http.Request) any { return nil }))
	control("GET /api/microbot/game", s.microbotRead(agent.CmdGameView, func(*http.Request) any { return nil }))
	control("GET /api/microbot/timers", s.microbotRead(agent.CmdTimers, func(*http.Request) any { return nil }))
	// Fazer agora (docs/08 phase 4): one run, driven by the agent; start and cancel are audited.
	control("GET /api/microbot/run", s.microbotRead(agent.CmdRunStatus, func(*http.Request) any { return nil }))
	control("GET /api/microbot/runs", s.microbotRead(agent.CmdRunHistory, func(*http.Request) any { return nil }))
	control("POST /api/microbot/run", s.microbotAction(agent.CmdRunStart, 30*time.Second))
	control("POST /api/microbot/run/cancel", s.microbotAction(agent.CmdRunCancel, 30*time.Second))
	control("GET /api/microbot/triggers", s.microbotRead(agent.CmdTriggerGet, func(*http.Request) any { return nil }))
	control("POST /api/microbot/triggers", s.microbotAction(agent.CmdTriggerSet, 30*time.Second))
	control("GET /api/microbot/client/logs", s.microbotRead(agent.CmdClientLogs, func(r *http.Request) any {
		n, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		return agent.LogsParams{Lines: n}
	}))
	control("GET /api/microbot/account", s.microbotRead(agent.CmdAccountShow, func(*http.Request) any { return nil }))
	control("POST /api/microbot/account", s.microbotAction(agent.CmdAccountUse, 90*time.Second))
	control("POST /api/microbot/world", s.microbotAction(agent.CmdWorldHop, 120*time.Second))
	control("GET /api/microbot/hub", s.microbotRead(agent.CmdHubSearch, func(r *http.Request) any {
		return agent.HubParams{Q: r.URL.Query().Get("q")}
	}))
	control("GET /api/microbot/scripts/detail", s.microbotRead(agent.CmdScriptDetail, func(r *http.Request) any {
		return agent.ScriptParams{ClassName: r.URL.Query().Get("className")}
	}))
	control("GET /api/microbot/config/schema", s.microbotRead(agent.CmdConfigSchema, func(r *http.Request) any {
		return agent.ScriptParams{ClassName: r.URL.Query().Get("className")}
	}))
	control("GET /api/microbot/config", s.microbotRead(agent.CmdConfigGet, func(r *http.Request) any {
		var keys []string
		for k := range strings.SplitSeq(r.URL.Query().Get("keys"), ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
		return agent.ConfigGetParams{Group: r.URL.Query().Get("group"), Keys: keys}
	}))
	control("POST /api/microbot/plugins/install", s.microbotAction(agent.CmdPluginInstall, 160*time.Second))
	control("POST /api/microbot/plugins/remove", s.microbotAction(agent.CmdPluginRemove, 100*time.Second))
	control("POST /api/microbot/scripts/start", s.microbotAction(agent.CmdScriptStart, 40*time.Second))
	control("POST /api/microbot/scripts/stop", s.microbotAction(agent.CmdScriptStop, 40*time.Second))
	control("POST /api/microbot/scripts/pause", s.microbotAction(agent.CmdScriptsPause, 20*time.Second))
	control("POST /api/microbot/scripts/resume", s.microbotAction(agent.CmdScriptsResume, 20*time.Second))
	control("POST /api/microbot/config", s.microbotAction(agent.CmdConfigSet, 20*time.Second))
	// Game actions (docs/08): POST /api/microbot/game/<command>.
	for _, cmd := range agent.GameActions {
		control("POST /api/microbot/game/"+cmd, s.microbotAction(cmd, 30*time.Second))
	}
}

// microbotRead is a read through the agent (not audited): params come from
// the query and are validated before the agent sees them.
func (s *Server) microbotRead(command string, params func(*http.Request) any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, ok := s.microbotAccount(w, r)
		if !ok {
			return
		}
		raw, err := json.Marshal(params(r))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err = agent.ValidateParams(command, raw); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		var data json.RawMessage
		if err = s.opts.Microbot.Link.Call(ctx, account, command, json.RawMessage(raw), &data); err != nil {
			s.writeMicrobotError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

// agentAuth guards the agent's websockets (docs/08), the link and the
// per-session VNC streams; never a user session. In dev mode the agent links
// from loopback with no token. In oidc mode it presents a Zitadel machine
// user's JWT access token: audience the project, role microbot:agent. A
// worker's pod (docs/11) presents its own token, checked by
// MicrobotOptions.Workers when it is not a machine user's (or, in dev mode,
// when the peer is not loopback); the pod's identity is bound to its
// account.
func (s *Server) agentAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var workers WorkerTokenVerifier
		if s.opts.Microbot != nil {
			workers = s.opts.Microbot.Workers
		}
		scheme, raw, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		token := strings.TrimSpace(raw)
		if !strings.EqualFold(scheme, "Bearer") {
			token = ""
		}
		if s.opts.DevMode {
			if isLoopback(r.RemoteAddr) {
				h(w, r.WithContext(agent.WithIdentity(r.Context(), agent.Identity{Subject: "dev"})))
				return
			}
			if workers == nil || token == "" {
				writeError(w, http.StatusForbidden, "in dev mode the agent links only from this machine")
				return
			}
			s.workerAgent(w, r, workers, token, h)
			return
		}
		if token == "" || (s.opts.OIDC == nil && workers == nil) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="microbot-agent"`)
			writeError(w, http.StatusUnauthorized, "agent token required (a Zitadel machine user's access token)")
			return
		}
		if s.opts.OIDC != nil {
			id, err := s.opts.OIDC.Provider.VerifyMachineToken(r.Context(), token)
			if err == nil {
				if !slices.Contains(id.Roles, MicrobotAgentRole) {
					writeError(w, http.StatusForbidden, "the agent's machine user needs the "+MicrobotAgentRole+" role")
					return
				}
				h(w, r.WithContext(agent.WithIdentity(r.Context(), agent.Identity{Subject: id.Subject})))
				return
			}
		}
		if workers != nil {
			s.workerAgent(w, r, workers, token, h)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="microbot-agent", error="invalid_token"`)
		writeError(w, http.StatusUnauthorized, "invalid agent token")
	}
}

// workerAgent authenticates a worker pod's token: its identity is
// pod:<name>, bound to the pod's account.
func (s *Server) workerAgent(w http.ResponseWriter, r *http.Request, workers WorkerTokenVerifier, token string, h http.HandlerFunc) {
	wid, err := workers.VerifyWorkerToken(r.Context(), token)
	if err != nil || wid.Pod == "" || wid.Account == "" {
		if err != nil {
			s.log.Warn("microbot: worker token refused", "err", err)
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="microbot-agent", error="invalid_token"`)
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}
	id := agent.Identity{Subject: "pod:" + wid.Pod, BoundAccount: wid.Account, Pod: wid.Pod}
	h(w, r.WithContext(agent.WithIdentity(r.Context(), id)))
}

// microbotAccount is the optional ?account= of a control route: present, it
// goes through accountParam (the session user's own account in oidc, rule
// 10); absent, the account is "" and the Hub picks the only agent.
func (s *Server) microbotAccount(w http.ResponseWriter, r *http.Request) (string, bool) {
	if strings.TrimSpace(r.URL.Query().Get("account")) == "" {
		return "", true
	}
	return s.accountParam(w, r)
}

// microbotControl adds the module's own checks to an authed route: in dev
// mode only a loopback peer (the server is not meant to be reachable), in
// oidc mode the microbot:control role.
func (s *Server) microbotControl(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.opts.DevMode && !isLoopback(r.RemoteAddr) {
			writeError(w, http.StatusForbidden, "Microbot control in dev mode only from this machine")
			return
		}
		if !s.opts.DevMode && !slices.Contains(requestRoles(r), MicrobotRole) {
			writeError(w, http.StatusForbidden, "the "+MicrobotRole+" role is required")
			return
		}
		h(w, r)
	}
}

type microbotStatus struct {
	Enabled bool `json:"enabled"`
	// Dev: AUTH_MODE=dev, where control answers only this machine.
	Dev bool `json:"dev"`
	// Account is the ?account= the status was asked for ("" for none).
	Account string `json:"account,omitempty"`
	// Agent is the agent that serves the account (or the only agent);
	// {"connected": false} when none does.
	Agent  agent.Info    `json:"agent"`
	Client *agent.Status `json:"client,omitempty"`
	Error  string        `json:"error,omitempty"`
	// Agents are every connected agent the caller may see.
	Agents []agent.Info `json:"agents"`
}

func (s *Server) handleMicrobotStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := s.microbotAccount(w, r)
	if !ok {
		return
	}
	link := s.opts.Microbot.Link
	out := microbotStatus{Enabled: true, Dev: s.opts.DevMode, Account: account, Agents: s.visibleAgents(r, link.Agents())}
	info, err := link.Route(account)
	switch {
	case err == nil:
		out.Agent = info
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var st agent.Status
		if err := link.CallAgent(ctx, info.ID, agent.CmdStatus, nil, &st); err != nil {
			out.Error = err.Error()
		} else {
			out.Client = &st
		}
	case errors.Is(err, agent.ErrAmbiguousAgent):
		out.Error = err.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// visibleAgents keeps, in oidc mode, the agents that declare none of the
// accounts or one the session user owns.
func (s *Server) visibleAgents(r *http.Request, agents []agent.Info) []agent.Info {
	if s.opts.DevMode {
		return agents
	}
	uid := sessionUser(r)
	out := make([]agent.Info, 0, len(agents))
	for _, a := range agents {
		if a.Account == "" {
			out = append(out, a)
			continue
		}
		if uid == "" || s.opts.Identity == nil {
			continue
		}
		if ok, err := s.opts.Identity.OwnsAccount(r.Context(), uid, a.Account); err == nil && ok {
			out = append(out, a)
		}
	}
	return out
}

func (s *Server) handleMicrobotScreenshot(w http.ResponseWriter, r *http.Request) {
	account, ok := s.microbotAccount(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var shot agent.Screenshot
	if err := s.opts.Microbot.Link.Call(ctx, account, agent.CmdScreenshot, nil, &shot); err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	w.Header().Set("Content-Type", shot.Type)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shot.Image)
}

func (s *Server) handleMicrobotAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	account, ok := s.microbotAccount(w, r)
	if !ok {
		return
	}
	entries, err := s.opts.Microbot.Audit.List(r.Context(), limit, account)
	if err != nil {
		s.log.Error("microbot audit", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// handleMicrobotVNC is the browser's live VNC (docs/08 phase 2): a websocket
// of raw RFB bytes, spliced through the agent to the harness's x11vnc. The
// websocket library checks that Origin matches Host, so another site cannot
// open it with the owner's cookie. Opening a session goes to the audit: it
// hands the mouse and keyboard of the game to the browser.
func (s *Server) handleMicrobotVNC(w http.ResponseWriter, r *http.Request) {
	account, ok := s.microbotAccount(w, r)
	if !ok {
		return
	}
	link := s.opts.Microbot.Link
	info, err := link.Route(account)
	if err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(-1)
	ctx := r.Context()
	stream, err := link.OpenVNCAgent(ctx, info.ID)
	s.auditMicrobot(r, account, info, agent.CmdVNCOpen, nil, err)
	if err != nil {
		_ = conn.Close(websocket.StatusTryAgainLater, truncateReason(err.Error()))
		return
	}
	agent.Splice(websocket.NetConn(ctx, conn, websocket.MessageBinary), stream)
}

// truncateReason fits a websocket close reason (125 bytes per frame).
func truncateReason(s string) string {
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// auditMicrobot appends one control action from the web, whatever its
// result, with the account it went to (the ?account=, else the account the
// agent declares) and the agent that served it (empty when none did); a
// failed write is logged, not returned.
func (s *Server) auditMicrobot(r *http.Request, account string, served agent.Info, command string, params json.RawMessage, err error) {
	actor := sessionUser(r)
	if actor == "" {
		actor = "dev"
	}
	if account == "" {
		account = served.Account
	}
	entry := storemicrobot.Entry{
		Actor: actor, Origin: "web", Action: command, Params: params, OK: err == nil,
		AccountID: account, AgentID: served.ID,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if _, aerr := s.opts.Microbot.Audit.Append(context.WithoutCancel(r.Context()), entry); aerr != nil {
		s.log.Error("microbot audit", "err", aerr)
	}
}

// microbotAction is a control command from the web: it runs on the agent,
// and the attempt and its result go to the audit either way.
func (s *Server) microbotAction(command string, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, ok := s.microbotAccount(w, r)
		if !ok {
			return
		}
		var params json.RawMessage
		switch command {
		case agent.CmdLogin:
			var p agent.LoginParams
			if r.ContentLength != 0 && !decodeBody(w, r, &p) {
				return
			}
			if p.World < 0 || p.World > 999 {
				writeError(w, http.StatusBadRequest, "world must be between 1 and 999")
				return
			}
			params, _ = json.Marshal(p)
		case agent.CmdClientStart, agent.CmdClientStop, agent.CmdLogout:
		default:
			// Plugin commands: the agent's own validation, before anything
			// reaches it; the params that pass are the ones audited.
			if r.ContentLength != 0 && !decodeBody(w, r, &params) {
				return
			}
			p, err := agent.ValidateParams(command, params)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			params = nil
			if p != nil {
				params, _ = json.Marshal(p)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		var data json.RawMessage
		link := s.opts.Microbot.Link
		served, err := link.Route(account)
		if err == nil {
			err = link.CallAgent(ctx, served.ID, command, params, &data)
		}
		s.auditMicrobot(r, account, served, command, params, err)
		if err != nil {
			s.writeMicrobotError(w, err)
			return
		}
		if len(data) == 0 {
			data = json.RawMessage("{}")
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": data})
	}
}

func (s *Server) writeMicrobotError(w http.ResponseWriter, err error) {
	var cmd *agent.CommandError
	switch {
	case errors.Is(err, agent.ErrNoAgent):
		writeError(w, http.StatusServiceUnavailable, "no agent connected (run `chronicle agent` on the game machine)")
	case errors.Is(err, agent.ErrAmbiguousAgent):
		writeError(w, http.StatusConflict, "more than one agent connected: pass ?account= to say which")
	case errors.As(err, &cmd):
		writeError(w, http.StatusBadGateway, cmd.Error())
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "the agent did not answer in time")
	default:
		s.log.Error("microbot", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
