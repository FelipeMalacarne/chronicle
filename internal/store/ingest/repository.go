package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

const adapterVersion = 1

// Service is the transport-facing ingest behavior.
type Service interface {
	Manifest(context.Context, string) (coreingest.Manifest, error)
	Accept(context.Context, string, string, json.RawMessage, coreingest.Push) (coreingest.Response, error)
	CreateToken(context.Context, string, string, []coreingest.TokenScope) (CreatedToken, error)
}

// Repository implements Service with PostgreSQL transactions.
type Repository struct {
	pool       *pgxpool.Pool
	queries    *storedb.Queries
	definition Definition
}

func New(pool *pgxpool.Pool, definition Definition) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), definition: definition}
}

func (r *Repository) Manifest(ctx context.Context, plaintextToken string) (coreingest.Manifest, error) {
	token, err := r.queries.GetPluginTokenByHash(ctx, tokenHash(plaintextToken))
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

func responseFor(definition Definition, acceptedSeq int64, ok, resync bool) coreingest.Response {
	return coreingest.Response{
		ContractVersion: coreingest.ContractVersion,
		OK:              ok,
		AcceptedSeq:     acceptedSeq,
		Resync:          resync,
		ManifestVersion: definition.Version,
	}
}

// Accept validates and persists a push atomically. The token row lock covers
// sequence comparison, all snapshot/state writes, and last_seq advancement.
func (r *Repository) Accept(ctx context.Context, plaintextToken, remoteIP string, raw json.RawMessage, push coreingest.Push) (resp coreingest.Response, err error) {
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
	token, err := q.GetPluginTokenByHashForUpdate(ctx, tokenHash(plaintextToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreingest.Response{}, ErrUnauthorized
		}
		return coreingest.Response{}, fmt.Errorf("lock ingest token: %w", err)
	}
	resp = responseFor(r.definition, token.LastSeq, false, true)
	if push.ClientSeq <= token.LastSeq {
		return resp, ErrStaleSequence
	}
	if push.ManifestVersion != r.definition.Version {
		return resp, ErrStaleManifest
	}
	if int64(token.ManifestVersion) != r.definition.Version && push.Kind != coreingest.PushFull {
		return resp, ErrStaleManifest
	}
	scopes, err := decodeScopes(token.Scopes)
	if err != nil {
		return coreingest.Response{}, err
	}
	manifest := r.definition.manifest(scopes, false)
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
	if err := tx.Commit(ctx); err != nil {
		return coreingest.Response{}, fmt.Errorf("commit ingest transaction: %w", err)
	}
	return responseFor(r.definition, push.ClientSeq, true, false), nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}
