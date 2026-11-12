package vault

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func keyOf(t *testing.T, b byte) (Key, []byte) {
	t.Helper()
	raw := bytes.Repeat([]byte{b}, KeySize)
	k, err := ParseKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return k, raw
}

func TestParseKey(t *testing.T) {
	raw := bytes.Repeat([]byte{7}, KeySize)
	sum := sha256.Sum256(raw)
	for _, tc := range []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"padded", base64.StdEncoding.EncodeToString(raw), false},
		{"unpadded", base64.RawStdEncoding.EncodeToString(raw), false},
		{"surrounding space", "  " + base64.StdEncoding.EncodeToString(raw) + "\n", false},
		{"short", base64.StdEncoding.EncodeToString(raw[:16]), true},
		{"long", base64.StdEncoding.EncodeToString(append(raw, 1)), true},
		{"empty", "", true},
		{"not base64", "not*base64!", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, err := ParseKey(tc.in)
			if tc.wantErr {
				if err == nil || k.Valid() {
					t.Fatalf("ParseKey(%q) = %v, %v; want an error", tc.in, k, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := hex.EncodeToString(sum[:])[:8]; k.ID() != want {
				t.Fatalf("ID = %q, want %q", k.ID(), want)
			}
		})
	}
}

func TestSealOpen(t *testing.T) {
	k, raw := keyOf(t, 1)
	other, _ := keyOf(t, 2)
	plaintext := []byte(`{"session_id":"s"}`)
	nonce, ciphertext, err := k.Seal(plaintext, []byte("acc_a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) != NonceSize || bytes.Contains(ciphertext, plaintext) {
		t.Fatalf("nonce %d bytes, ciphertext %x", len(nonce), ciphertext)
	}
	tampered := append([]byte(nil), ciphertext...)
	tampered[0] ^= 1
	for _, tc := range []struct {
		name      string
		key       Key
		nonce, ct []byte
		aad       string
		wantErr   error
		wantPlain []byte
	}{
		{"round trip", k, nonce, ciphertext, "acc_a", nil, plaintext},
		{"same key parsed again", mustParse(t, raw), nonce, ciphertext, "acc_a", nil, plaintext},
		{"other aad", k, nonce, ciphertext, "acc_b", ErrOpen, nil},
		{"empty aad", k, nonce, ciphertext, "", ErrOpen, nil},
		{"wrong key", other, nonce, ciphertext, "acc_a", ErrOpen, nil},
		{"tampered", k, nonce, tampered, "acc_a", ErrOpen, nil},
		{"short nonce", k, nonce[:8], ciphertext, "acc_a", ErrOpen, nil},
		{"zero key", Key{}, nonce, ciphertext, "acc_a", ErrNoKey, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.key.Open(tc.nonce, tc.ct, []byte(tc.aad))
			if !errors.Is(err, tc.wantErr) || !bytes.Equal(got, tc.wantPlain) {
				t.Fatalf("Open = %q, %v; want %q, %v", got, err, tc.wantPlain, tc.wantErr)
			}
		})
	}
}

func TestSealUsesFreshNonces(t *testing.T) {
	k, _ := keyOf(t, 3)
	n1, c1, err := k.Seal([]byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	n2, c2, err := k.Seal([]byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(n1, n2) || bytes.Equal(c1, c2) {
		t.Fatal("two seals share a nonce")
	}
	if _, _, err := (Key{}).Seal([]byte("x"), nil); !errors.Is(err, ErrNoKey) {
		t.Fatalf("zero key Seal = %v", err)
	}
}

func TestKeyNeverPrintsItself(t *testing.T) {
	k, raw := keyOf(t, 4)
	for _, s := range []string{fmt.Sprint(k), fmt.Sprintf("%v %+v %#v %s", k, k, k, k)} {
		if strings.Contains(s, base64.StdEncoding.EncodeToString(raw)) || strings.Contains(s, string(raw)) || !strings.Contains(s, k.ID()) {
			t.Fatalf("printed key: %q", s)
		}
	}
}

func mustParse(t *testing.T, raw []byte) Key {
	t.Helper()
	k, err := ParseKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return k
}
