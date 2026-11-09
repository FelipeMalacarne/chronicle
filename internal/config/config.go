// Package config loads the ironman configuration from environment variables.
//
// Every setting is an environment variable; there is no config file. The
// variable names below are a contract shared with .env.example, the Docker
// image and the compose file, so they must not be renamed.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the fully resolved configuration of the ironman binary.
type Config struct {
	// Addr is the listen address of `ironman serve`.
	// Env: IRONMAN_ADDR. Default ":8080".
	Addr string

	// DatabaseURL is the Postgres connection string (pgx/libpq format, e.g.
	// postgres://user:pass@host:5432/ironman?sslmode=disable). Optional: when
	// empty the server starts without a database and /api/health reports
	// "db":"not_configured".
	// Env: DATABASE_URL.
	DatabaseURL string

	// AuthMode selects authentication: "dev" (no login, single local user) or
	// "oidc" (OpenID Connect; requires the OIDC_* variables and SESSION_SECRET).
	// Env: AUTH_MODE. Default "dev".
	AuthMode string

	// OIDCIssuer is the OpenID Connect issuer URL (e.g. https://auth.example.com).
	// Required when AUTH_MODE=oidc.
	// Env: OIDC_ISSUER.
	OIDCIssuer string

	// OIDCClientID is the OAuth2 client id registered at the issuer.
	// Required when AUTH_MODE=oidc.
	// Env: OIDC_CLIENT_ID.
	OIDCClientID string

	// OIDCRedirectURL is the callback URL registered at the issuer. Defaults to
	// PUBLIC_BASE_URL + "/auth/callback" when AUTH_MODE=oidc.
	// Env: OIDC_REDIRECT_URL.
	OIDCRedirectURL string

	// OIDCAllowedSubjects lists the OIDC `sub` values allowed to log in
	// (comma separated). With OIDCRequiredRole also set, a login needs both a
	// listed subject and the role; AUTH_MODE=oidc needs at least one of the two.
	// Env: OIDC_ALLOWED_SUBJECTS.
	OIDCAllowedSubjects []string

	// OIDCRequiredRole is the Zitadel project role that allows a login (read
	// from the urn:zitadel:iam:org:project[:<id>]:roles claims of the ID token).
	// Env: OIDC_REQUIRED_ROLE.
	OIDCRequiredRole string

	// OIDCProjectID pins the project-scoped role claim
	// (urn:zitadel:iam:org:project:<id>:roles) to the planner's Zitadel
	// project; empty accepts any project id. Env: OIDC_PROJECT_ID.
	OIDCProjectID string

	// OIDCNativeClientID is the public native application (Device Code grant,
	// no secret) that the CLI, `ironman mcp` and, later, the plugin log in
	// with (docs/04 section 2.5). Its JWT access tokens are accepted as
	// `Authorization: Bearer` on the account routes; empty disables bearer
	// login, leaving only the web session. Env: OIDC_NATIVE_CLIENT_ID.
	OIDCNativeClientID string

	// SessionSecret keys the encrypted cookie that carries the login state
	// (state, nonce, PKCE verifier) between /auth/login and /auth/callback; the
	// session itself is a random token over the sessions table. 32+ characters
	// of random data. Required when AUTH_MODE=oidc.
	// Env: SESSION_SECRET.
	SessionSecret string

	// ContentDir is the directory holding the guide content, laid out as
	// <ContentDir>/guides/<guide-id>/guide.json.
	// Env: CONTENT_DIR. Default "content".
	ContentDir string

	// LogLevel is the minimum slog level: "debug", "info", "warn" or "error".
	// Env: LOG_LEVEL. Default "info".
	LogLevel string

	// PublicBaseURL is the URL users reach the server at (scheme + host, no
	// trailing slash), used to build absolute links, the default OIDC redirect
	// and the post-logout redirect; an https URL makes the session cookie
	// Secure. Required when AUTH_MODE=oidc.
	// Env: PUBLIC_BASE_URL.
	PublicBaseURL string

	// SyncHiscoresMinutes is the cadence of the hiscores scheduler per
	// account (docs/04 section 6.1); 0 disables it (POST /api/sync still works).
	// Env: SYNC_HISCORES_MINUTES. Default 30.
	SyncHiscoresMinutes int

	// MicrobotControl turns on the Microbot control module (docs/08): the
	// agent link and every /api/microbot route. Off by default; with it off
	// the planner behaves exactly as without the module.
	// Env: MICROBOT_CONTROL (on|off). Default off.
	// The agent authenticates by mode: in dev only from loopback, with no
	// token; in oidc with a Zitadel machine user's JWT access token, whose
	// audience is OIDC_PROJECT_ID and which carries the microbot:agent role.
	MicrobotControl bool
}

// Defaults applied when a variable is unset or empty.
const (
	DefaultAddr       = ":8080"
	DefaultAuthMode   = "dev"
	DefaultContentDir = "content"
	DefaultLogLevel   = "info"

	DefaultSyncHiscoresMinutes = 30
)

