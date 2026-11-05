package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Credential is how a plugin request authenticates: an ipl_ token, or (only
// in AUTH_MODE=dev, from loopback) a tokenless installation. The transport
// decides which one a request may use; the repository accepts the tokenless
// form only when WithDevInstallations enabled it.
type Credential struct {
	// Token is the plaintext ipl_ token; empty for a dev installation.
	Token string
	// Installation is the plugin's installation id (dev installations).
	Installation string
	// Account is the account id the plugin's "Conta" setting names; empty
	// lets the server resolve it (coreingest.ResolveDevAccount).
	Account string
}

// Tokenless reports whether the credential is a dev installation.
func (c Credential) Tokenless() bool { return c.Token == "" }

// Principal is the account a plugin credential acts for and what it may do.
type Principal struct {
	AccountID string
	Scopes    []coreingest.TokenScope
	// Tokenless: a dev installation (AUTH_MODE=dev, loopback).
	Tokenless bool
}

// Allows reports whether the principal has the scope.
func (p Principal) Allows(scope coreingest.TokenScope) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// devScopes are the scopes of a dev installation: everything, because the
// server is the owner's and only listens on loopback. The plugin's own item
// toggles stay the second opt-in.
var devScopes = []coreingest.TokenScope{
	coreingest.ScopeItemsBank, coreingest.ScopeItemsEquipment, coreingest.ScopeItemsInventory,
	coreingest.ScopeMarksWrite, coreingest.ScopeStateWrite,
}

// WithDevInstallations accepts tokenless dev installations (AUTH_MODE=dev
// only; the transport also requires a loopback connection).
func (r *Repository) WithDevInstallations(on bool) *Repository {
	r.devInstallations = on
	return r
}

// credentialRow is the locked or read state of a token or an installation.
type credentialRow struct {
	ID              string
	AccountID       string
	Scopes          []coreingest.TokenScope
	ManifestVersion int64
	LastSeq         int64
}

func (r *Repository) checkCredential(c Credential) error {
	if c.Tokenless() {
		if !r.devInstallations || !coreingest.ValidInstallationID(c.Installation) {
			return ErrUnauthorized
		}
	}
	return nil
}

// Principal returns the account a credential acts for.
func (r *Repository) Principal(ctx context.Context, c Credential) (Principal, error) {
	if err := r.checkCredential(c); err != nil {
		return Principal{}, err
	}
	if !c.Tokenless() {
		token, err := r.queries.GetPluginTokenByHash(ctx, text(tokenHash(c.Token)))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Principal{}, ErrUnauthorized
			}
			return Principal{}, fmt.Errorf("get ingest token: %w", err)
		}
		scopes, err := decodeScopes(token.Scopes)
		if err != nil {
			return Principal{}, err
		}
		return Principal{AccountID: token.AccountID, Scopes: scopes}, nil
	}
	bound := ""
	latest, err := r.queries.GetLatestPluginInstallation(ctx, text(c.Installation))
	switch {
	case err == nil:
		bound = latest.AccountID
	case !errors.Is(err, pgx.ErrNoRows):
		return Principal{}, fmt.Errorf("get plugin installation: %w", err)
	}
	account, err := r.resolveDevAccount(ctx, r.queries, coreingest.DevAccountHint{Explicit: c.Account, Bound: bound})
	if err != nil {
		return Principal{}, err
	}
	return Principal{AccountID: account, Scopes: append([]coreingest.TokenScope(nil), devScopes...), Tokenless: true}, nil
}

