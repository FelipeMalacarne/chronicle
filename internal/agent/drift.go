package agent

import (
	"encoding/json"
	"slices"
)

// Drift (docs/11 section 4.3, F5 part 2): how a client differs from the
// account's effective profile, read through the Agent Server with the
// commands every agent has (config_get, plugins). A change made straight in
// the RuneLite panel over VNC shows here; the owner saves it to the profile
// or discards it (W9: by the button, never by itself).

// Plugin drift problems.
const (
	DriftNotInstalled = "not_installed" // a Hub plugin the profile installs is not loaded
	DriftVersion      = "version"       // a Hub plugin runs another version than the pinned one
	DriftNotStarted   = "not_started"   // a plugin the profile starts is not active
)

// Drift is what differs.
type Drift struct {
	Settings []SettingDrift `json:"settings"`
	Plugins  []PluginDrift  `json:"plugins"`
	// Unchecked are the plugins the client cannot be asked about: the
	// Agent Server lists Microbot plugins only, not RuneLite's own.
	Unchecked []string `json:"unchecked"`
}

// SettingDrift is a setting whose client value is not the profile's.
type SettingDrift struct {
	Group   string `json:"group"`
	Key     string `json:"key"`
	Profile string `json:"profile"`
	// Client is the client's value; nil when unset there.
	Client *string `json:"client"`
}

// PluginDrift is a plugin the client does not run as the profile says.
type PluginDrift struct {
	Source    string `json:"source"`
	Name      string `json:"name"`
	ClassName string `json:"class_name,omitempty"`
	Problem   string `json:"problem"`
	// Version is the version the client has (DriftVersion).
	Version string `json:"version,omitempty"`
}

// ClientState is what the client shows of a profile's parts.
type ClientState struct {
	// Values by group and key; nil is unset. A group not read is absent.
	Values map[string]map[string]*string
	// Active by class, from the Agent Server's /scripts (Microbot plugins).
	Active map[string]bool
	// Installed is each loaded Hub plugin's version, by internal name.
	Installed map[string]string
}

// ProfileDrift compares a client with its effective profile.
func ProfileDrift(p ClientProfile, c ClientState) Drift {
	out := Drift{Settings: []SettingDrift{}, Plugins: []PluginDrift{}, Unchecked: []string{}}
	for _, s := range p.Settings {
		group, ok := c.Values[s.Group]
		if !ok {
			continue
		}
		if v := group[s.Key]; v == nil || *v != s.Value {
			out.Settings = append(out.Settings, SettingDrift{Group: s.Group, Key: s.Key, Profile: s.Value, Client: v})
		}
	}
	for _, pl := range p.Plugins {
		d := PluginDrift{Source: pl.Source, Name: pl.Name, ClassName: pl.ClassName}
		if pl.Source == PluginHub {
			version, loaded := c.Installed[pl.Name]
			switch {
			case !loaded:
				d.Problem = DriftNotInstalled
			case pl.Version != "" && version != pl.Version:
				d.Problem, d.Version = DriftVersion, version
			}
			if d.Problem != "" {
				out.Plugins = append(out.Plugins, d)
				continue
			}
		}
		if !pl.Start {
			continue
		}
		active, listed := c.Active[pl.class()]
		switch {
		case !listed && pl.Source == PluginCore:
			out.Unchecked = append(out.Unchecked, pl.class())
		case !active:
			d.Problem = DriftNotStarted
			out.Plugins = append(out.Plugins, d)
		}
	}
	return out
}

// ActiveScripts reads the Agent Server's /scripts answer (Plugins.Scripts):
// each Microbot plugin's class and whether it is active.
func ActiveScripts(raw json.RawMessage) map[string]bool {
	var s struct {
		Scripts []struct {
			ClassName string `json:"className"`
			Active    bool   `json:"active"`
		} `json:"scripts"`
	}
	out := map[string]bool{}
	if json.Unmarshal(raw, &s) == nil {
		for _, x := range s.Scripts {
			out[x.ClassName] = x.Active
		}
	}
	return out
}

// InstalledPlugins reads the Agent Server's /plugins?installed=true answer
// (Plugins.Installed): each loaded Hub plugin's version.
func InstalledPlugins(raw json.RawMessage) map[string]string {
	var s struct {
		Plugins []struct {
			InternalName     string  `json:"internalName"`
			InstalledVersion *string `json:"installedVersion"`
			Loaded           bool    `json:"loaded"`
		} `json:"plugins"`
	}
	out := map[string]string{}
	if json.Unmarshal(raw, &s) == nil {
		for _, x := range s.Plugins {
			if x.Loaded {
				v := ""
				if x.InstalledVersion != nil {
					v = *x.InstalledVersion
				}
				out[x.InternalName] = v
			}
		}
	}
	return out
}

// SettingGroups groups a profile's setting keys by config group, in at most
// size keys per group (config_get reads up to 50 at once).
func SettingGroups(settings []ProfileSetting, size int) []ConfigGetParams {
	var out []ConfigGetParams
	index := map[string]int{}
	for _, s := range settings {
		i, ok := index[s.Group]
		if !ok || len(out[i].Keys) >= size {
			out = append(out, ConfigGetParams{Group: s.Group})
			i = len(out) - 1
			index[s.Group] = i
		}
		if !slices.Contains(out[i].Keys, s.Key) {
			out[i].Keys = append(out[i].Keys, s.Key)
		}
	}
	return out
}
