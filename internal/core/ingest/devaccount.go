package ingest

import (
	"errors"
	"regexp"
	"strings"
)

// Errors of the tokenless dev account resolution.
var (
	// ErrNoDevAccount: no active account fits the request (unknown explicit
	// account, or no account at all).
	ErrNoDevAccount = errors.New("no dev account for this request")
	// ErrAmbiguousDevAccount: several accounts exist and nothing in the
	// request picks one; the plugin's "Conta" setting must name it.
	ErrAmbiguousDevAccount = errors.New("several dev accounts: set the plugin's account")
)

// DevAccount is an active account the tokenless dev path may pick.
type DevAccount struct {
	ID string
	// RSNNormalized is accounts.rsn_normalized (lower case, spaces as "_").
	RSNNormalized string
}

// DevAccountHint is what a tokenless dev request says about its account.
type DevAccountHint struct {
	// Explicit is the account id of the plugin's "Conta" setting.
	Explicit string
	// Player is the logged-in character's name (push.username).
	Player string
	// Bound is the account the installation pushed to most recently.
	Bound string
}

// ResolveDevAccount picks the account of a tokenless AUTH_MODE=dev request
// (docs/04 section 2.5): the explicit account when set, else the account of
// the logged-in character, else the installation's bound account, else the
// only active account. Nothing is guessed beyond that: with several accounts
// and no hint it is ErrAmbiguousDevAccount.
func ResolveDevAccount(hint DevAccountHint, accounts []DevAccount) (string, error) {
	has := func(id string) bool {
		for _, a := range accounts {
			if a.ID == id {
				return true
			}
		}
		return false
	}
	if explicit := strings.TrimSpace(hint.Explicit); explicit != "" {
		if has(explicit) {
			return explicit, nil
		}
		return "", ErrNoDevAccount
	}
	if player := NormalizePlayerName(hint.Player); player != "" {
		for _, a := range accounts {
			if a.RSNNormalized == player {
				return a.ID, nil
			}
		}
	}
	if hint.Bound != "" && has(hint.Bound) {
		return hint.Bound, nil
	}
	switch len(accounts) {
	case 0:
		return "", ErrNoDevAccount
	case 1:
		return accounts[0].ID, nil
	default:
		return "", ErrAmbiguousDevAccount
	}
}

// NormalizePlayerName is the accounts.rsn_normalized key of a name as the
// game client reports it: RuneLite names use non-breaking spaces, which count
// as spaces; then lower case with spaces as "_" (docs/04 section 6.3).
func NormalizePlayerName(name string) string {
	name = strings.ReplaceAll(name, "\u00a0", " ")
	name = strings.Join(strings.Fields(name), " ")
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

var installationPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidInstallationID reports whether id is a canonical lower-case UUID, the
// form the plugin generates once and keeps in its config.
func ValidInstallationID(id string) bool { return installationPattern.MatchString(id) }