// manifestFor builds the manifest of a credential: a token's scopes, or the
// dev scopes; resync unless the credential last pushed with this version.
func (r *Repository) manifestFor(ctx context.Context, c Credential) (coreingest.Manifest, error) {
	if err := r.checkCredential(c); err != nil {
		return coreingest.Manifest{}, err
	}
	if !c.Tokenless() {
		token, err := r.queries.GetPluginTokenByHash(ctx, text(tokenHash(c.Token)))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return coreingest.Manifest{}, ErrUnauthorized
			}
			return coreingest.Manifest{}, fmt.Errorf("get ingest token: %w", err)
		}
		scopes, err := decodeScopes(token.Scopes)
		if err != nil {
			return coreingest.Manifest{}, err
		}
		manifest := r.definition.manifest(scopes, int64(token.ManifestVersion) != r.definition.Version)
		return manifest, manifest.Validate()
	}
	// The manifest does not depend on the account: a tokenless manifest is
	// answered before login, when nothing names the character yet.
	resync := true
	latest, err := r.queries.GetLatestPluginInstallation(ctx, text(c.Installation))
	switch {
	case err == nil:
		resync = int64(latest.ManifestVersion) != r.definition.Version
	case !errors.Is(err, pgx.ErrNoRows):
		return coreingest.Manifest{}, fmt.Errorf("get plugin installation: %w", err)
	}
	manifest := r.definition.manifest(append([]coreingest.TokenScope(nil), devScopes...), resync)
	return manifest, manifest.Validate()
}

// lockCredential locks the protocol row of a push: the token's, or the dev
// installation's record for the push's account (created on first use).
func (r *Repository) lockCredential(ctx context.Context, q *storedb.Queries, c Credential, push coreingest.Push) (credentialRow, error) {
	if err := r.checkCredential(c); err != nil {
		return credentialRow{}, err
	}
	if !c.Tokenless() {
		token, err := q.GetPluginTokenByHashForUpdate(ctx, text(tokenHash(c.Token)))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return credentialRow{}, ErrUnauthorized
			}
			return credentialRow{}, fmt.Errorf("lock ingest token: %w", err)
		}
		scopes, err := decodeScopes(token.Scopes)
		if err != nil {
			return credentialRow{}, err
		}
		return credentialRow{ID: token.ID, AccountID: token.AccountID, Scopes: scopes, ManifestVersion: int64(token.ManifestVersion), LastSeq: token.LastSeq}, nil
	}
	account, err := r.resolveDevAccount(ctx, q, coreingest.DevAccountHint{Explicit: c.Account, Player: push.Username})
	if err != nil {
		return credentialRow{}, err
	}
	id, err := randomText("pin_", 16)
	if err != nil {
		return credentialRow{}, fmt.Errorf("generate installation record id: %w", err)
	}
	rawScopes, err := encodeScopes(devScopes)
	if err != nil {
		return credentialRow{}, err
	}
	if err = q.EnsurePluginInstallation(ctx, storedb.EnsurePluginInstallationParams{
		ID: id, AccountID: account, InstallationID: text(c.Installation),
		Label: text("dev installation"), Scopes: rawScopes,
	}); err != nil {
		return credentialRow{}, fmt.Errorf("record plugin installation: %w", err)
	}
	row, err := q.GetPluginInstallationForUpdate(ctx, storedb.GetPluginInstallationForUpdateParams{
		AccountID: account, InstallationID: text(c.Installation),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// revoked record or inactive account
			return credentialRow{}, ErrUnauthorized
		}
		return credentialRow{}, fmt.Errorf("lock plugin installation: %w", err)
	}
	return credentialRow{ID: row.ID, AccountID: row.AccountID, Scopes: append([]coreingest.TokenScope(nil), devScopes...), ManifestVersion: int64(row.ManifestVersion), LastSeq: row.LastSeq}, nil
}

// resolveDevAccount maps the request's hint onto the active accounts.
func (r *Repository) resolveDevAccount(ctx context.Context, q *storedb.Queries, hint coreingest.DevAccountHint) (string, error) {
	rows, err := q.ListActiveAccounts(ctx)
	if err != nil {
		return "", fmt.Errorf("list active accounts: %w", err)
	}
	accounts := make([]coreingest.DevAccount, 0, len(rows))
	for _, a := range rows {
		accounts = append(accounts, coreingest.DevAccount{ID: a.ID, RSNNormalized: a.RsnNormalized})
	}
	id, err := coreingest.ResolveDevAccount(hint, accounts)
	switch {
	case errors.Is(err, coreingest.ErrNoDevAccount):
		return "", ErrAccountNotFound
	case errors.Is(err, coreingest.ErrAmbiguousDevAccount):
		return "", ErrAccountAmbiguous
	}
	return id, err
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}
