package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// FlowTTL bounds the time between /auth/login and /auth/callback.
const FlowTTL = 10 * time.Minute

// ErrBadFlow rejects a missing, tampered or expired login state.
var ErrBadFlow = errors.New("auth: invalid or expired login state")

// Flow is what /auth/callback needs from /auth/login: the state and nonce it
// sent, the PKCE verifier and where to land afterwards.
type Flow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
	Issued   int64  `json:"t"`
}

// Sealer encrypts the flow cookie with AES-256-GCM under a key derived from
// SESSION_SECRET, so the browser carries it without being able to read or
// change it and no database row exists before the login succeeds.
type Sealer struct {
	aead cipher.AEAD
	now  func() time.Time
}

// NewSealer derives the cookie key from secret.
func NewSealer(secret string) (*Sealer, error) {
	if secret == "" {
		return nil, errors.New("auth: empty session secret")
	}
	key := sha256.Sum256([]byte("ironman-planner/oidc-flow/v1\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, now: time.Now}, nil
}

// Seal stamps and encrypts f.
func (s *Sealer) Seal(f Flow) (string, error) {
	f.Issued = s.now().Unix()
	plain, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plain, nil)), nil
}

// Open decrypts a sealed flow and checks its age.
func (s *Sealer) Open(sealed string) (Flow, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return Flow{}, ErrBadFlow
	}
	n := s.aead.NonceSize()
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return Flow{}, ErrBadFlow
	}
	var f Flow
	if json.Unmarshal(plain, &f) != nil || f.State == "" || f.Nonce == "" || f.Verifier == "" {
		return Flow{}, ErrBadFlow
	}
	age := s.now().Sub(time.Unix(f.Issued, 0))
	if age < -time.Minute || age > FlowTTL {
		return Flow{}, ErrBadFlow
	}
	return f, nil
}

// NewFlow starts one login: fresh state, nonce and PKCE verifier, and the
// sanitized destination.
func NewFlow(returnTo string) Flow {
	return Flow{State: RandomToken(), Nonce: RandomToken(), Verifier: oauth2.GenerateVerifier(), ReturnTo: SafeReturnPath(returnTo)}
}

// RandomToken returns 32 random bytes in base64url (state and nonce).
func RandomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// SafeReturnPath keeps a post-login destination only when it is a path on
// this site ("/plan?x=1"); anything else (absolute or protocol-relative URLs,
// backslashes, the /auth routes themselves) becomes "/".
func SafeReturnPath(raw string) string {
	if raw == "" || raw[0] != '/' || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	if u.Path == "/auth" || strings.HasPrefix(u.Path, "/auth/") {
		return "/"
	}
	return raw
}
