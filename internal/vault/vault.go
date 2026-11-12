// Package vault seals small secrets for the database with AES-256-GCM under
// CHRONICLE_VAULT_KEY (docs/11 sections 4.2 and 13.2). It is pure: callers
// store the nonce, the ciphertext and the key id themselves.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// KeySize is the size of a vault key: AES-256.
const KeySize = 32

// NonceSize is the size of the GCM nonce Seal returns.
const NonceSize = 12

var (
	// ErrNoKey: Seal or Open on the zero Key.
	ErrNoKey = errors.New("vault: no key")
	// ErrOpen: the ciphertext does not open with this key and additional
	// data (wrong key, another account's row, or tampered bytes).
	ErrOpen = errors.New("vault: ciphertext does not open with this key and additional data")
)

// Key is a parsed vault key. The zero Key seals nothing. Its String never
// shows the key, only its id.
type Key struct {
	aead cipher.AEAD
	id   string
}

// ParseKey reads a base64 key (standard alphabet, padded or not) of exactly
// KeySize bytes.
func ParseKey(encoded string) (Key, error) {
	encoded = strings.TrimSpace(encoded)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		var rawErr error
		if raw, rawErr = base64.RawStdEncoding.DecodeString(encoded); rawErr != nil {
			return Key{}, fmt.Errorf("vault: key is not base64: %w", err)
		}
	}
	if len(raw) != KeySize {
		return Key{}, fmt.Errorf("vault: key has %d bytes, want %d", len(raw), KeySize)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return Key{}, fmt.Errorf("vault: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Key{}, fmt.Errorf("vault: %w", err)
	}
	sum := sha256.Sum256(raw)
	return Key{aead: aead, id: hex.EncodeToString(sum[:4])}, nil
}

// ID is the key's id: the first 8 hex digits of its SHA-256, stored next to
// each sealed value so a rotated key is detected instead of failing to open.
// Empty for the zero Key.
func (k Key) ID() string { return k.id }

// Valid reports whether k is a parsed key.
func (k Key) Valid() bool { return k.aead != nil }

// String shows the id only.
func (k Key) String() string { return "vault.Key(" + k.id + ")" }

// GoString shows the id only, also under %#v.
func (k Key) GoString() string { return k.String() }

// Seal encrypts plaintext with a fresh random nonce. aad is authenticated but
// not encrypted: Open must be given the same bytes.
func (k Key) Seal(plaintext, aad []byte) (nonce, ciphertext []byte, err error) {
	if k.aead == nil {
		return nil, nil, ErrNoKey
	}
	nonce = make([]byte, NonceSize)
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("vault: nonce: %w", err)
	}
	return nonce, k.aead.Seal(nil, nonce, plaintext, aad), nil
}

// Open decrypts what Seal returned for the same aad.
func (k Key) Open(nonce, ciphertext, aad []byte) ([]byte, error) {
	if k.aead == nil {
		return nil, ErrNoKey
	}
	if len(nonce) != NonceSize {
		return nil, ErrOpen
	}
	plaintext, err := k.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}
