package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// The fleet's plugins (the v7's Plugins screen, slice V8; artboard
// V7Plugins1440). The owner decided on 2026-11-13: the "Padrão da frota" is
// a model profile, FleetModel, and each account has its own profile on top
// of it, which holds the account's own adjustment; the screen's groups come
// from use, never marked by hand: what the profiles start on boot is
// "Sempre ligados", what a trigger runs is "Rotinas usam", the rest
// installed is "Jogam". The writes go through the profile routes and
// POST /api/accounts/client/apply.

// FleetModel is the name of the fleet's model profile.
const FleetModel = "frota"

// Plugin groups of the Plugins screen.
const (
	GroupAlways   = "always"
	GroupRoutines = "routines"
	GroupPlays    = "plays"
)

// ownPluginPrefix is the package of this repository's plugins (microbot/).
const ownPluginPrefix = "net.runelite.client.plugins.microbot.fm."

type fleetPluginsAnswer struct {
	// Model is the fleet's model profile; nil before its first save.
	Model    *storeworkers.Profile `json:"model"`
	Accounts []fleetPluginAccount  `json:"accounts"`
	Plugins  []fleetPlugin         `json:"plugins"`
}

type fleetPluginAccount struct {
	Account string `json:"account"`
	RSN     string `json:"rsn"`
	// Profile is the account's profile ("" none); Own its own layer of
	// settings, the account's adjustment over the model.
	Profile string                 `json:"profile,omitempty"`
	Parent  string                 `json:"parent,omitempty"`
	Own     []agent.ProfileSetting `json:"own"`
	// OwnPlugins are the plugins of the account's own profile layer.
	OwnPlugins []agent.ProfilePlugin `json:"own_plugins"`
	// Connected: its agent is linked, so the live states below are known.
	Connected bool `json:"connected"`
}

type fleetPlugin struct {
	Source    string `json:"source"`
	Name      string `json:"name"`
	ClassName string `json:"class_name,omitempty"`
	// Title is the plugin's name as the client shows it, when known.
	Title string `json:"title,omitempty"`
	Group string `json:"group"`
	// InModel and Model are the plugin as the fleet's model declares it.
	InModel bool                 `json:"in_model"`
	Model   *agent.ProfilePlugin `json:"model,omitempty"`
	// Latest is the Hub's latest version (Hub plugins, when an agent could ask).
	Latest   string             `json:"latest,omitempty"`
	Accounts []fleetPluginState `json:"accounts"`
}

type fleetPluginState struct {
	Account string `json:"account"`
	// InProfile: the account's effective profile installs or starts it.
	InProfile bool `json:"in_profile"`
	// Installed is the version the client has (Hub plugins); Active whether
	// it runs now (Microbot plugins). Both absent when the agent is away.
	Installed *string `json:"installed,omitempty"`
	Active    *bool   `json:"active,omitempty"`
}

func (s *Server) fleetPluginRoutes() {
	m := s.opts.Microbot
	if m == nil || m.Profiles == nil || s.opts.Identity == nil || !s.accountRoutes() {
		return
	}
	s.mux.Handle("GET /api/fleet/plugins", s.authed(s.microbotControl(s.handleFleetPlugins)))
}

