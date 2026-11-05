package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// adapterVersion 2: snapshot_facts, quests and diary tiers (migration 0005).
const adapterVersion = 2

// Service is the transport-facing ingest behavior. Every plugin call names
// its Credential: an ipl_ token or a tokenless dev installation.
type Service interface {
	ManifestFor(context.Context, Credential) (coreingest.Manifest, error)
	AcceptFor(context.Context, Credential, string, json.RawMessage, coreingest.Push) (coreingest.Response, error)
	Principal(context.Context, Credential) (Principal, error)
	CreateToken(context.Context, string, string, []coreingest.TokenScope) (CreatedToken, error)
}

// Reevaluator reevaluates an account's progress inside the ingest
// transaction. It returns how many steps changed status in every guide
// (steps_changed) and what changed in the active route (the response's
// route, read by the plugin panel).
type Reevaluator interface {
	ReevaluateRoute(ctx context.Context, tx pgx.Tx, accountID, snapshotID string) (int, coreingest.RouteUpdate, error)
}

// Repository implements Service with PostgreSQL transactions.
type Repository struct {
	pool        *pgxpool.Pool
	queries     *storedb.Queries
	definition  Definition
	reevaluator Reevaluator
	publisher   events.Publisher
	// devInstallations accepts tokenless credentials (AUTH_MODE=dev).
	devInstallations bool
}

func New(pool *pgxpool.Pool, definition Definition) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), definition: definition}
}

// WithReevaluator makes every accepted push reevaluate the account's progress
// in the same transaction, so a push and its step changes commit together.
func (r *Repository) WithReevaluator(re Reevaluator) *Repository {
	r.reevaluator = re
	return r
}

// WithPublisher makes every committed push publish a progress event for the
// account (GET /api/events). nil disables it.
func (r *Repository) WithPublisher(p events.Publisher) *Repository {
	r.publisher = p
	return r
}

// Manifest returns the manifest of an ipl_ token.
func (r *Repository) Manifest(ctx context.Context, plaintextToken string) (coreingest.Manifest, error) {
	return r.manifestFor(ctx, Credential{Token: plaintextToken})
}

// ManifestFor returns the manifest of a token or a dev installation.
func (r *Repository) ManifestFor(ctx context.Context, c Credential) (coreingest.Manifest, error) {
	return r.manifestFor(ctx, c)
}

func responseFor(definition Definition, acceptedSeq int64, ok, resync bool) coreingest.Response {
	return coreingest.Response{
		ContractVersion: coreingest.ContractVersion,
		OK:              ok,
		AcceptedSeq:     acceptedSeq,
		Resync:          resync,
		ManifestVersion: definition.Version,
	}
}

// Accept validates and persists a push of an ipl_ token.
func (r *Repository) Accept(ctx context.Context, plaintextToken, remoteIP string, raw json.RawMessage, push coreingest.Push) (coreingest.Response, error) {
	return r.AcceptFor(ctx, Credential{Token: plaintextToken}, remoteIP, raw, push)
}

// AcceptFor validates and persists a push atomically. The credential row
// lock (the token's, or the dev installation's record for the push's
// account) covers sequence comparison, all snapshot/state writes, and
// last_seq advancement.
func (r *Repository) AcceptFor(ctx context.Context, c Credential, remoteIP string, raw json.RawMessage, push coreingest.Push) (resp coreingest.Response, err error) {
	if err = push.Validate(); err != nil {
		return coreingest.Response{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	if len(raw) == 0 {
		raw, err = json.Marshal(push)
		if err != nil {
			return coreingest.Response{}, fmt.Errorf("marshal raw push: %w", err)
		}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return coreingest.Response{}, fmt.Errorf("begin ingest transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	token, err := r.lockCredential(ctx, q, c, push)
	if err != nil {
		return coreingest.Response{}, err
	}
	resp = responseFor(r.definition, token.LastSeq, false, true)
	if push.ClientSeq <= token.LastSeq {
		return resp, ErrStaleSequence
	}
	if push.ManifestVersion != r.definition.Version {
		return resp, ErrStaleManifest
	}
	if token.ManifestVersion != r.definition.Version && push.Kind != coreingest.PushFull {
		return resp, ErrStaleManifest
	}
	manifest := r.definition.manifest(token.Scopes, false)
	if err = push.ValidateAgainst(manifest); err != nil {
		return responseFor(r.definition, token.LastSeq, false, false), fmt.Errorf("%w: %v", ErrScopeDenied, err)
	}

	snapshotID, err := randomText("snap_", 16)
	if err != nil {
		return coreingest.Response{}, fmt.Errorf("generate snapshot id: %w", err)
	}
	rawHash := sha256.Sum256(raw)
	_, err = q.InsertSnapshot(ctx, storedb.InsertSnapshotParams{
		ID:             snapshotID,
		AccountID:      token.AccountID,
		Source:         "plugin",
		Kind:           string(push.Kind),
		FetchedAt:      timestamptz(push.SentAt),
		Raw:            append(json.RawMessage(nil), raw...),
		RawHash:        hex.EncodeToString(rawHash[:]),
		AdapterVersion: adapterVersion,
		ClientSeq:      pgtype.Int8{Int64: push.ClientSeq, Valid: true},
		IngestTokenID:  pgtype.Text{String: token.ID, Valid: true},
	})
	if err != nil {
		return coreingest.Response{}, fmt.Errorf("insert raw ingest snapshot: %w", err)
	}
	if err := r.persistNormalized(ctx, q, token.AccountID, snapshotID, push); err != nil {
		return coreingest.Response{}, err
	}
	if err := q.UpdatePluginTokenAfterPush(ctx, storedb.UpdatePluginTokenAfterPushParams{
		ID:              token.ID,
		LastSeq:         push.ClientSeq,
		ManifestVersion: int32(r.definition.Version),
		LastIp: pgtype.Text{
			String: strings.TrimSpace(remoteIP),
			Valid:  strings.TrimSpace(remoteIP) != "",
		},
	}); err != nil {
		return coreingest.Response{}, fmt.Errorf("advance ingest token: %w", err)
	}
	resp = responseFor(r.definition, push.ClientSeq, true, false)
	changed := 0
	if r.reevaluator != nil {
		n, route, err := r.reevaluator.ReevaluateRoute(ctx, tx, token.AccountID, snapshotID)
		if err != nil {
			return coreingest.Response{}, fmt.Errorf("reevaluate progress: %w", err)
		}
		changed = n
		steps := int64(n)
		resp.StepsChanged = &steps
		if route.Guide != "" {
			resp.Route = &route
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return coreingest.Response{}, fmt.Errorf("commit ingest transaction: %w", err)
	}
	// Only after the commit: the push changed facts (and maybe steps) of the
	// account, so open screens refetch.
	if r.publisher != nil {
		r.publisher.Publish(events.Event{
			Type: events.TypeProgress, Account: token.AccountID, Source: events.SourcePlugin, StepsChanged: changed,
		})
	}
	return resp, nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}
