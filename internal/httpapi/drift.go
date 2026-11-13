package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// The profile against the running client (docs/11 section 4.3, F5 part 2):
// the drift, saving the client's values (to the account or to its profile)
// and applying the profile to the client. Everything goes through the
// commands every agent has (config_get, plugins, config_set, plugin_install,
// script_start), so an attached agent from before F5 serves it too.

// configGetBatch is how many keys one config_get reads (agent.ValidateParams).
const configGetBatch = 50

func (s *Server) driftRoutes() {
	m := s.opts.Microbot
	if m == nil || m.Profiles == nil {
		return
	}
	control := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(s.microbotControl(h))) }
	control("GET /api/accounts/client/drift", s.handleClientDrift)
	control("POST /api/accounts/client/drift/save", s.handleClientDriftSave)
	control("POST /api/accounts/client/apply", s.handleClientApply)
}

type driftAnswer struct {
	Account   string    `json:"account"`
	Profile   string    `json:"profile,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	agent.Drift
}

// clientState reads from the account's agent what the client has of the
// profile's settings and plugins.
func (s *Server) clientState(ctx context.Context, served agent.Info, p agent.ClientProfile) (agent.ClientState, error) {
	link := s.opts.Microbot.Link
	st := agent.ClientState{Values: map[string]map[string]*string{}}
	for _, q := range agent.SettingGroups(p.Settings, configGetBatch) {
		var v agent.ConfigValues
		if err := link.CallAgent(ctx, served.ID, agent.CmdConfigGet, q, &v); err != nil {
			return st, err
		}
		if st.Values[q.Group] == nil {
			st.Values[q.Group] = map[string]*string{}
		}
		for k, val := range v.Values {
			st.Values[q.Group][k] = val
		}
	}
	if len(p.Plugins) > 0 {
		var pl agent.Plugins
		if err := link.CallAgent(ctx, served.ID, agent.CmdPlugins, nil, &pl); err != nil {
			return st, err
		}
		st.Active, st.Installed = agent.ActiveScripts(pl.Scripts), agent.InstalledPlugins(pl.Installed)
	}
	return st, nil
}

// driftTarget is the account's profile and the agent that serves it.
func (s *Server) driftTarget(w http.ResponseWriter, r *http.Request) (storeworkers.AccountClient, agent.Info, bool) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return storeworkers.AccountClient{}, agent.Info{}, false
	}
	ac, err := s.opts.Microbot.Profiles.AccountClient(r.Context(), account)
	if err != nil {
		s.writeProfileError(w, err)
		return ac, agent.Info{}, false
	}
	served, err := s.opts.Microbot.Link.Route(account)
	if err != nil {
		s.writeMicrobotError(w, err)
		return ac, served, false
	}
	return ac, served, true
}

func (s *Server) handleClientDrift(w http.ResponseWriter, r *http.Request) {
	ac, served, ok := s.driftTarget(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	st, err := s.clientState(ctx, served, ac.Effective)
	if err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, driftAnswer{
		Account: ac.AccountID, Profile: ac.Profile, CheckedAt: time.Now().UTC(), Drift: agent.ProfileDrift(ac.Effective, st),
	})
}

// settingKey names one setting.
type settingKey struct {
	Group string `json:"group"`
	Key   string `json:"key"`
}

// driftSaveBody is POST /api/accounts/client/drift/save: which settings of
// the profile to take from the client, and where to keep them: "account"
// (its own settings, over the profile) or "profile" (the account's profile,
// which other accounts may share).
type driftSaveBody struct {
	To   string       `json:"to"`
	Keys []settingKey `json:"keys"`
}

type driftSaveAnswer struct {
	storeworkers.AccountClient
	// Skipped are the keys unset in the client: nothing to save.
	Skipped []settingKey `json:"skipped"`
}

func (s *Server) handleClientDriftSave(w http.ResponseWriter, r *http.Request) {
	ac, served, ok := s.driftTarget(w, r)
	if !ok {
		return
	}
	var body driftSaveBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.To != "account" && body.To != "profile" {
		writeError(w, http.StatusBadRequest, `to must be "account" or "profile"`)
		return
	}
	if body.To == "profile" && ac.Profile == "" {
		writeError(w, http.StatusConflict, "the account has no profile: save to the account")
		return
	}
	if len(body.Keys) == 0 {
		writeError(w, http.StatusBadRequest, "keys is empty")
		return
	}
	var wanted []agent.ProfileSetting
	for _, k := range body.Keys {
		if err := agent.ValidateSetting(agent.ProfileSetting{Group: k.Group, Key: k.Key}); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		wanted = append(wanted, agent.ProfileSetting{Group: k.Group, Key: k.Key})
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	st, err := s.clientState(ctx, served, agent.ClientProfile{Settings: wanted})
	if err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	out := driftSaveAnswer{Skipped: []settingKey{}}
	var taken []agent.ProfileSetting
	for _, k := range body.Keys {
		v := st.Values[k.Group][k.Key]
		if v == nil {
			out.Skipped = append(out.Skipped, k)
			continue
		}
		taken = append(taken, agent.ProfileSetting{Group: k.Group, Key: k.Key, Value: *v})
	}
	params, _ := json.Marshal(map[string]any{"to": body.To, "keys": body.Keys})
	switch body.To {
	case "account":
		out.AccountClient, err = s.opts.Microbot.Profiles.SetAccountClient(r.Context(), ac.AccountID, ac.Profile, mergeSettings(ac.Settings, taken))
	default:
		err = s.saveToProfile(r, ac.Profile, taken)
		if err == nil {
			out.AccountClient, err = s.opts.Microbot.Profiles.AccountClient(r.Context(), ac.AccountID)
		}
	}
	s.auditMicrobot(r, ac.AccountID, served, "profile_save", params, err)
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// saveToProfile puts settings into the profile's own layer.
func (s *Server) saveToProfile(r *http.Request, name string, settings []agent.ProfileSetting) error {
	p, err := s.opts.Microbot.Profiles.GetProfile(r.Context(), profileUser(r), name)
	if err != nil {
		return err
	}
	_, err = s.opts.Microbot.Profiles.PutProfile(r.Context(), profileUser(r), p.Name, p.Parent,
		agent.ClientProfile{Plugins: p.Plugins, Settings: mergeSettings(p.Settings, settings)})
	return err
}

// mergeSettings replaces in base the settings with the same group and key
// as over, and appends the rest.
func mergeSettings(base, over []agent.ProfileSetting) []agent.ProfileSetting {
	out := slices.Clone(base)
	for _, o := range over {
		i := slices.IndexFunc(out, func(b agent.ProfileSetting) bool { return b.Group == o.Group && b.Key == o.Key })
		if i >= 0 {
			out[i] = o
		} else {
			out = append(out, o)
		}
	}
	if out == nil {
		out = []agent.ProfileSetting{}
	}
	return out
}

// applyBody is POST /api/accounts/client/apply: the settings to put back in
// the client (none listed: every setting and the plugins), "Descartar" in
// the drift and "Aplicar o perfil" for an attached client.
type applyBody struct {
	Keys []settingKey `json:"keys"`
}

type applyAnswer struct {
	Applied  int      `json:"applied"`
	Problems []string `json:"problems"`
}

func (s *Server) handleClientApply(w http.ResponseWriter, r *http.Request) {
	ac, served, ok := s.driftTarget(w, r)
	if !ok {
		return
	}
	var body applyBody
	if r.ContentLength != 0 && !decodeBody(w, r, &body) {
		return
	}
	p := ac.Effective
	if len(body.Keys) > 0 {
		var settings []agent.ProfileSetting
		for _, k := range body.Keys {
			i := slices.IndexFunc(p.Settings, func(st agent.ProfileSetting) bool { return st.Group == k.Group && st.Key == k.Key })
			if i < 0 {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("%s.%s is not in the account's profile", k.Group, k.Key))
				return
			}
			settings = append(settings, p.Settings[i])
		}
		p = agent.ClientProfile{Settings: settings}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	out := s.applyProfile(ctx, served, p)
	params, _ := json.Marshal(map[string]any{"keys": body.Keys})
	var err error
	if len(out.Problems) > 0 {
		err = errors.New(out.Problems[0])
	}
	s.auditMicrobot(r, ac.AccountID, served, "profile_apply", params, err)
	writeJSON(w, http.StatusOK, out)
}

// applyProfile puts a profile into the client through its agent, in the
// order the worker does it (docs/11, W7): the settings, then the Hub
// installs, then the starts of the plugins not active yet.
func (s *Server) applyProfile(ctx context.Context, served agent.Info, p agent.ClientProfile) applyAnswer {
	link := s.opts.Microbot.Link
	out := applyAnswer{Problems: []string{}}
	call := func(what, command string, params any) bool {
		raw, _ := json.Marshal(params)
		if _, err := agent.ValidateParams(command, raw); err != nil {
			out.Problems = append(out.Problems, what+": "+err.Error())
			return false
		}
		if err := link.CallAgent(ctx, served.ID, command, json.RawMessage(raw), nil); err != nil {
			out.Problems = append(out.Problems, what+": "+err.Error())
			return false
		}
		out.Applied++
		return true
	}
	for _, st := range p.Settings {
		v := st.Value
		call("setting "+st.Group+"."+st.Key, agent.CmdConfigSet, agent.ConfigSetParams{Group: st.Group, Key: st.Key, Value: &v})
	}
	if len(p.Plugins) == 0 {
		return out
	}
	var pl agent.Plugins
	if err := link.CallAgent(ctx, served.ID, agent.CmdPlugins, nil, &pl); err != nil {
		out.Problems = append(out.Problems, "plugins: "+err.Error())
		return out
	}
	installed, active := agent.InstalledPlugins(pl.Installed), agent.ActiveScripts(pl.Scripts)
	missing := map[string]bool{}
	for _, x := range p.Plugins {
		if x.Source != agent.PluginHub {
			continue
		}
		if v, ok := installed[x.Name]; ok && (x.Version == "" || v == x.Version) {
			continue
		}
		if !call("install "+x.Name, agent.CmdPluginInstall, agent.PluginParams{Name: x.Name, Version: x.Version}) {
			missing[x.Name] = true
		}
	}
	for _, x := range p.Plugins {
		class := x.Name
		if x.Source == agent.PluginHub {
			class = x.ClassName
		}
		if !x.Start || missing[x.Name] || active[class] {
			continue
		}
		call("start "+class, agent.CmdScriptStart, agent.ScriptParams{ClassName: class})
	}
	return out
}
