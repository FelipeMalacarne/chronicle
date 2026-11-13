package httpapi

import "testing"

func TestPlainPluginName(t *testing.T) {
	for raw, want := range map[string]string{
		"<html>[<font color=#b8f704M>M</font>] Web Walker": "Web Walker",
		"Tick Fisher":           "Tick Fisher",
		"<html>Fish &amp; Cook": "Fish & Cook",
		"[M]":                   "[M]",
	} {
		if got := plainPluginName(raw); got != want {
			t.Errorf("plainPluginName(%q) = %q, want %q", raw, got, want)
		}
	}
}