// Auth modes accepted in AUTH_MODE.
const (
	AuthModeDev  = "dev"
	AuthModeOIDC = "oidc"
)

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup builds a Config using lookup (same contract as os.LookupEnv).
// It applies defaults and validates cross-field requirements.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	c := Config{
		Addr:               get("IRONMAN_ADDR", DefaultAddr),
		DatabaseURL:        get("DATABASE_URL", ""),
		AuthMode:           strings.ToLower(get("AUTH_MODE", DefaultAuthMode)),
		OIDCIssuer:         get("OIDC_ISSUER", ""),
		OIDCClientID:       get("OIDC_CLIENT_ID", ""),
		OIDCRedirectURL:    get("OIDC_REDIRECT_URL", ""),
		OIDCRequiredRole:   get("OIDC_REQUIRED_ROLE", ""),
		OIDCProjectID:      get("OIDC_PROJECT_ID", ""),
		OIDCNativeClientID: get("OIDC_NATIVE_CLIENT_ID", ""),
		SessionSecret:      get("SESSION_SECRET", ""),
		ContentDir:         get("CONTENT_DIR", DefaultContentDir),
		LogLevel:           strings.ToLower(get("LOG_LEVEL", DefaultLogLevel)),
		PublicBaseURL:      strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/"),
	}
	minutes, err := strconv.Atoi(get("SYNC_HISCORES_MINUTES", strconv.Itoa(DefaultSyncHiscoresMinutes)))
	if err != nil || minutes < 0 {
		return c, fmt.Errorf("config: SYNC_HISCORES_MINUTES must be a whole number of minutes, 0 or more")
	}
	c.SyncHiscoresMinutes = minutes
	switch strings.ToLower(get("MICROBOT_CONTROL", "off")) {
	case "on", "true", "1":
		c.MicrobotControl = true
	case "off", "false", "0", "":
	default:
		return c, fmt.Errorf("config: MICROBOT_CONTROL must be on or off")
	}
	for _, sub := range strings.Split(get("OIDC_ALLOWED_SUBJECTS", ""), ",") {
		if sub = strings.TrimSpace(sub); sub != "" {
			c.OIDCAllowedSubjects = append(c.OIDCAllowedSubjects, sub)
		}
	}
	if c.AuthMode == AuthModeOIDC && c.OIDCRedirectURL == "" && c.PublicBaseURL != "" {
		c.OIDCRedirectURL = c.PublicBaseURL + "/auth/callback"
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// Validate checks enum values and the variables required by the chosen auth mode.
func (c Config) Validate() error {
	switch c.AuthMode {
	case AuthModeDev:
	case AuthModeOIDC:
		var missing []string
		for _, kv := range []struct{ k, v string }{
			{"OIDC_ISSUER", c.OIDCIssuer},
			{"OIDC_CLIENT_ID", c.OIDCClientID},
			{"PUBLIC_BASE_URL", c.PublicBaseURL},
			{"SESSION_SECRET", c.SessionSecret},
		} {
			if kv.v == "" {
				missing = append(missing, kv.k)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("config: AUTH_MODE=oidc requires %s", strings.Join(missing, ", "))
		}
		if err := c.validateOIDC(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("config: AUTH_MODE must be %q or %q, got %q", AuthModeDev, AuthModeOIDC, c.AuthMode)
	}
	if _, err := c.SlogLevel(); err != nil {
		return err
	}
	if c.MicrobotControl && c.AuthMode == AuthModeOIDC && c.OIDCProjectID == "" {
		return fmt.Errorf("config: MICROBOT_CONTROL=on with AUTH_MODE=oidc needs OIDC_PROJECT_ID (the audience of the agent's machine-user token)")
	}
	if c.MicrobotControl && c.DatabaseURL == "" {
		return fmt.Errorf("config: MICROBOT_CONTROL=on needs DATABASE_URL (the control audit lives in Postgres)")
	}
	return nil
}

// MinSessionSecret is the shortest SESSION_SECRET accepted in AUTH_MODE=oidc.
const MinSessionSecret = 32

// placeholderSecret is the value .env.example ships with.
const placeholderSecret = "troque-por-32-bytes-aleatorios-em-hex"

func (c Config) validateOIDC() error {
	for _, kv := range []struct{ k, v string }{
		{"OIDC_ISSUER", c.OIDCIssuer},
		{"PUBLIC_BASE_URL", c.PublicBaseURL},
		{"OIDC_REDIRECT_URL", c.OIDCRedirectURL},
	} {
		u, err := url.Parse(kv.v)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("config: %s must be an absolute http(s) URL, got %q", kv.k, kv.v)
		}
	}
	if len(c.SessionSecret) < MinSessionSecret || c.SessionSecret == placeholderSecret {
		return fmt.Errorf("config: SESSION_SECRET must be %d+ random characters (openssl rand -hex 32)", MinSessionSecret)
	}
	if len(c.OIDCAllowedSubjects) == 0 && c.OIDCRequiredRole == "" {
		return fmt.Errorf("config: AUTH_MODE=oidc requires OIDC_ALLOWED_SUBJECTS or OIDC_REQUIRED_ROLE (nobody could log in)")
	}
	return nil
}

// SecureCookies reports whether cookies must carry the Secure attribute: the
// public URL is https.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicBaseURL, "https://") }

// SlogLevel parses LogLevel into a slog.Level.
func (c Config) SlogLevel() (slog.Level, error) {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("config: LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel)
}

// IsDev reports whether the server runs in dev auth mode.
func (c Config) IsDev() bool { return c.AuthMode == AuthModeDev }
