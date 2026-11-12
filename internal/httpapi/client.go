package httpapi

import "net/http"

// authClientResponse is GET /api/auth/client: how a native client (the CLI,
// `chronicle mcp`, later the plugin) logs in to this server. In dev mode it
// needs nothing; in oidc mode it runs the device flow (RFC 8628) at Issuer
// with ClientID and Scopes and sends the access token as a bearer. Without
// ClientID the server takes no bearer tokens (only the web session).
type authClientResponse struct {
	AuthMode string   `json:"auth_mode"`
	Issuer   string   `json:"issuer,omitempty"`
	ClientID string   `json:"client_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	// AgentScopes are the scopes of the Microbot agent's machine user in the
	// client_credentials grant (docs/08); only with MICROBOT_CONTROL=on.
	AgentScopes []string `json:"agent_scopes,omitempty"`
}

func (s *Server) handleAuthClient(w http.ResponseWriter, _ *http.Request) {
	out := authClientResponse{AuthMode: "none"}
	switch {
	case s.opts.DevMode:
		out.AuthMode = "dev"
	case s.opts.OIDC != nil:
		p := s.opts.OIDC.Provider
		out.AuthMode, out.Issuer = "oidc", p.Issuer()
		if p.NativeClientID() != "" {
			out.ClientID, out.Scopes = p.NativeClientID(), p.DeviceScopes()
		}
		if s.opts.Microbot != nil {
			out.AgentScopes = p.MachineScopes()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRoute is GET /api/route?account=&limit=: the window of the
// account's active route (the plugin panel's, panel.Window), for the CLI's
// `now` and `chronicle mcp`.
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	limit, ok := windowLimit(w, r)
	if !ok {
		return
	}
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	s.writeRouteWindow(w, r, account, limit)
}
