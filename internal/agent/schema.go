package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CmdConfigSchema is the config form of one plugin: the options Dev Bootstrap
// (microbot-plugins fm/devbootstrap 1.1+) exports to
// .runelite/agent/config-schema.json, because the Agent Server reads and
// writes values but does not list a plugin's options. The values still go
// through CmdConfigGet and CmdConfigSet.
const CmdConfigSchema = "config_schema"

// SchemaFile is the export's path under the dev home.
const SchemaFile = ".runelite/agent/config-schema.json"

// ErrNoSchema: the dev client has not exported the options (an older Dev
// Bootstrap, or the client has not started yet).
var ErrNoSchema = errors.New("config schema: the dev client exported no options (Dev Bootstrap 1.1+: mb-install devbootstrap and restart the client)")

// PluginSchema is one plugin's config form.
type PluginSchema struct {
	ClassName string          `json:"className"`
	Name      string          `json:"name"`
	Group     string          `json:"group"`
	Sections  []SchemaSection `json:"sections"`
	Items     []SchemaItem    `json:"items"`
}

// SchemaSection is a collapsible group of items in the plugin's panel.
type SchemaSection struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
	Description     string `json:"description,omitempty"`
	ClosedByDefault bool   `json:"closedByDefault,omitempty"`
}

// SchemaItem is one option: its type (boolean, int, double, string, enum,
// color, keybind, other), and for enums the stored names and their labels.
type SchemaItem struct {
	Key         string        `json:"key"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Position    int           `json:"position"`
	Section     string        `json:"section,omitempty"`
	Hidden      bool          `json:"hidden,omitempty"`
	Warning     string        `json:"warning,omitempty"`
	Type        string        `json:"type"`
	Values      []SchemaValue `json:"values,omitempty"`
	Min         *int          `json:"min,omitempty"`
	Max         *int          `json:"max,omitempty"`
	Units       string        `json:"units,omitempty"`
}

// SchemaValue is one enum constant: what ConfigManager stores and what the
// panel shows.
type SchemaValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// configSchema reads the export and returns className's form, under the same
// rules as CmdConfigGet: no protected group, no key that looks like a secret.
func (e *Executor) configSchema(className string) (PluginSchema, error) {
	raw, err := os.ReadFile(filepath.Join(e.Home, SchemaFile))
	if errors.Is(err, os.ErrNotExist) {
		return PluginSchema{}, ErrNoSchema
	}
	if err != nil {
		return PluginSchema{}, err
	}
	var file struct {
		Plugins []PluginSchema `json:"plugins"`
	}
	if err = json.Unmarshal(raw, &file); err != nil {
		return PluginSchema{}, fmt.Errorf("config schema: %w", err)
	}
	for _, p := range file.Plugins {
		if p.ClassName != className {
			continue
		}
		if gerr := checkGroup(p.Group); gerr != nil {
			return PluginSchema{}, gerr
		}
		items := p.Items[:0]
		for _, it := range p.Items {
			if checkKey(it.Key) == nil {
				items = append(items, it)
			}
		}
		p.Items = items
		return p, nil
	}
	return PluginSchema{}, fmt.Errorf("config schema: %s has no config options", className)
}
