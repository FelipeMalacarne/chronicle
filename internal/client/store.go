package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultURL is the server a client talks to when nothing else says: the
// planner of `just dev` on the same machine.
const DefaultURL = "http://localhost:8080"

// Settings are the client's defaults, in <dir>/config.json. Flags and the
// IRONMAN_URL and IRONMAN_ACCOUNT variables override them.
type Settings struct {
	URL     string `json:"url,omitempty"`
	Account string `json:"account,omitempty"`
}

// Credential is what `ironman login` keeps for one server: the device flow's
// client and its tokens. The access token is renewed with the refresh token.
type Credential struct {
	Issuer       string    `json:"issuer"`
	ClientID     string    `json:"client_id"`
	TokenURL     string    `json:"token_url"`
	Scopes       []string  `json:"scopes,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`
}

// Store is the client's configuration directory: config.json (Settings) and
// credentials.json (Credential per server URL, mode 0600).
type Store struct {
	Dir string
}

// DefaultDir is $IRONMAN_CONFIG_DIR, else <user config dir>/ironman
// (~/.config/ironman on Linux).
func DefaultDir() (string, error) {
	if dir := os.Getenv("IRONMAN_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("client: config directory: %w", err)
	}
	return filepath.Join(base, "ironman"), nil
}

// NormalizeURL is the key of a server: no trailing slash.
func NormalizeURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }

// Settings reads config.json; a missing file is the zero Settings.
func (s Store) Settings() (Settings, error) {
	var out Settings
	err := s.read("config.json", &out)
	return out, err
}

// SaveSettings writes config.json.
func (s Store) SaveSettings(v Settings) error {
	v.URL = NormalizeURL(v.URL)
	return s.write("config.json", v)
}

// Credential returns the credential of the server at url.
func (s Store) Credential(url string) (Credential, bool, error) {
	all := map[string]Credential{}
	if err := s.read("credentials.json", &all); err != nil {
		return Credential{}, false, err
	}
	c, ok := all[NormalizeURL(url)]
	return c, ok, nil
}

// SaveCredential stores the credential of the server at url.
func (s Store) SaveCredential(url string, c Credential) error {
	all := map[string]Credential{}
	if err := s.read("credentials.json", &all); err != nil {
		return err
	}
	all[NormalizeURL(url)] = c
	return s.write("credentials.json", all)
}

// DeleteCredential forgets the server at url; it reports whether there was
// one.
func (s Store) DeleteCredential(url string) (bool, error) {
	all := map[string]Credential{}
	if err := s.read("credentials.json", &all); err != nil {
		return false, err
	}
	key := NormalizeURL(url)
	if _, ok := all[key]; !ok {
		return false, nil
	}
	delete(all, key)
	return true, s.write("credentials.json", all)
}

func (s Store) read(name string, into any) error {
	b, err := os.ReadFile(filepath.Join(s.Dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("client: read %s: %w", name, err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		return fmt.Errorf("client: %s: %w", filepath.Join(s.Dir, name), err)
	}
	return nil
}

// write replaces the file atomically; both files are private to the user
// (the credentials hold a refresh token).
func (s Store) write(name string, v any) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("client: config directory: %w", err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, name+".*")
	if err != nil {
		return fmt.Errorf("client: write %s: %w", name, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("client: write %s: %w", name, err)
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("client: write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("client: write %s: %w", name, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.Dir, name)); err != nil {
		return fmt.Errorf("client: write %s: %w", name, err)
	}
	return nil
}
