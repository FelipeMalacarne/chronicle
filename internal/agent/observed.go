package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ownPackage is where the plugins of this repository live (microbot/).
const ownPackage = "net.runelite.client.plugins.microbot.fm."

// ObservedPlugins is what a client has on, read from the answer to
// CmdPlugins, as the profile plugins that would start it again (docs/11
// section 15.4): a pod's home is new on every boot, so the plugins the owner
// turned on by hand are kept in the database and started by the next
// bootstrap, beside the client profile.
//
// Only Hub and own plugins are kept. The client's core plugins are mostly on
// by default and keep their state in the home, and starting one that is
// already on is an error of the Agent Server. The control path and the
// Chronicle Link are never kept (Protected).
//
// An error means the read was not complete (an answer that does not decode,
// or a list shorter than its count); the caller keeps what it had.
func ObservedPlugins(p Plugins) ([]ProfilePlugin, error) {
	var scripts struct {
		Count   *int `json:"count"`
		Scripts []struct {
			ClassName string `json:"className"`
			Enabled   bool   `json:"enabled"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(p.Scripts, &scripts); err != nil || scripts.Count == nil {
		return nil, errors.New("observed plugins: the script list does not decode")
	}
	if *scripts.Count != len(scripts.Scripts) {
		return nil, fmt.Errorf("observed plugins: the script list has %d of %d entries", len(scripts.Scripts), *scripts.Count)
	}
	var installed struct {
		Plugins []struct {
			InternalName     string  `json:"internalName"`
			InstalledVersion *string `json:"installedVersion"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(p.Installed, &installed); err != nil {
		return nil, errors.New("observed plugins: the Hub list does not decode")
	}
	hub := map[string]bool{}
	for _, h := range installed.Plugins {
		if h.InstalledVersion != nil && *h.InstalledVersion != "" {
			hub[h.InternalName] = true
		}
	}
	out := []ProfilePlugin{}
	for _, s := range scripts.Scripts {
		if !s.Enabled || Protected(s.ClassName) {
			continue
		}
		simple := s.ClassName[strings.LastIndex(s.ClassName, ".")+1:]
		var pl ProfilePlugin
		switch {
		case strings.HasPrefix(s.ClassName, ownPackage):
			pl = ProfilePlugin{Source: PluginOwn, Name: s.ClassName, Start: true}
		case hub[simple]:
			pl = ProfilePlugin{Source: PluginHub, Name: simple, ClassName: s.ClassName, Start: true}
		default:
			continue
		}
		if ValidatePlugin(pl) == nil {
			out = append(out, pl)
		}
	}
	return out, nil
}
