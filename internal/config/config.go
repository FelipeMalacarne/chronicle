// Package config loads the ironman configuration from environment variables.
//
// Every setting is an environment variable; there is no config file. The
// variable names below are a contract shared with .env.example, the Docker
// image and the compose file, so they must not be renamed.
package config

import (
	"fmt"
	"log/slog"
	"os"
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

	// OIDCRedirectURL is the callback URL registered at the issuer, normally
	// PUBLIC_BASE_URL + "/auth/callback". Required when AUTH_MODE=oidc.
	// Env: OIDC_REDIRECT_URL.
	OIDCRedirectURL string

	// SessionSecret is the key used to sign/encrypt session cookies. Any long
	// random string (32+ bytes). Required when AUTH_MODE=oidc.
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
	// trailing slash), used to build absolute links and the default OIDC
	// redirect. Optional; defaults to http://localhost<Addr> semantics in the UI.
	// Env: PUBLIC_BASE_URL.
	PublicBaseURL string
}

// Defaults applied when a variable is unset or empty.
const (
	DefaultAddr       = ":8080"
	DefaultAuthMode   = "dev"
	DefaultContentDir = "content"
	DefaultLogLevel   = "info"
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
		Addr:            get("IRONMAN_ADDR", DefaultAddr),
		DatabaseURL:     get("DATABASE_URL", ""),
		AuthMode:        strings.ToLower(get("AUTH_MODE", DefaultAuthMode)),
		OIDCIssuer:      get("OIDC_ISSUER", ""),
		OIDCClientID:    get("OIDC_CLIENT_ID", ""),
		OIDCRedirectURL: get("OIDC_REDIRECT_URL", ""),
		SessionSecret:   get("SESSION_SECRET", ""),
		ContentDir:      get("CONTENT_DIR", DefaultContentDir),
		LogLevel:        strings.ToLower(get("LOG_LEVEL", DefaultLogLevel)),
		PublicBaseURL:   strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/"),
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
			{"OIDC_REDIRECT_URL", c.OIDCRedirectURL},
			{"SESSION_SECRET", c.SessionSecret},
		} {
			if kv.v == "" {
				missing = append(missing, kv.k)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("config: AUTH_MODE=oidc requires %s", strings.Join(missing, ", "))
		}
	default:
		return fmt.Errorf("config: AUTH_MODE must be %q or %q, got %q", AuthModeDev, AuthModeOIDC, c.AuthMode)
	}
	if _, err := c.SlogLevel(); err != nil {
		return err
	}
	return nil
}

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
