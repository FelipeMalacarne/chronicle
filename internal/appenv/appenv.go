// Package appenv holds the product's names in the environment: the
// CHRONICLE_* variables and the config directory. The product was called
// "ironman" (the binary, IRONMAN_* variables, ~/.config/ironman) until
// 2026-11-11; the old names keep working as a fallback, and the old config
// directory moves to the new one on first use.
package appenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Prefix of the product's variables; LegacyPrefix is read when it is unset.
const (
	Prefix       = "CHRONICLE_"
	LegacyPrefix = "IRONMAN_"
)

// Get reads CHRONICLE_<name>, else IRONMAN_<name>.
func Get(name string) string {
	if v, ok := os.LookupEnv(Prefix + name); ok {
		return v
	}
	return os.Getenv(LegacyPrefix + name)
}

// ConfigDir is $CHRONICLE_CONFIG_DIR (or $IRONMAN_CONFIG_DIR), else
// <user config dir>/chronicle (~/.config/chronicle on Linux): the CLI's
// login and settings and the agent's files. A ~/.config/ironman left by an
// older binary is renamed to it the first time, so a login survives.
func ConfigDir() (string, error) {
	if dir := Get("CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config directory: %w", err)
	}
	dir := filepath.Join(base, "chronicle")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		legacy := filepath.Join(base, "ironman")
		if info, err := os.Stat(legacy); err == nil && info.IsDir() {
			// best effort: on failure the new directory starts empty
			_ = os.Rename(legacy, dir)
		}
	}
	return dir, nil
}
