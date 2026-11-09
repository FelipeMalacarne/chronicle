package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Plugin, script and config commands (docs/08 phase 3). Each one is a fixed
// Agent Server request built from typed, validated parameters; nothing here
// forwards a path or a body. /scripts/deploy (arbitrary Java), raw keyboard
// and widgets stay out of the list.
const (
	CmdPlugins       = "plugins"        // loaded plugins (/scripts) and Hub installs
	CmdHubSearch     = "hub_search"     // search the Plugin Hub
	CmdPluginInstall = "plugin_install" // install (or update) from the Hub
	CmdPluginRemove  = "plugin_remove"  // stop, unload and delete a Hub plugin
	CmdScriptStart   = "script_start"
	CmdScriptStop    = "script_stop"
	CmdScriptsPause  = "scripts_pause" // every script loop (Microbot.pauseAllScripts)
	CmdScriptsResume = "scripts_resume"
	CmdScriptDetail  = "script_detail" // status, health, results, state machines
	CmdConfigGet     = "config_get"
	CmdConfigSet     = "config_set"
)

// HubParams are the parameters of CmdHubSearch.
type HubParams struct {
	Q string `json:"q"`
}

// PluginParams are the parameters of CmdPluginInstall and CmdPluginRemove:
// the Hub's internal name (e.g. "GotrPlugin") and, to install, a version
// (empty is the Hub's latest).
type PluginParams struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ScriptParams are the parameters of CmdScriptStart, CmdScriptStop and
// CmdScriptDetail: the plugin's fully qualified class name.
type ScriptParams struct {
	ClassName string `json:"className"`
}

// ConfigGetParams are the parameters of CmdConfigGet.
type ConfigGetParams struct {
	Group string   `json:"group"`
	Keys  []string `json:"keys"`
}

