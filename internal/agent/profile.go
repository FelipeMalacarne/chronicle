package agent

import (
	"fmt"
	"strings"
)

// Client profiles (docs/11 section 4.3, phase F5): what a worker's client
// runs and how it is configured, kept in the database because a pod loses
// its disk. The server sends the account's effective profile in the
// bootstrap; once the Agent Server answers, the agent writes the settings
// first, then installs the Hub plugins and starts the plugins marked start,
// so each plugin reads its configuration when it starts (W7, decided
// 2026-11-12: through the Agent Server, not a profile file written before
// the client).

// Sources of a profile plugin.
const (
	// PluginHub is a plugin of the Microbot Plugin Hub, installed by its
	// internal name.
	PluginHub = "hub"
	// PluginOwn is a plugin of this repository (microbot/), which the worker
	// image ships prebuilt and the client always loads.
	PluginOwn = "own"
	// PluginCore is a plugin of the client itself (RuneLite or Microbot).
	PluginCore = "core"
)

// Limits of one profile layer.
const (
	maxProfilePlugins  = 100
	maxProfileSettings = 500
	maxSettingValue    = 2000
)

// ClientProfile is a client profile as the agent applies it: the plugins
// in the order they start and the RuneLite settings.
type ClientProfile struct {
	Plugins  []ProfilePlugin  `json:"plugins"`
	Settings []ProfileSetting `json:"settings"`
}

// ProfilePlugin is one plugin of a profile.
type ProfilePlugin struct {
	// Source is PluginHub, PluginOwn or PluginCore.
	Source string `json:"source"`
	// Name is the Hub's internal name (hub) or the plugin's fully qualified
	// class name (own, core).
	Name string `json:"name"`
	// Version pins a Hub version; empty is the Hub's latest (hub only).
	Version string `json:"version,omitempty"`
	// ClassName is the class of a Hub plugin, needed to start it (the Hub
	// installs by name, the Agent Server starts by class); hub only.
	ClassName string `json:"class_name,omitempty"`
	// Start: the agent starts the plugin when the client comes up. A plugin
	// that does not start is still installed (hub) and keeps the client's
	// own enabled state.
	Start bool `json:"start"`
}

// ProfileSetting is one RuneLite config value (ConfigManager group and key).
type ProfileSetting struct {
	Group string `json:"group"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// class is the class the Agent Server starts for the plugin ("" when a Hub
// plugin names none).
func (p ProfilePlugin) class() string {
	if p.Source == PluginHub {
		return p.ClassName
	}
	return p.Name
}

func (p ProfilePlugin) id() string { return p.Source + ":" + p.Name }

func (s ProfileSetting) id() string { return s.Group + "\x00" + s.Key }

// ValidatePlugin checks one plugin of a profile.
func ValidatePlugin(p ProfilePlugin) error {
	switch p.Source {
	case PluginHub:
		if !reHubName.MatchString(p.Name) {
			return fmt.Errorf("plugin %q: a Hub plugin is named by its internal name (letters, digits, _)", p.Name)
		}
		if p.Version != "" && !reVersion.MatchString(p.Version) {
			return fmt.Errorf("plugin %s: bad version %q", p.Name, p.Version)
		}
		if p.ClassName != "" && (len(p.ClassName) > 256 || !reClassName.MatchString(p.ClassName)) {
			return fmt.Errorf("plugin %s: class_name must be a fully qualified class name", p.Name)
		}
		if p.Start && p.ClassName == "" {
			return fmt.Errorf("plugin %s: a Hub plugin that starts needs its class_name", p.Name)
		}
	case PluginOwn, PluginCore:
		if len(p.Name) > 256 || !reClassName.MatchString(p.Name) {
			return fmt.Errorf("plugin %q: an %s plugin is named by its fully qualified class name", p.Name, p.Source)
		}
		if p.Version != "" || p.ClassName != "" {
			return fmt.Errorf("plugin %s: version and class_name are for Hub plugins only", p.Name)
		}
	default:
		return fmt.Errorf("plugin %q: source must be %s, %s or %s", p.Name, PluginHub, PluginOwn, PluginCore)
	}
	if c := p.class(); c != "" && Protected(c) {
		return fmt.Errorf("plugin %s: %s is not controlled from the planner", p.Name, c)
	}
	return nil
}

// ValidateSetting checks one setting: the same groups, keys and values
// config_set takes.
func ValidateSetting(s ProfileSetting) error {
	if err := checkGroup(s.Group); err != nil {
		return err
	}
	if err := checkKey(s.Key); err != nil {
		return err
	}
	if len(s.Value) > maxSettingValue || strings.ContainsRune(s.Value, 0) {
		return fmt.Errorf("config: %s.%s: value of at most %d characters", s.Group, s.Key, maxSettingValue)
	}
	return nil
}

// ValidateProfile checks one layer of a profile (a profile or an account's
// overrides): every plugin and setting, no plugin or setting twice, and the
// size limits. The server checks before storing and the agent again before
// applying.
func ValidateProfile(p ClientProfile) error {
	if len(p.Plugins) > maxProfilePlugins {
		return fmt.Errorf("profile: at most %d plugins", maxProfilePlugins)
	}
	if len(p.Settings) > maxProfileSettings {
		return fmt.Errorf("profile: at most %d settings", maxProfileSettings)
	}
	seen := map[string]bool{}
	for _, pl := range p.Plugins {
		if err := ValidatePlugin(pl); err != nil {
			return err
		}
		if seen[pl.id()] {
			return fmt.Errorf("plugin %s (%s) is listed twice", pl.Name, pl.Source)
		}
		seen[pl.id()] = true
	}
	for _, s := range p.Settings {
		if err := ValidateSetting(s); err != nil {
			return err
		}
		if seen[s.id()] {
			return fmt.Errorf("setting %s.%s is listed twice", s.Group, s.Key)
		}
		seen[s.id()] = true
	}
	return nil
}

// MergeProfiles is the effective profile of layers applied in order (the
// model, the profile, the account's overrides): a later layer's plugin or
// setting replaces an earlier one with the same source and name, or group
// and key, in the earlier one's place; new ones go after.
func MergeProfiles(layers ...ClientProfile) ClientProfile {
	out := ClientProfile{Plugins: []ProfilePlugin{}, Settings: []ProfileSetting{}}
	plugin, setting := map[string]int{}, map[string]int{}
	for _, l := range layers {
		for _, p := range l.Plugins {
			if i, ok := plugin[p.id()]; ok {
				out.Plugins[i] = p
				continue
			}
			plugin[p.id()] = len(out.Plugins)
			out.Plugins = append(out.Plugins, p)
		}
		for _, s := range l.Settings {
			if i, ok := setting[s.id()]; ok {
				out.Settings[i] = s
				continue
			}
			setting[s.id()] = len(out.Settings)
			out.Settings = append(out.Settings, s)
		}
	}
	return out
}
