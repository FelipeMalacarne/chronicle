package ingest

import (
	"errors"
	"testing"
)

func TestResolveDevAccount(t *testing.T) {
	one := []DevAccount{{ID: "acc_main", RSNNormalized: "iron_felipe"}}
	two := []DevAccount{{ID: "acc_main", RSNNormalized: "iron_felipe"}, {ID: "acc_alt", RSNNormalized: "alt_one"}}
	cases := []struct {
		name     string
		hint     DevAccountHint
		accounts []DevAccount
		want     string
		err      error
	}{
		{name: "explicit wins over the player", hint: DevAccountHint{Explicit: "acc_alt", Player: "Iron Felipe"}, accounts: two, want: "acc_alt"},
		{name: "unknown explicit is refused, never replaced", hint: DevAccountHint{Explicit: "acc_gone", Player: "Iron Felipe"}, accounts: two, err: ErrNoDevAccount},
		{name: "player name with a non-breaking space", hint: DevAccountHint{Player: "Iron\u00a0Felipe"}, accounts: two, want: "acc_main"},
		{name: "player name in another case", hint: DevAccountHint{Player: "ALT ONE"}, accounts: two, want: "acc_alt"},
		{name: "unknown player falls back to the only account", hint: DevAccountHint{Player: "Someone"}, accounts: one, want: "acc_main"},
		{name: "unknown player with several accounts", hint: DevAccountHint{Player: "Someone"}, accounts: two, err: ErrAmbiguousDevAccount},
		{name: "bound account when there is no player", hint: DevAccountHint{Bound: "acc_alt"}, accounts: two, want: "acc_alt"},
		{name: "bound account that is gone", hint: DevAccountHint{Bound: "acc_gone"}, accounts: two, err: ErrAmbiguousDevAccount},
		{name: "player beats the bound account", hint: DevAccountHint{Player: "Iron Felipe", Bound: "acc_alt"}, accounts: two, want: "acc_main"},
		{name: "no hint and one account", accounts: one, want: "acc_main"},
		{name: "no account at all", hint: DevAccountHint{Player: "Iron Felipe"}, err: ErrNoDevAccount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDevAccount(tc.hint, tc.accounts)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("ResolveDevAccount = %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestNormalizePlayerName(t *testing.T) {
	for in, want := range map[string]string{
		"Iron Felipe":        "iron_felipe",
		" Iron\u00a0Felipe ": "iron_felipe",
		"iron  felipe":       "iron_felipe",
		"Zezima":             "zezima",
		"":                   "",
	} {
		if got := NormalizePlayerName(in); got != want {
			t.Errorf("NormalizePlayerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidInstallationID(t *testing.T) {
	for id, want := range map[string]bool{
		"3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c": true,
		"3F2B8C1E-9A4D-4E7B-8C2A-1D5E6F7A8B9C": false,
		"3f2b8c1e9a4d4e7b8c2a1d5e6f7a8b9c":     false,
		"":                                     false,
		"ipl_secret":                           false,
	} {
		if got := ValidInstallationID(id); got != want {
			t.Errorf("ValidInstallationID(%q) = %v, want %v", id, got, want)
		}
	}
}
