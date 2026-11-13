// Package clientcfg holds what the planner may name and set in the Microbot
// client, wherever the name comes from: a command of the agent's closed
// list (docs/08, Segurança item 4), a client profile (docs/11 section 4.3)
// or a train task that delegates to a plugin (docs/10 section 7). A plugin
// is its fully qualified class; a setting is a RuneLite config group, key
// and value. The control path and Chronicle's own plugin are never started
// or stopped, and the groups that hold the control's secrets, and keys that
// look like secrets, are never read or written. Pure: the agent, the server
// and the content verifier apply the same rules.
package clientcfg

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Bounds.
const (
	// MaxClass is the longest class name.
	MaxClass = 256
	// MaxValue is the longest setting value.
	MaxValue = 2000
)

var (
	// ClassName is a fully qualified Java class name.
	ClassName = regexp.MustCompile(`^[A-Za-z_$][\w$]*(\.[A-Za-z_$][\w$]*)+$`)
	// ConfigID is a config group or key.
	ConfigID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	// SecretKey matches keys that hold secrets, whatever the plugin.
	SecretKey = regexp.MustCompile(`(?i)(password|passwd|token|secret|credential|cookie|apikey|api_key|bankpin|^pin$)`)
)

// ProtectedClasses are never started or stopped from the planner: the
// control path itself (Agent Server, the Microbot core, the bootstrap that
// starts the server) and Chronicle's own plugin, Chronicle Link (tracking and
// the control bridge, docs/06), also under its old name while an old jar may
// linger.
var ProtectedClasses = []string{
	"net.runelite.client.plugins.microbot.MicrobotPlugin",
	"net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin",
	"net.runelite.client.plugins.microbot.fm.devbootstrap.DevBootstrapPlugin",
	"net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin",
	"net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin",
}

// ProtectedGroups are config groups the planner never reads or writes: the
// Agent Server's (port, token), RuneLite's and Microbot's core, and the
// tracking plugin's (its planner credential).
var ProtectedGroups = []string{"agentServer", "runelite", "microbot", "fm-ironmanplanner"}

// Protected reports whether a plugin class is out of the planner's reach.
func Protected(className string) bool { return slices.Contains(ProtectedClasses, className) }

// ValidClass reports whether s is a well-formed class name of bounded length.
func ValidClass(s string) bool { return len(s) <= MaxClass && ClassName.MatchString(s) }

// CheckClass verifies a plugin the planner starts and stops: a class name,
// not one of the protected ones.
func CheckClass(s string) error {
	if !ValidClass(s) {
		return errors.New("plugin: a fully qualified class name")
	}
	if Protected(s) {
		return fmt.Errorf("plugin: %s is not controlled from the planner", s)
	}
	return nil
}

// CheckGroup verifies a config group: its shape, not a protected one.
func CheckGroup(g string) error {
	if !ConfigID.MatchString(g) {
		return errors.New("config: group must be letters, digits, '.', '_' or '-'")
	}
	for _, p := range ProtectedGroups {
		if strings.EqualFold(g, p) {
			return fmt.Errorf("config: the %q group is not edited from the planner", g)
		}
	}
	return nil
}

// CheckKey verifies a config key: its shape, not a secret.
func CheckKey(k string) error {
	if !ConfigID.MatchString(k) {
		return errors.New("config: key must be letters, digits, '.', '_' or '-'")
	}
	if SecretKey.MatchString(k) {
		return fmt.Errorf("config: %q looks like a secret and is not read or written from the planner", k)
	}
	return nil
}

// CheckValue verifies a setting's value: bounded, no NUL.
func CheckValue(v string) error {
	if len(v) > MaxValue || strings.ContainsRune(v, 0) {
		return fmt.Errorf("config: value of at most %d characters", MaxValue)
	}
	return nil
}

// Setting is one RuneLite config value a task gives its plugin.
type Setting struct {
	Group string `json:"group"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MaxSettings bounds the settings of one task.
const MaxSettings = 50

// CheckSettings verifies a task's settings: bounded, each one valid, no key
// twice.
func CheckSettings(list []Setting) error {
	if len(list) > MaxSettings {
		return fmt.Errorf("config: at most %d settings", MaxSettings)
	}
	for i, s := range list {
		for _, err := range []error{CheckGroup(s.Group), CheckKey(s.Key), CheckValue(s.Value)} {
			if err != nil {
				return fmt.Errorf("config/%d: %w", i, err)
			}
		}
		for _, o := range list[:i] {
			if strings.EqualFold(o.Group, s.Group) && o.Key == s.Key {
				return fmt.Errorf("config/%d: %s.%s twice", i, s.Group, s.Key)
			}
		}
	}
	return nil
}
