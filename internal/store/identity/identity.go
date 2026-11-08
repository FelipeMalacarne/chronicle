// Package identity persists the users of the web login (docs/04 section 8):
// the OIDC upsert by (issuer, subject), the first-login claim of the dev
// accounts, and the ownership checks behind every account-addressed route.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// DevSubject is the subject of the single user of AUTH_MODE=dev.
const DevSubject = "dev"

// Errors returned to the transport.
var (
	// ErrSubjectConflict: the subject already belongs to the dev user or to
	// a user of another issuer; the login is refused instead of merged.
	ErrSubjectConflict = errors.New("identity: subject belongs to another issuer")
	ErrUserNotFound    = errors.New("identity: user not found")
)

// User is a row of users as the API shows it.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
}

// Login is a verified OIDC identity.
type Login struct {
	Issuer      string
	Subject     string
	Username    string
	Email       string
	DisplayName string
}

// LoginResult is the user after the login.
type LoginResult struct {
	User User
	// Created: first login of this subject.
	Created bool
	// ClaimedAccounts is how many dev accounts moved to this user (see
	// LoginOIDC).
	ClaimedAccounts int64
}

// Repository implements identity persistence over PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

func userOf(u storedb.User) User {
	return User{ID: u.ID, Username: u.Username, Email: u.Email.String, DisplayName: u.DisplayName.String, Role: u.Role}
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// LoginOIDC creates or updates the user of (issuer, subject) in one
// transaction serialized with every other login.
//
// Account claim (the documented linking rule): when this is the first login
// of the subject AND no other OIDC user exists, every account of the dev user
// (subject 'dev', no issuer) moves to the new user. That is the owner's first
// login on a database that came from AUTH_MODE=dev (zaros → saradomin by
// pg_dump). Any later user starts without accounts; nothing else is guessed.
func (r *Repository) LoginOIDC(ctx context.Context, in Login) (LoginResult, error) {
	if in.Issuer == "" || in.Subject == "" {
		return LoginResult{}, fmt.Errorf("identity: login without issuer or subject")
	}
	var out LoginResult
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return out, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ironman-planner:oidc-login'))`); err != nil {
		return out, fmt.Errorf("lock logins: %w", err)
	}
	q := r.queries.WithTx(tx)
	existing, err := q.GetUserBySubject(ctx, in.Subject)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		out.Created = true
	case err != nil:
		return out, fmt.Errorf("get user: %w", err)
	case !existing.Issuer.Valid || existing.Issuer.String != in.Issuer:
		return out, ErrSubjectConflict
	}
	username := existing.Username
	if out.Created {
		if username, err = freeUsername(ctx, q, in); err != nil {
			return out, err
		}
	}
	others := int64(0)
	if out.Created {
		if others, err = q.CountOIDCUsersExcept(ctx, in.Subject); err != nil {
			return out, fmt.Errorf("count users: %w", err)
		}
	}
	id := existing.ID
	// The first OIDC user owns the planner; later ones are members (owner
	// decision of 2026-11-05). An existing user keeps its role.
	role := existing.Role
	if out.Created {
		id = newID()
		role = "member"
		if others == 0 {
			role = "owner"
		}
	}
	row, err := q.UpsertOIDCUser(ctx, storedb.UpsertOIDCUserParams{
		ID: id, Subject: in.Subject, Issuer: in.Issuer, Username: username,
		Email: text(in.Email), DisplayName: text(in.DisplayName), Role: role,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrSubjectConflict
	}
	if err != nil {
		return out, fmt.Errorf("upsert user: %w", err)
	}
	out.User = userOf(row)
	if out.Created && others == 0 {
		if out.ClaimedAccounts, err = q.ClaimDevAccounts(ctx, row.ID); err != nil {
			return out, fmt.Errorf("claim dev accounts: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return out, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// freeUsername picks the UNIQUE username of a new user: preferred_username,
// else the email, else the subject; a taken name gets the subject appended.
func freeUsername(ctx context.Context, q *storedb.Queries, in Login) (string, error) {
	name := strings.TrimSpace(in.Username)
	if name == "" {
		name = strings.TrimSpace(in.Email)
	}
	if name == "" {
		name = in.Subject
	}
	for _, candidate := range []string{name, name + "#" + in.Subject} {
		taken, err := q.UsernameTaken(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("check username: %w", err)
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("identity: username %q is taken", name)
}

// User returns one user by id.
func (r *Repository) User(ctx context.Context, id string) (User, error) {
	u, err := r.queries.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return userOf(u), nil
}

// UserBySubject returns the OIDC user of (issuer, subject): the user a bearer
// token of the native client speaks for. It never creates one; the first
// login of a subject is the web's (LoginOIDC), which also runs the dev
// account claim. A subject of another issuer or of the dev user is not found.
func (r *Repository) UserBySubject(ctx context.Context, issuer, subject string) (User, error) {
	u, err := r.queries.GetUserBySubject(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	if issuer == "" || !u.Issuer.Valid || u.Issuer.String != issuer {
		return User{}, ErrUserNotFound
	}
	return userOf(u), nil
}

// DevUser returns the AUTH_MODE=dev user; found is false before the first
// account exists.
func (r *Repository) DevUser(ctx context.Context) (User, bool, error) {
	u, err := r.queries.GetUserBySubject(ctx, DevSubject)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get dev user: %w", err)
	}
	return userOf(u), true, nil
}

// Accounts lists the user's accounts, active first.
func (r *Repository) Accounts(ctx context.Context, userID string) ([]storeprogress.AccountView, error) {
	rows, err := r.queries.ListAccountsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]storeprogress.AccountView, 0, len(rows))
	for _, a := range rows {
		out = append(out, storeprogress.AccountView{ID: a.ID, RSN: a.Rsn, Mode: a.Mode, GuideID: a.GuideID, Layer: a.Layer})
	}
	return out, nil
}

// OwnsAccount reports whether accountID exists and belongs to userID.
func (r *Repository) OwnsAccount(ctx context.Context, userID, accountID string) (bool, error) {
	ok, err := r.queries.AccountOwnedBy(ctx, storedb.AccountOwnedByParams{AccountID: accountID, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("account owner: %w", err)
	}
	return ok, nil
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "usr_" + hex.EncodeToString(b)
}