func (s *Server) handleFleetPlugins(w http.ResponseWriter, r *http.Request) {
	m := s.opts.Microbot
	accounts, err := s.requestAccounts(r)
	if err != nil {
		s.log.Error("fleet plugins: accounts", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the accounts")
		return
	}
	out := fleetPluginsAnswer{Accounts: []fleetPluginAccount{}, Plugins: []fleetPlugin{}}
	model, err := m.Profiles.GetProfile(r.Context(), profileUser(r), FleetModel)
	switch {
	case err == nil:
		out.Model = &model
	case !errors.Is(err, storeworkers.ErrProfileNotFound):
		s.writeProfileError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	var views []accountPlugins
	for _, a := range accounts {
		v, verr := s.accountPlugins(ctx, r, a)
		if verr != nil {
			s.writeProfileError(w, verr)
			return
		}
		views = append(views, v)
		out.Accounts = append(out.Accounts, v.account)
	}
	out.Plugins = groupFleetPlugins(out.Model, views)
	s.hubLatest(ctx, views, out.Plugins)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// accountPlugins is what one account has: its profile and, with its agent
// linked, what its client runs.
type accountPlugins struct {
	account   fleetPluginAccount
	effective agent.ClientProfile
	served    agent.Info
	live      bool
	scripts   map[string]bool   // class → active
	titles    map[string]string // class → name in the client
	installed map[string]string // Hub name → version
	triggers  []agent.Trigger
}

func (s *Server) accountPlugins(ctx context.Context, r *http.Request, a storeprogress.AccountView) (accountPlugins, error) {
	m := s.opts.Microbot
	v := accountPlugins{account: fleetPluginAccount{Account: a.ID, RSN: a.RSN, Own: []agent.ProfileSetting{}, OwnPlugins: []agent.ProfilePlugin{}}}
	ac, err := m.Profiles.AccountClient(ctx, a.ID)
	if err != nil {
		return v, err
	}
	v.effective, v.account.Profile = ac.Effective, ac.Profile
	if ac.Profile != "" {
		p, perr := m.Profiles.GetProfile(ctx, profileUser(r), ac.Profile)
		if perr != nil {
			return v, perr
		}
		v.account.Parent, v.account.Own, v.account.OwnPlugins = p.Parent, p.Settings, p.Plugins
	}
	served, err := m.Link.Route(a.ID)
	if err != nil || served.Account != a.ID {
		return v, nil // no agent of its own: no live state
	}
	v.served, v.account.Connected = served, true
	var pl agent.Plugins
	if m.Link.CallAgent(ctx, served.ID, agent.CmdPlugins, nil, &pl) == nil {
		v.live = true
		v.scripts, v.installed = agent.ActiveScripts(pl.Scripts), agent.InstalledPlugins(pl.Installed)
		v.titles = scriptTitles(pl.Scripts)
	}
	var ts agent.TriggerStatus
	if m.Link.CallAgent(ctx, served.ID, agent.CmdTriggerGet, nil, &ts) == nil {
		v.triggers = ts.Triggers
	}
	return v, nil
}

func scriptTitles(raw json.RawMessage) map[string]string {
	var s struct {
		Scripts []struct {
			Name      string `json:"name"`
			ClassName string `json:"className"`
		} `json:"scripts"`
	}
	out := map[string]string{}
	if json.Unmarshal(raw, &s) == nil {
		for _, x := range s.Scripts {
			out[x.ClassName] = plainPluginName(x.Name)
		}
	}
	return out
}

var (
	htmlTag      = regexp.MustCompile(`<[^>]*>`)
	tagPrefix    = regexp.MustCompile(`^\s*\[[^\]]{1,6}\]\s*`)
	htmlEntities = strings.NewReplacer("&amp;", "&", "&#39;", "'", "&apos;", "'")
)

// plainPluginName is a plugin's name without the HTML the client puts in it
// ("<html>[<font color=…>M</font>] Web Walker" is "Web Walker"), as the web's
// cleanName does.
func plainPluginName(raw string) string {
	text := htmlEntities.Replace(htmlTag.ReplaceAllString(raw, ""))
	if clean := strings.TrimSpace(tagPrefix.ReplaceAllString(text, "")); clean != "" {
		return clean
	}
	return raw
}

// groupFleetPlugins lists every plugin the model, an account's profile or
// a client has (this repository's and the Hub's; the client's own and the
// protected ones stay out), with its group and each account's state.
func groupFleetPlugins(model *storeworkers.Profile, views []accountPlugins) []fleetPlugin {
	byID := map[string]*fleetPlugin{}
	var order []string
	add := func(p agent.ProfilePlugin) *fleetPlugin {
		id := p.Source + ":" + p.Name
		if f, ok := byID[id]; ok {
			return f
		}
		f := &fleetPlugin{Source: p.Source, Name: p.Name, ClassName: p.ClassName}
		byID[id] = f
		order = append(order, id)
		return f
	}
	starts := map[string]bool{}
	if model != nil {
		for _, p := range model.Plugins {
			mp := p
			f := add(p)
			f.InModel, f.Model = true, &mp
			if p.Start {
				starts[pluginClass(p)] = true
			}
		}
	}
	routine := map[string]bool{}
	for _, v := range views {
		for _, p := range v.effective.Plugins {
			add(p)
			if p.Start {
				starts[pluginClass(p)] = true
			}
		}
		for name := range v.installed {
			add(agent.ProfilePlugin{Source: agent.PluginHub, Name: name})
		}
		for class := range v.scripts {
			if strings.HasPrefix(class, ownPluginPrefix) && !agent.Protected(class) {
				add(agent.ProfilePlugin{Source: agent.PluginOwn, Name: class})
			}
		}
		for _, t := range v.triggers {
			routine[t.Run.ClassName] = true
		}
	}
	out := make([]fleetPlugin, 0, len(order))
	for _, id := range order {
		f := byID[id]
		class := cmp.Or(f.ClassName, f.Name)
		switch {
		case routine[class]:
			f.Group = GroupRoutines
		case starts[class]:
			f.Group = GroupAlways
		default:
			f.Group = GroupPlays
		}
		f.Accounts = []fleetPluginState{}
		for _, v := range views {
			st := fleetPluginState{Account: v.account.Account}
			st.InProfile = slices.ContainsFunc(v.effective.Plugins, func(p agent.ProfilePlugin) bool {
				return p.Source == f.Source && p.Name == f.Name
			})
			if v.live {
				if f.Source == agent.PluginHub {
					if ver, ok := v.installed[f.Name]; ok {
						st.Installed = &ver
					}
				}
				if active, ok := v.scripts[class]; ok && f.ClassName+f.Name != "" {
					st.Active = &active
					if f.Title == "" {
						f.Title = v.titles[class]
					}
				}
			}
			f.Accounts = append(f.Accounts, st)
		}
		out = append(out, *f)
	}
	slices.SortStableFunc(out, func(a, b fleetPlugin) int {
		return cmp.Or(cmp.Compare(groupRank(a.Group), groupRank(b.Group)), cmp.Compare(strings.ToLower(cmp.Or(a.Title, a.Name)), strings.ToLower(cmp.Or(b.Title, b.Name))))
	})
	return out
}

func groupRank(g string) int {
	switch g {
	case GroupPlays:
		return 0
	case GroupRoutines:
		return 1
	}
	return 2
}

func pluginClass(p agent.ProfilePlugin) string {
	if p.Source == agent.PluginHub {
		return p.ClassName
	}
	return p.Name
}

// hubLatest asks one linked agent the Hub's latest version of each Hub
// plugin listed; a failure leaves it out.
func (s *Server) hubLatest(ctx context.Context, views []accountPlugins, plugins []fleetPlugin) {
	i := slices.IndexFunc(views, func(v accountPlugins) bool { return v.live })
	if i < 0 {
		return
	}
	served := views[i].served
	for k := range plugins {
		p := &plugins[k]
		if p.Source != agent.PluginHub {
			continue
		}
		var res struct {
			Plugins []struct {
				InternalName  string `json:"internalName"`
				Name          string `json:"name"`
				LatestVersion string `json:"latestVersion"`
			} `json:"plugins"`
		}
		if s.opts.Microbot.Link.CallAgent(ctx, served.ID, agent.CmdHubSearch, agent.HubParams{Q: p.Name}, &res) != nil {
			continue
		}
		for _, h := range res.Plugins {
			if h.InternalName == p.Name {
				p.Latest = h.LatestVersion
				if p.Title == "" {
					p.Title = h.Name
				}
			}
		}
	}
}
