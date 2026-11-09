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
		"OIDC_REDIRECT_URL": "https://p/auth/callback", "SESSION_SECRET": testSecret, "PUBLIC_BASE_URL": "https://p/",
		"OIDC_ALLOWED_SUBJECTS": " 123, ,456 ", "LOG_LEVEL": "debug", "IRONMAN_ADDR": "127.0.0.1:9000",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthMode != "oidc" || c.PublicBaseURL != "https://p" || c.Addr != "127.0.0.1:9000" || !c.SecureCookies() {
		t.Errorf("cfg = %+v", c)
	}
	if strings.Join(c.OIDCAllowedSubjects, "|") != "123|456" {
		t.Errorf("allowed subjects = %q", c.OIDCAllowedSubjects)
	}
	if _, err := FromLookup(lookup(map[string]string{"AUTH_MODE": "magic"})); err == nil {
		t.Error("bad auth mode accepted")
	}
	if _, err := FromLookup(lookup(map[string]string{"LOG_LEVEL": "loud"})); err == nil {
		t.Error("bad log level accepted")
	}
}

const testSecret = "0123456789abcdef0123456789abcdef"

func oidcEnv(over map[string]string) map[string]string {
	m := map[string]string{
		"AUTH_MODE": "oidc", "OIDC_ISSUER": "https://auth.example", "OIDC_CLIENT_ID": "id",
		"SESSION_SECRET": testSecret, "PUBLIC_BASE_URL": "http://localhost:8080", "OIDC_REQUIRED_ROLE": "owner",
	}
	for k, v := range over {
		m[k] = v
	}
	return m
}

func TestOIDCValidation(t *testing.T) {
	c, err := FromLookup(lookup(oidcEnv(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCRedirectURL != "http://localhost:8080/auth/callback" || c.OIDCRequiredRole != "owner" || c.SecureCookies() {
		t.Errorf("cfg = %+v", c)
	}
	for name, over := range map[string]map[string]string{
		"no authorization rule": {"OIDC_REQUIRED_ROLE": ""},
		"short secret":          {"SESSION_SECRET": "short"},
		"placeholder secret":    {"SESSION_SECRET": "troque-por-32-bytes-aleatorios-em-hex"},
		"relative base url":     {"PUBLIC_BASE_URL": "planner.example"},
		"no base url":           {"PUBLIC_BASE_URL": ""},
		"bad issuer":            {"OIDC_ISSUER": "auth.example"},
		"bad redirect":          {"OIDC_REDIRECT_URL": "/auth/callback"},
	} {
		if _, err := FromLookup(lookup(oidcEnv(over))); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Dev mode ignores the OIDC variables.
	if _, err := FromLookup(lookup(map[string]string{"SESSION_SECRET": "short"})); err != nil {
		t.Errorf("dev: %v", err)
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

// The Microbot agent needs no token in dev; in oidc its machine-user token is
// verified against the project id.
func TestMicrobotControl(t *testing.T) {
	db := map[string]string{"MICROBOT_CONTROL": "on", "DATABASE_URL": "postgres://x"}
	if c, err := FromLookup(lookup(db)); err != nil || !c.MicrobotControl {
		t.Fatalf("dev with control on = %+v %v", c, err)
	}
	if _, err := FromLookup(lookup(map[string]string{"MICROBOT_CONTROL": "on"})); err == nil {
		t.Error("control on without DATABASE_URL: accepted")
	}
	over := map[string]string{"MICROBOT_CONTROL": "on", "DATABASE_URL": "postgres://x"}
	if _, err := FromLookup(lookup(oidcEnv(over))); err == nil || !strings.Contains(err.Error(), "OIDC_PROJECT_ID") {
		t.Errorf("oidc control without a project id = %v", err)
	}
	over["OIDC_PROJECT_ID"] = "proj-1"
	if _, err := FromLookup(lookup(oidcEnv(over))); err != nil {
		t.Errorf("oidc control with a project id = %v", err)
	}
}
