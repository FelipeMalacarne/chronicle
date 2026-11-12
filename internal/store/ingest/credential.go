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

// Credential is how a plugin request authenticates: an ipl_ token, or an
// installation without one. An installation is either a dev one (only in
// AUTH_MODE=dev, from loopback) or one signed in as a planner user through
// OIDC (AUTH_MODE=oidc, the device flow; docs/04 section 2.5). The transport
// decides which one a request may use; the repository accepts the dev form
// only when WithDevInstallations enabled it.
type Credential struct {
	// Token is the plaintext ipl_ token; empty for an installation.
	Token string
	// Installation is the plugin's installation id (installations).
	Installation string
	// Account is the account id the plugin's "Conta" setting names; empty
	// lets the server resolve it (coreingest.ResolveDevAccount).
	Account string
	// User is the planner user an OIDC installation signed in as: it acts
	// only on that user's accounts, with Scopes from the user's project
	// roles. Empty for a dev installation.
	User   string
	Scopes []coreingest.TokenScope
}

// Tokenless reports whether the credential is an installation (dev or OIDC).
func (c Credential) Tokenless() bool { return c.Token == "" }

// scopes of an installation: everything for a dev one, the roles' for an
// OIDC one.
func (c Credential) scopes() []coreingest.TokenScope {
	if c.User == "" {
		return append([]coreingest.TokenScope(nil), devScopes...)
	}
	return append([]coreingest.TokenScope(nil), c.Scopes...)
}

// Principal is the account a plugin credential acts for and what it may do.
type Principal struct {
	AccountID string
	Scopes    []coreingest.TokenScope
	// Tokenless: an installation (dev, or signed in through OIDC).
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
		if !coreingest.ValidInstallationID(c.Installation) || (c.User == "" && !r.devInstallations) {
			return ErrUnauthorized
		}
	}
	return nil
}

// latestInstallation is the record the installation pushed through most
// recently, among the user's accounts for an OIDC installation.
func (r *Repository) latestInstallation(ctx context.Context, c Credential) (storedb.GetLatestPluginInstallationRow, bool, error) {
	row, err := r.queries.GetLatestPluginInstallation(ctx, storedb.GetLatestPluginInstallationParams{
		InstallationID: text(c.Installation), UserID: text(c.User),
	})
	switch {
	case err == nil:
		return row, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return row, false, nil
	}
	return row, false, fmt.Errorf("get plugin installation: %w", err)
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
	latest, found, err := r.latestInstallation(ctx, c)
	if err != nil {
		return Principal{}, err
	}
	if found {
		bound = latest.AccountID
	}
	account, err := r.resolveAccount(ctx, r.queries, c.User, coreingest.DevAccountHint{Explicit: c.Account, Bound: bound})
	if err != nil {
		return Principal{}, err
	}
	return Principal{AccountID: account, Scopes: c.scopes(), Tokenless: true}, nil
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
	latest, found, err := r.latestInstallation(ctx, c)
	if err != nil {
		return coreingest.Manifest{}, err
	}
	if found {
		resync = int64(latest.ManifestVersion) != r.definition.Version
	}
	manifest := r.definition.manifest(c.scopes(), resync)
	return manifest, manifest.Validate()
}

// lockCredential locks the protocol row of a push: the token's, or the
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
	account, err := r.resolveAccount(ctx, q, c.User, coreingest.DevAccountHint{Explicit: c.Account, Player: push.Username})
	if err != nil {
		return credentialRow{}, err
	}
	id, err := randomText("pin_", 16)
	if err != nil {
		return credentialRow{}, fmt.Errorf("generate installation record id: %w", err)
	}
	scopes := c.scopes()
	rawScopes, err := encodeScopes(scopes)
	if err != nil {
		return credentialRow{}, err
	}
	label := "dev installation"
	if c.User != "" {
		label = "oidc installation"
	}
	if err = q.EnsurePluginInstallation(ctx, storedb.EnsurePluginInstallationParams{
		ID: id, AccountID: account, InstallationID: text(c.Installation),
		Label: text(label), Scopes: rawScopes,
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
	return credentialRow{ID: row.ID, AccountID: row.AccountID, Scopes: scopes, ManifestVersion: int64(row.ManifestVersion), LastSeq: row.LastSeq}, nil
}

// resolveAccount maps the request's hint onto the active accounts: all of
// them for a dev installation, the user's for an OIDC one.
func (r *Repository) resolveAccount(ctx context.Context, q *storedb.Queries, user string, hint coreingest.DevAccountHint) (string, error) {
	var (
		rows []storedb.Account
		err  error
	)
	if user == "" {
		rows, err = q.ListActiveAccounts(ctx)
	} else {
		rows, err = q.ListUserActiveAccounts(ctx, user)
	}
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
