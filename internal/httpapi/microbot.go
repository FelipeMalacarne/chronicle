package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	storemicrobot "github.com/FelipeMalacarne/ironman-planner/internal/store/microbot"
)

// MicrobotRole is the Zitadel project role every control route requires in
// AUTH_MODE=oidc (docs/08, Segurança item 2).
const MicrobotRole = "microbot:control"

// MicrobotAudit persists the control actions (internal/store/microbot).
type MicrobotAudit interface {
	Append(ctx context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error)
	List(ctx context.Context, limit int) ([]storemicrobot.Entry, error)
}

// MicrobotOptions turns on the Microbot control module (MICROBOT_CONTROL=on,
// docs/08). Without it no /api/microbot route exists.
type MicrobotOptions struct {
	Link  *agent.Link
	Audit MicrobotAudit
	// AgentToken is the machine token `ironman agent` presents
	// (MICROBOT_AGENT_TOKEN).
	AgentToken string
}

func (s *Server) microbotRoutes() {
	m := s.opts.Microbot
	if m == nil || !s.accountRoutes() {
		return
	}
	s.mux.HandleFunc("GET /api/microbot/agent", s.handleMicrobotAgent)
	control := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(s.microbotControl(h))) }
	control("GET /api/microbot/status", s.handleMicrobotStatus)
	control("GET /api/microbot/screenshot", s.handleMicrobotScreenshot)
	control("GET /api/microbot/audit", s.handleMicrobotAudit)
	control("POST /api/microbot/client/start", s.microbotAction(agent.CmdClientStart, 6*time.Minute))
	control("POST /api/microbot/client/stop", s.microbotAction(agent.CmdClientStop, 3*time.Minute))
	control("POST /api/microbot/login", s.microbotAction(agent.CmdLogin, 90*time.Second))
	control("POST /api/microbot/logout", s.microbotAction(agent.CmdLogout, 45*time.Second))
}

// handleMicrobotAgent is the agent's websocket (docs/08): the machine token,
// never a user session. The link holds the connection until it closes.
func (s *Server) handleMicrobotAgent(w http.ResponseWriter, r *http.Request) {
	scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	want := s.opts.Microbot.AgentToken
	if !strings.EqualFold(scheme, "Bearer") || want == "" ||
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(want)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="microbot-agent"`)
		writeError(w, http.StatusUnauthorized, "agent token required")
		return
	}
	s.opts.Microbot.Link.Serve(w, r)
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
	Dev    bool          `json:"dev"`
	Agent  agent.Info    `json:"agent"`
	Client *agent.Status `json:"client,omitempty"`
	Error  string        `json:"error,omitempty"`
}

func (s *Server) handleMicrobotStatus(w http.ResponseWriter, r *http.Request) {
	link := s.opts.Microbot.Link
	out := microbotStatus{Enabled: true, Dev: s.opts.DevMode, Agent: link.Info()}
	if out.Agent.Connected {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var st agent.Status
		if err := link.Call(ctx, agent.CmdStatus, nil, &st); err != nil {
			out.Error = err.Error()
		} else {
			out.Client = &st
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMicrobotScreenshot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var shot agent.Screenshot
	if err := s.opts.Microbot.Link.Call(ctx, agent.CmdScreenshot, nil, &shot); err != nil {
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
	entries, err := s.opts.Microbot.Audit.List(r.Context(), limit)
	if err != nil {
		s.log.Error("microbot audit", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// microbotAction is a control command from the web: it runs on the agent,
// and the attempt and its result go to the audit either way.
func (s *Server) microbotAction(command string, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params json.RawMessage
		if command == agent.CmdLogin {
			var p agent.LoginParams
			if r.ContentLength != 0 && !decodeBody(w, r, &p) {
				return
			}
			if p.World < 0 || p.World > 999 {
				writeError(w, http.StatusBadRequest, "world must be between 1 and 999")
				return
			}
			params, _ = json.Marshal(p)
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		var data json.RawMessage
		err := s.opts.Microbot.Link.Call(ctx, command, params, &data)
		actor := sessionUser(r)
		if actor == "" {
			actor = "dev"
		}
		entry := storemicrobot.Entry{Actor: actor, Origin: "web", Action: command, Params: params, OK: err == nil}
		if err != nil {
			entry.Error = err.Error()
		}
		if _, aerr := s.opts.Microbot.Audit.Append(context.WithoutCancel(r.Context()), entry); aerr != nil {
			s.log.Error("microbot audit", "err", aerr)
		}
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
		writeError(w, http.StatusServiceUnavailable, "no agent connected (run `ironman agent` on the game machine)")
	case errors.As(err, &cmd):
		writeError(w, http.StatusBadGateway, cmd.Error())
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "the agent did not answer in time")
	default:
		s.log.Error("microbot", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
