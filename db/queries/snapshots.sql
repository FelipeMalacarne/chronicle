-- name: InsertSnapshot :one
-- Non-plugin sources retain raw-hash dedupe. Plugin snapshots are idempotent
-- by (ingest_token_id, client_seq), so two installs can submit identical JSON.
INSERT INTO snapshots (id, account_id, source, kind, fetched_at, raw, raw_hash, adapter_version, client_seq, ingest_token_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSnapshot :one
SELECT * FROM snapshots WHERE id = $1;

-- name: GetSnapshotByHash :one
SELECT * FROM snapshots WHERE account_id = $1 AND source = $2 AND raw_hash = $3;

-- name: GetLatestSnapshotBySource :one
SELECT * FROM snapshots
WHERE account_id = $1 AND source = $2
ORDER BY fetched_at DESC
LIMIT 1;

-- name: ListLatestSnapshotPerSource :many
-- One row per source: the most recent snapshot. Feeds the freshness indicator.
SELECT DISTINCT ON (source) *
FROM snapshots
WHERE account_id = $1
ORDER BY source, fetched_at DESC;

-- name: ListSnapshots :many
SELECT id, account_id, source, kind, fetched_at, raw_hash, adapter_version, client_seq, created_at
FROM snapshots
WHERE account_id = $1
ORDER BY fetched_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteSnapshotsOlderThan :execrows
-- Retention job. player_state, objective_progress and the item-container
-- provenance keep FKs to snapshots, so only snapshots not referenced by them
-- are removed.
DELETE FROM snapshots s
WHERE s.account_id = $1
  AND s.fetched_at < $2
  AND NOT EXISTS (SELECT 1 FROM player_state ps WHERE ps.snapshot_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM objective_progress op WHERE op.evidence_snapshot_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM ingest_container_freshness f WHERE f.snapshot_id = s.id);

-- Normalized facts (snapshot_facts has the shape of player_state). Bulk
-- inserts use pgx CopyFrom (sqlc :copyfrom).

-- name: InsertSnapshotFacts :copyfrom
INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
VALUES ($1, $2, $3, $4);

-- name: ListSnapshotFacts :many
SELECT * FROM snapshot_facts WHERE snapshot_id = $1 ORDER BY facet, key;