// ConfigSetParams are the parameters of CmdConfigSet; a null value unsets
// the key.
type ConfigSetParams struct {
	Group string  `json:"group"`
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

// Plugins is the answer to CmdPlugins: the Agent Server's own JSON.
type Plugins struct {
	Scripts   json.RawMessage `json:"scripts"`   // GET /scripts
	Installed json.RawMessage `json:"installed"` // GET /plugins?installed=true
}

// ScriptDetail is the answer to CmdScriptDetail; a part that failed carries
// its error instead.
type ScriptDetail struct {
	Status   json.RawMessage   `json:"status,omitempty"`
	Health   json.RawMessage   `json:"health,omitempty"`
	Results  json.RawMessage   `json:"results,omitempty"`
	Machines json.RawMessage   `json:"machines,omitempty"` // GET /debug/snapshot
	Errors   map[string]string `json:"errors,omitempty"`
}

// ConfigValues is the answer to CmdConfigGet; an unset key is null.
type ConfigValues struct {
	Group  string             `json:"group"`
	Values map[string]*string `json:"values"`
}

var (
	reHubName   = regexp.MustCompile(`^[A-Za-z0-9_]{1,80}$`)
	reVersion   = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,32}$`)
	reClassName = regexp.MustCompile(`^[A-Za-z_$][\w$]*(\.[A-Za-z_$][\w$]*)+$`)
	reConfigID  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	// Keys that hold secrets, whatever the plugin.
	reSecretKey = regexp.MustCompile(`(?i)(password|passwd|token|secret|credential|cookie|apikey|api_key|bankpin|^pin$)`)
)

// protectedClasses are never started or stopped from the web: the control
// path itself (Agent Server, the Microbot core, the bootstrap that starts the
// server) and the planner's read-only tracking plugin (docs/06).
var protectedClasses = []string{
	"net.runelite.client.plugins.microbot.MicrobotPlugin",
	"net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin",
	"net.runelite.client.plugins.microbot.fm.devbootstrap.DevBootstrapPlugin",
	"net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin",
}

// protectedGroups are config groups the web never reads or writes: the Agent
// Server's (port, token), RuneLite's and Microbot's core, and the tracking
// plugin's (its planner credential).
var protectedGroups = []string{"agentServer", "runelite", "microbot", "fm-ironmanplanner"}

// Protected reports whether a plugin class is out of the web's reach.
func Protected(className string) bool { return slices.Contains(protectedClasses, className) }

// ValidateParams decodes and checks the parameters of a command; the server
// calls it to refuse early and the agent calls it again before acting.
func ValidateParams(command string, raw json.RawMessage) (any, error) {
	switch command {
	case CmdHubSearch:
		var p HubParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		p.Q = strings.TrimSpace(p.Q)
		if len(p.Q) > 80 || strings.ContainsAny(p.Q, "\x00\r\n") {
			return nil, errors.New("search: at most 80 characters, one line")
		}
		return p, nil
	case CmdPluginInstall, CmdPluginRemove:
		var p PluginParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if !reHubName.MatchString(p.Name) {
			return nil, errors.New("plugin: name must be the Hub's internal name (letters, digits, _)")
		}
		if p.Version != "" && (command == CmdPluginRemove || !reVersion.MatchString(p.Version)) {
			return nil, errors.New("plugin: bad version")
		}
		return p, nil
	case CmdScriptStart, CmdScriptStop, CmdScriptDetail, CmdConfigSchema:
		var p ScriptParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if len(p.ClassName) > 256 || !reClassName.MatchString(p.ClassName) {
			return nil, errors.New("script: className must be a fully qualified class name")
		}
		if command != CmdScriptDetail && command != CmdConfigSchema && Protected(p.ClassName) {
			return nil, fmt.Errorf("script: %s is not controlled from the planner", p.ClassName)
		}
		return p, nil
	case CmdConfigGet:
		var p ConfigGetParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := checkGroup(p.Group); err != nil {
			return nil, err
		}
		if len(p.Keys) == 0 || len(p.Keys) > 50 {
			return nil, errors.New("config: between 1 and 50 keys")
		}
		for _, k := range p.Keys {
			if err := checkKey(k); err != nil {
				return nil, err
			}
		}
		return p, nil
	case CmdConfigSet:
		var p ConfigSetParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := checkGroup(p.Group); err != nil {
			return nil, err
		}
		if err := checkKey(p.Key); err != nil {
			return nil, err
		}
		if p.Value != nil && (len(*p.Value) > 2000 || strings.ContainsRune(*p.Value, 0)) {
			return nil, errors.New("config: value of at most 2000 characters")
		}
		return p, nil
	case CmdPlugins, CmdScriptsPause, CmdScriptsResume, CmdGameView:
		return nil, nil
	}
	if slices.Contains(GameActions, command) {
		return validateGame(command, raw)
	}
	switch command {
	case CmdClientLogs, CmdAccountShow, CmdAccountUse, CmdWorldHop:
		return validateClient(command, raw)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownCommand, command)
}

func decodeParams(raw json.RawMessage, into any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	return nil
}

func checkGroup(g string) error {
	if !reConfigID.MatchString(g) {
		return errors.New("config: group must be letters, digits, '.', '_' or '-'")
	}
	for _, p := range protectedGroups {
		if strings.EqualFold(g, p) {
			return fmt.Errorf("config: the %q group is not edited from the planner", g)
		}
	}
	return nil
}

func checkKey(k string) error {
	if !reConfigID.MatchString(k) {
		return errors.New("config: key must be letters, digits, '.', '_' or '-'")
	}
	if reSecretKey.MatchString(k) {
		return fmt.Errorf("config: %q looks like a secret and is not read or written from the planner", k)
	}
	return nil
}

// plugin runs one plugin, script or config command.
func (e *Executor) plugin(ctx context.Context, command string, params json.RawMessage) (any, error) {
	p, err := ValidateParams(command, params)
	if err != nil {
		return nil, err
	}
	g := e.Game
	switch pp := p.(type) {
	case nil:
		switch command {
		case CmdPlugins:
			scripts, err := g.JSON(ctx, http.MethodGet, "/scripts", nil, nil, 15*time.Second)
			if err != nil {
				return nil, err
			}
			installed, err := g.JSON(ctx, http.MethodGet, "/plugins", url.Values{"installed": {"true"}}, nil, 15*time.Second)
			if err != nil {
				return nil, err
			}
			return Plugins{Scripts: scripts, Installed: installed}, nil
		case CmdScriptsPause:
			return g.JSON(ctx, http.MethodPost, "/scripts/pause", nil, map[string]any{}, 15*time.Second)
		case CmdScriptsResume:
			return g.JSON(ctx, http.MethodPost, "/scripts/resume", nil, map[string]any{}, 15*time.Second)
		}
	case HubParams:
		q := url.Values{"limit": {"60"}}
		if pp.Q != "" {
			q.Set("q", pp.Q)
		}
		return g.JSON(ctx, http.MethodGet, "/plugins", q, nil, 20*time.Second)
	case PluginParams:
		if command == CmdPluginRemove {
			return g.JSON(ctx, http.MethodPost, "/plugins/remove", nil, map[string]any{"name": pp.Name, "timeout": 60}, 90*time.Second)
		}
		body := map[string]any{"name": pp.Name, "timeout": 120}
		if pp.Version != "" {
			body["version"] = pp.Version
		}
		return g.JSON(ctx, http.MethodPost, "/plugins/install", nil, body, 150*time.Second)
	case ScriptParams:
		switch command {
		case CmdScriptStart:
			return g.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": pp.ClassName}, 30*time.Second)
		case CmdScriptStop:
			return g.JSON(ctx, http.MethodPost, "/scripts/stop", nil, map[string]any{"className": pp.ClassName}, 30*time.Second)
		case CmdScriptDetail:
			return e.scriptDetail(ctx, pp.ClassName), nil
		case CmdConfigSchema:
			return e.configSchema(pp.ClassName)
		}
	case ConfigGetParams:
		out := ConfigValues{Group: pp.Group, Values: map[string]*string{}}
		for _, k := range pp.Keys {
			raw, err := g.JSON(ctx, http.MethodGet, "/settings/plugin", url.Values{"group": {pp.Group}, "key": {k}}, nil, 10*time.Second)
			if err != nil {
				return nil, err
			}
			var v struct {
				Value *string `json:"value"`
			}
			_ = json.Unmarshal(raw, &v)
			out.Values[k] = v.Value
		}
		return out, nil
	case ConfigSetParams:
		return g.JSON(ctx, http.MethodPost, "/settings/plugin", nil,
			map[string]any{"group": pp.Group, "key": pp.Key, "value": pp.Value}, 10*time.Second)
	}
	return nil, fmt.Errorf("command %q is not in the agent's closed list", command)
}

func (e *Executor) scriptDetail(ctx context.Context, className string) ScriptDetail {
	var d ScriptDetail
	q := url.Values{"className": {className}}
	get := func(name, path string, query url.Values) json.RawMessage {
		raw, err := e.Game.JSON(ctx, http.MethodGet, path, query, nil, 10*time.Second)
		if err != nil {
			if d.Errors == nil {
				d.Errors = map[string]string{}
			}
			d.Errors[name] = err.Error()
		}
		return raw
	}
	d.Status = get("status", "/scripts/status", q)
	d.Health = get("health", "/scripts/health", q)
	d.Results = get("results", "/scripts/results", q)
	d.Machines = get("machines", "/debug/snapshot", nil)
	return d
}
