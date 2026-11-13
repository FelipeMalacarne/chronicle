// Package workers persists the managed Microbot workers (docs/11 sections
// 4.2, 4.5 and 13.5): the account's game session, sealed with the vault key,
// the worker's desired state for the reconciler, and the worker's own ipl_
// token for the Chronicle Link in its pod.
//
// The database cannot hold the invariants that span tables (migration 0012),
// so this repository does: accounts.managed is true exactly while a session
// exists, a worker is desired running only with a session, and the session
// cannot be deleted while the worker is running. Every write locks the
// accounts row first, so the check and the write are serialized per account.
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/vault"
)

// Desired states of a worker (workers.desired_state).
const (
	StateRunning = "running"
	StateStopped = "stopped"
)

// Errors returned to the transport.
var (
	// ErrNotFound: the account does not exist, or (Get, SetError and the
	// token calls) it has no worker.
	ErrNotFound = errors.New("workers: account or worker not found")
	// ErrNoSession: the account has no game session.
	ErrNoSession = errors.New("workers: the account has no game session")
	// ErrWorkerRunning: the session cannot be deleted while the worker is
	// desired running; stop it first.
	ErrWorkerRunning = errors.New("workers: the worker is running")
	// ErrKeyMismatch: the session was sealed with another vault key than
	// CHRONICLE_VAULT_KEY (a rotated or wrong key); connect the session again.
	ErrKeyMismatch = errors.New("workers: the session was sealed with another vault key")
	// ErrInvalid: a parameter is out of the contract (state, world, an empty
	// session field).
	ErrInvalid = errors.New("workers: invalid parameter")
)

// workerScopes are the scopes of a worker's ipl_ token: the Chronicle Link
// tracks state and the three item containers, never marks.
var workerScopes = []coreingest.TokenScope{
	coreingest.ScopeStateWrite, coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment,
}

// TokenMinter creates ipl_ tokens; *ingest.Repository satisfies it.
type TokenMinter interface {
	CreateToken(ctx context.Context, accountID, label string, scopes []coreingest.TokenScope) (ingest.CreatedToken, error)
}

var _ TokenMinter = (*ingest.Repository)(nil)

// SessionInfo is what may be shown of a game session: never the secret.
type SessionInfo struct {
	AccountID string    `json:"account_id"`
	Character string    `json:"character"`
	KeyID     string    `json:"key_id"`
	PushedAt  time.Time `json:"pushed_at"`
	PushedBy  string    `json:"pushed_by"`
	// LastLoginAt and LastLoginResult are the last login a worker made with
	// this session; nil and empty until the first one. The result is LoginOK
	// or the game's reason for refusing it.
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	LastLoginResult string     `json:"last_login_result,omitempty"`
	// LoginFailed is derived: the last login with this session did not get
	// in. It is the only sign of an expired session the server has (it
	// cannot see when the Jagex session ends); a new session (`chronicle
	// accounts connect`) clears it, as does a login that gets in.
	LoginFailed bool `json:"login_failed,omitempty"`
}

// LoginOK is the LastLoginResult of a login that got in.
const LoginOK = "ok"

// Worker is a managed account as the reconciler and the panel see it: the
// worker row (zero values and a nil UpdatedAt while it was never started)
// plus the account's rsn and session info.
type Worker struct {
	AccountID     string     `json:"account_id"`
	UserID        string     `json:"user_id"`
	RSN           string     `json:"rsn"`
	DesiredState  string     `json:"desired_state"`
	World         int        `json:"world"`
	Image         string     `json:"image,omitempty"`
	PluginTokenID string     `json:"plugin_token_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	StoppedAt     *time.Time `json:"stopped_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	// AutoLogin: log the worker in again after a new pod, when it had been
	// logged in (WasLoggedIn); docs/11 section 15.5.
	AutoLogin   bool `json:"auto_login"`
	WasLoggedIn bool `json:"was_logged_in"`
	// Session is nil when the account has no session.
	Session *SessionInfo `json:"session,omitempty"`
}

// Repository reads and writes game_sessions, workers and accounts.managed.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
	key     vault.Key
	tokens  TokenMinter
}

// New returns a repository over pool. key seals and opens the sessions
// (CHRONICLE_VAULT_KEY); tokens mints the workers' ipl_ tokens.
func New(pool *pgxpool.Pool, key vault.Key, tokens TokenMinter) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), key: key, tokens: tokens}
}

