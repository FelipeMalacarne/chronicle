// Package buildinfo exposes the binary's version string.
//
// Version is set at build time with
//
//	-ldflags "-X github.com/FelipeMalacarne/ironman-planner/internal/buildinfo.Version=v1.2.3"
//
// and falls back to the VCS revision recorded by the Go toolchain, then "dev".
package buildinfo

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Version is the release version. Overridden via -ldflags; see package doc.
var Version = ""

var (
	once     sync.Once
	resolved string
)

// String returns the effective version: Version when set, otherwise
// "dev+<short vcs revision>[-dirty]" when the toolchain recorded one, otherwise "dev".
func String() string {
	once.Do(func() {
		resolved = resolve()
	})
	return resolved
}

func resolve() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	return strings.TrimSpace("dev+" + rev + dirty)
}
