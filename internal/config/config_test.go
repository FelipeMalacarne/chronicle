package config

import (
	"log/slog"
	"strings"
	"testing"
)

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	c, err := FromLookup(lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.AuthMode != "dev" || c.ContentDir != "content" || c.LogLevel != "info" || c.DatabaseURL != "" {
		t.Errorf("defaults = %+v", c)
	}
	if lvl, _ := c.SlogLevel(); lvl != slog.LevelInfo {
		t.Errorf("level = %v", lvl)
	}
}

func TestOIDCRequiresFields(t *testing.T) {
	_, err := FromLookup(lookup(map[string]string{"AUTH_MODE": "oidc", "OIDC_ISSUER": "https://x"}))
	if err == nil || !strings.Contains(err.Error(), "OIDC_CLIENT_ID") || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Errorf("err = %v", err)
	}
	c, err := FromLookup(lookup(map[string]string{
		"AUTH_MODE": "OIDC", "OIDC_ISSUER": "https://x", "OIDC_CLIENT_ID": "id",
		"OIDC_REDIRECT_URL": "https://p/auth/callback", "SESSION_SECRET": "s", "PUBLIC_BASE_URL": "https://p/",
		"LOG_LEVEL": "debug", "IRONMAN_ADDR": "127.0.0.1:9000",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthMode != "oidc" || c.PublicBaseURL != "https://p" || c.Addr != "127.0.0.1:9000" {
		t.Errorf("cfg = %+v", c)
	}
	if _, err := FromLookup(lookup(map[string]string{"AUTH_MODE": "magic"})); err == nil {
		t.Error("bad auth mode accepted")
	}
	if _, err := FromLookup(lookup(map[string]string{"LOG_LEVEL": "loud"})); err == nil {
		t.Error("bad log level accepted")
	}
}

func TestSyncHiscoresMinutes(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "", want: 30},
		{raw: "0", want: 0},
		{raw: "10", want: 10},
		{raw: "-1", wantErr: true},
		{raw: "half", wantErr: true},
	} {
		c, err := FromLookup(lookup(map[string]string{"SYNC_HISCORES_MINUTES": tc.raw}))
		if (err != nil) != tc.wantErr || (!tc.wantErr && c.SyncHiscoresMinutes != tc.want) {
			t.Errorf("%q = %d, %v", tc.raw, c.SyncHiscoresMinutes, err)
		}
	}
}