func (r *Repository) inTx(ctx context.Context, fn func(q *storedb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if fnErr := fn(r.queries.WithTx(tx)); fnErr != nil {
		return fnErr
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// lockAccount locks the account row: every write takes it first.
func lockAccount(ctx context.Context, q *storedb.Queries, account string) (storedb.LockAccountForWorkerRow, error) {
	row, err := q.LockAccountForWorker(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock account: %w", err)
	}
	return row, nil
}

// aad binds a sealed session to its account.
func aad(account string) []byte { return []byte("game_session:" + account) }

// PutSession seals s and stores it as the account's session (replacing the
// previous one and its last login), and marks the account managed.
func (r *Repository) PutSession(ctx context.Context, account, pushedBy string, s agent.GameSession) error {
	s.Character = strings.TrimSpace(s.Character)
	if s.Character == "" || strings.TrimSpace(s.SessionID) == "" || strings.TrimSpace(s.CharacterID) == "" {
		return fmt.Errorf("%w: the session needs character, session_id and character_id", ErrInvalid)
	}
	if strings.TrimSpace(pushedBy) == "" {
		return fmt.Errorf("%w: pushed_by is empty", ErrInvalid)
	}
	plaintext, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	nonce, ciphertext, err := r.key.Seal(plaintext, aad(account))
	if err != nil {
		return err
	}
	return r.inTx(ctx, func(q *storedb.Queries) error {
		if _, lockErr := lockAccount(ctx, q, account); lockErr != nil {
			return lockErr
		}
		upErr := q.UpsertGameSession(ctx, storedb.UpsertGameSessionParams{
			AccountID: account, CharacterName: s.Character, KeyID: r.key.ID(),
			Nonce: nonce, Ciphertext: ciphertext, PushedBy: pushedBy,
		})
		if upErr != nil {
			return fmt.Errorf("store session: %w", upErr)
		}
		if upErr = q.SetAccountManaged(ctx, storedb.SetAccountManagedParams{ID: account, Managed: true}); upErr != nil {
			return fmt.Errorf("mark account managed: %w", upErr)
		}
		return nil
	})
}

// SessionInfo returns the account's session without the secret.
func (r *Repository) SessionInfo(ctx context.Context, account string) (SessionInfo, error) {
	row, err := r.queries.GetGameSessionInfo(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionInfo{}, ErrNoSession
	}
	if err != nil {
		return SessionInfo{}, fmt.Errorf("read session info: %w", err)
	}
	return sessionInfo(row.AccountID, row.CharacterName, row.KeyID, row.PushedAt, row.PushedBy, row.LastLoginAt, row.LastLoginResult), nil
}

// Session returns the account's session, opened. Only the worker bootstrap
// may call it; the API never returns it.
func (r *Repository) Session(ctx context.Context, account string) (agent.GameSession, error) {
	row, err := r.queries.GetGameSession(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.GameSession{}, ErrNoSession
	}
	if err != nil {
		return agent.GameSession{}, fmt.Errorf("read session: %w", err)
	}
	if row.KeyID != r.key.ID() {
		return agent.GameSession{}, fmt.Errorf("%w: sealed with key %s, the current key is %q", ErrKeyMismatch, row.KeyID, r.key.ID())
	}
	plaintext, err := r.key.Open(row.Nonce, row.Ciphertext, aad(account))
	if err != nil {
		return agent.GameSession{}, fmt.Errorf("open session of %s: %w", account, err)
	}
	var s agent.GameSession
	if err = json.Unmarshal(plaintext, &s); err != nil {
		return agent.GameSession{}, fmt.Errorf("decode session of %s: %w", account, err)
	}
	return s, nil
}

// DeleteSession deletes the account's session and clears managed. It refuses
// with ErrWorkerRunning while the worker is desired running.
func (r *Repository) DeleteSession(ctx context.Context, account string) error {
	return r.inTx(ctx, func(q *storedb.Queries) error {
		if _, err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		w, err := q.GetWorkerForUpdate(ctx, account)
		switch {
		case err == nil && w.DesiredState == StateRunning:
			return ErrWorkerRunning
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("read worker: %w", err)
		}
		n, err := q.DeleteGameSession(ctx, account)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
		if n == 0 {
			return ErrNoSession
		}
		if err = q.SetAccountManaged(ctx, storedb.SetAccountManagedParams{ID: account, Managed: false}); err != nil {
			return fmt.Errorf("clear account managed: %w", err)
		}
		return nil
	})
}

// RecordLogin records the outcome of a login a worker made with the
// account's session.
func (r *Repository) RecordLogin(ctx context.Context, account, result string) error {
	n, err := r.queries.RecordGameSessionLogin(ctx, storedb.RecordGameSessionLoginParams{AccountID: account, LastLoginResult: result})
	if err != nil {
		return fmt.Errorf("record login: %w", err)
	}
	if n == 0 {
		return ErrNoSession
	}
	return nil
}

// SetDesired sets what the reconciler should run for the account, creating
// the worker row on first use. started_at and stopped_at record the last
// change to running and to stopped (a start clears stopped_at and
// last_error); world is stored as given (0 keeps the profile's) and an empty
// image keeps the stored one. Running needs a session (ErrNoSession).
func (r *Repository) SetDesired(ctx context.Context, account, state string, world int, image string) error {
	if state != StateRunning && state != StateStopped {
		return fmt.Errorf("%w: desired state %q", ErrInvalid, state)
	}
	if world < 0 || world > 1<<16 {
		return fmt.Errorf("%w: world %d", ErrInvalid, world)
	}
	return r.inTx(ctx, func(q *storedb.Queries) error {
		if _, err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		if state == StateRunning {
			if _, err := q.GetGameSessionInfo(ctx, account); errors.Is(err, pgx.ErrNoRows) {
				return ErrNoSession
			} else if err != nil {
				return fmt.Errorf("read session: %w", err)
			}
		}
		if err := q.UpsertWorkerDesired(ctx, storedb.UpsertWorkerDesiredParams{
			AccountID: account, DesiredState: state, World: int32(world), Image: strings.TrimSpace(image), //nolint:gosec // bounded above
		}); err != nil {
			return fmt.Errorf("set desired state: %w", err)
		}
		return nil
	})
}

// Get returns the account's worker (ErrNotFound without a worker row or a
// session).
func (r *Repository) Get(ctx context.Context, account string) (Worker, error) {
	return get(ctx, r.queries, account)
}

func get(ctx context.Context, q *storedb.Queries, account string) (Worker, error) {
	if account == "" {
		return Worker{}, ErrNotFound
	}
	rows, err := q.ListWorkers(ctx, storedb.ListWorkersParams{AccountID: account})
	if err != nil {
		return Worker{}, fmt.Errorf("read worker: %w", err)
	}
	if len(rows) == 0 {
		return Worker{}, ErrNotFound
	}
	return workerOf(rows[0]), nil
}

// List returns every managed account (a worker row or a session), oldest
// account first.
func (r *Repository) List(ctx context.Context) ([]Worker, error) {
	return r.list(ctx, "")
}

// ListForUser is List restricted to the accounts of userID.
func (r *Repository) ListForUser(ctx context.Context, userID string) ([]Worker, error) {
	if userID == "" {
		return []Worker{}, nil
	}
	return r.list(ctx, userID)
}

func (r *Repository) list(ctx context.Context, userID string) ([]Worker, error) {
	rows, err := r.queries.ListWorkers(ctx, storedb.ListWorkersParams{UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	out := make([]Worker, 0, len(rows))
	for _, row := range rows {
		out = append(out, workerOf(row))
	}
	return out, nil
}

// SetRunningImage moves the worker to image while it is meant to run (a
// restart on the server's current image) and reports whether it did; a
// stopped worker, or none, is left as it is and answers false, so a restart
// that races a stop never starts the worker again.
func (r *Repository) SetRunningImage(ctx context.Context, account, image string) (bool, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return false, fmt.Errorf("%w: empty image", ErrInvalid)
	}
	var moved bool
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if _, err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		n, err := q.SetRunningWorkerImage(ctx, storedb.SetRunningWorkerImageParams{AccountID: account, Image: image})
		if err != nil {
			return fmt.Errorf("set worker image: %w", err)
		}
		moved = n > 0
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return moved, err
}

// SetError records the reconciler's last error for the worker ("" clears it).
func (r *Repository) SetError(ctx context.Context, account, msg string) error {
	n, err := r.queries.SetWorkerError(ctx, storedb.SetWorkerErrorParams{AccountID: account, LastError: msg})
	if err != nil {
		return fmt.Errorf("set worker error: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAutoLogin turns the worker's automatic login on or off (ErrNotFound
// without a worker row).
func (r *Repository) SetAutoLogin(ctx context.Context, account string, on bool) error {
	n, err := r.queries.SetWorkerAutoLogin(ctx, storedb.SetWorkerAutoLoginParams{AccountID: account, AutoLogin: on})
	if err != nil {
		return fmt.Errorf("set auto login: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLoggedIn records whether the worker's client was last logged in
// (docs/11 section 15.5); the reconciler sets it, the owner's logout and a
// stop clear it.
func (r *Repository) SetLoggedIn(ctx context.Context, account string, on bool) error {
	n, err := r.queries.SetWorkerLoggedIn(ctx, storedb.SetWorkerLoggedInParams{AccountID: account, WasLoggedIn: on})
	if err != nil {
		return fmt.Errorf("set logged in: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RunningPlugins returns the plugins the worker's client had on at the last
// complete read (docs/11 section 15.4); none without a worker row.
func (r *Repository) RunningPlugins(ctx context.Context, account string) ([]agent.ProfilePlugin, error) {
	raw, err := r.queries.GetWorkerRunningPlugins(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running plugins: %w", err)
	}
	var out []agent.ProfilePlugin
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode running plugins: %w", err)
	}
	return out, nil
}

// SetRunningPlugins replaces the worker's running plugins; the caller sends
// only a complete read. A list that is already stored writes nothing.
func (r *Repository) SetRunningPlugins(ctx context.Context, account string, plugins []agent.ProfilePlugin) error {
	for _, p := range plugins {
		if err := agent.ValidatePlugin(p); err != nil {
			return err
		}
	}
	if plugins == nil {
		plugins = []agent.ProfilePlugin{}
	}
	have, err := r.RunningPlugins(ctx, account)
	if err != nil {
		return err
	}
	if len(have) == len(plugins) && (len(have) == 0 || reflect.DeepEqual(have, plugins)) {
		return nil
	}
	raw, err := json.Marshal(plugins)
	if err != nil {
		return err
	}
	n, err := r.queries.SetWorkerRunningPlugins(ctx, storedb.SetWorkerRunningPluginsParams{AccountID: account, RunningPlugins: raw})
	if err != nil {
		return fmt.Errorf("set running plugins: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RunningSettings returns the option values of the plugins the worker's
// client had on at the last complete read (docs/11 section 15.6); none
// without a worker row.
func (r *Repository) RunningSettings(ctx context.Context, account string) ([]agent.ProfileSetting, error) {
	raw, err := r.queries.GetWorkerRunningSettings(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get running settings: %w", err)
	}
	var out []agent.ProfileSetting
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode running settings: %w", err)
	}
	return out, nil
}

// SetRunningSettings replaces them; the caller sends only a complete read.
// A list that is already stored writes nothing.
func (r *Repository) SetRunningSettings(ctx context.Context, account string, settings []agent.ProfileSetting) error {
	for _, s := range settings {
		if err := agent.ValidateSetting(s); err != nil {
			return err
		}
	}
	if settings == nil {
		settings = []agent.ProfileSetting{}
	}
	have, err := r.RunningSettings(ctx, account)
	if err != nil {
		return err
	}
	if len(have) == len(settings) && (len(have) == 0 || reflect.DeepEqual(have, settings)) {
		return nil
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	n, err := r.queries.SetWorkerRunningSettings(ctx, storedb.SetWorkerRunningSettingsParams{AccountID: account, RunningSettings: raw})
	if err != nil {
		return fmt.Errorf("set running settings: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// OtherClientSince reports whether another plugin installation of the account
// (not the worker's own token) pushed after since: someone is playing the
// account on another client (docs/11 section 6.2).
func (r *Repository) OtherClientSince(ctx context.Context, account string, since time.Time) (bool, error) {
	seen, err := r.queries.OtherPluginSeenSince(ctx, storedb.OtherPluginSeenSinceParams{
		AccountID: account, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("other plugin seen: %w", err)
	}
	return seen, nil
}

// MintPluginToken creates a new ipl_ token for the worker's Chronicle Link
// (label "worker <account>", scopes state:write and items:*), stores its id
// and revokes the previous one. The plaintext is returned only here.
func (r *Repository) MintPluginToken(ctx context.Context, account string) (string, error) {
	if _, err := r.Get(ctx, account); err != nil {
		return "", err
	}
	if r.tokens == nil {
		return "", errors.New("workers: no token minter")
	}
	created, err := r.tokens.CreateToken(ctx, account, "worker "+account, workerScopes)
	if errors.Is(err, ingest.ErrAccountNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("mint worker token: %w", err)
	}
	err = r.inTx(ctx, func(q *storedb.Queries) error {
		if _, lockErr := lockAccount(ctx, q, account); lockErr != nil {
			return lockErr
		}
		w, getErr := q.GetWorkerForUpdate(ctx, account)
		if errors.Is(getErr, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if getErr != nil {
			return fmt.Errorf("read worker: %w", getErr)
		}
		if w.PluginTokenID.Valid && w.PluginTokenID.String != created.ID {
			if _, revErr := q.RevokePluginToken(ctx, storedb.RevokePluginTokenParams{ID: w.PluginTokenID.String, AccountID: account}); revErr != nil {
				return fmt.Errorf("revoke previous worker token: %w", revErr)
			}
		}
		if setErr := q.SetWorkerPluginToken(ctx, storedb.SetWorkerPluginTokenParams{
			AccountID: account, PluginTokenID: pgtype.Text{String: created.ID, Valid: true},
		}); setErr != nil {
			return fmt.Errorf("store worker token: %w", setErr)
		}
		return nil
	})
	if err != nil {
		// The new token was never handed out: do not leave it valid.
		if _, revErr := r.queries.RevokePluginToken(context.WithoutCancel(ctx), storedb.RevokePluginTokenParams{ID: created.ID, AccountID: account}); revErr != nil {
			err = errors.Join(err, fmt.Errorf("revoke unused worker token: %w", revErr))
		}
		return "", err
	}
	return created.Token, nil
}

// RevokePluginToken revokes the worker's ipl_ token, if it has one, and
// clears it (ErrNotFound without a worker row).
func (r *Repository) RevokePluginToken(ctx context.Context, account string) error {
	return r.inTx(ctx, func(q *storedb.Queries) error {
		if _, err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		w, err := q.GetWorkerForUpdate(ctx, account)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read worker: %w", err)
		}
		if !w.PluginTokenID.Valid {
			return nil
		}
		if _, err = q.RevokePluginToken(ctx, storedb.RevokePluginTokenParams{ID: w.PluginTokenID.String, AccountID: account}); err != nil {
			return fmt.Errorf("revoke worker token: %w", err)
		}
		if err = q.SetWorkerPluginToken(ctx, storedb.SetWorkerPluginTokenParams{AccountID: account}); err != nil {
			return fmt.Errorf("clear worker token: %w", err)
		}
		return nil
	})
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func sessionInfo(account, character, keyID string, pushedAt pgtype.Timestamptz, pushedBy string, lastLoginAt pgtype.Timestamptz, lastLoginResult string) SessionInfo {
	return SessionInfo{
		AccountID: account, Character: character, KeyID: keyID, PushedAt: pushedAt.Time, PushedBy: pushedBy,
		LastLoginAt: timePtr(lastLoginAt), LastLoginResult: lastLoginResult,
		LoginFailed: lastLoginResult != "" && lastLoginResult != LoginOK,
	}
}

func workerOf(row storedb.ListWorkersRow) Worker {
	w := Worker{
		AccountID: row.AccountID, UserID: row.UserID, RSN: row.Rsn,
		DesiredState: row.DesiredState, World: int(row.World), Image: row.Image, PluginTokenID: row.PluginTokenID,
		StartedAt: timePtr(row.StartedAt), StoppedAt: timePtr(row.StoppedAt), UpdatedAt: timePtr(row.UpdatedAt),
		LastError: row.LastError, AutoLogin: row.AutoLogin, WasLoggedIn: row.WasLoggedIn,
	}
	if row.HasSession {
		info := sessionInfo(row.AccountID, row.CharacterName, row.KeyID, row.PushedAt, row.PushedBy, row.LastLoginAt, row.LastLoginResult)
		w.Session = &info
	}
	return w
}
