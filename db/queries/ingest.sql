-- Queries used by internal/store/ingest. All mutation queries run inside the
-- same transaction that locks plugin_ingest_tokens.

-- name: InsertSnapshotItemContainer :exec
INSERT INTO snapshot_item_containers (
  snapshot_id, container, state, observed_at, revision, live_epoch
) VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpsertIngestContainerFreshness :one
INSERT INTO ingest_container_freshness (
  account_id, container, state, observed_at, revision, live_epoch, snapshot_id
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, container) DO UPDATE SET
  state       = EXCLUDED.state,
  observed_at = EXCLUDED.observed_at,
  revision    = EXCLUDED.revision,
  live_epoch  = EXCLUDED.live_epoch,
  snapshot_id = EXCLUDED.snapshot_id,
  updated_at  = now()
WHERE
  EXCLUDED.observed_at > ingest_container_freshness.observed_at
  OR (
    EXCLUDED.observed_at = ingest_container_freshness.observed_at
    AND CASE
      WHEN EXCLUDED.state <> ingest_container_freshness.state THEN
        CASE EXCLUDED.state WHEN 'live' THEN 3 WHEN 'restored' THEN 2 ELSE 1 END
          > CASE ingest_container_freshness.state WHEN 'live' THEN 3 WHEN 'restored' THEN 2 ELSE 1 END
      WHEN EXCLUDED.state = 'unavailable' THEN true
      WHEN EXCLUDED.state = 'restored' THEN EXCLUDED.revision > ingest_container_freshness.revision
      WHEN EXCLUDED.live_epoch = ingest_container_freshness.live_epoch THEN
        EXCLUDED.revision > ingest_container_freshness.revision
      ELSE true
    END
  )
RETURNING *;

-- name: ListItemContainerFreshness :many
-- Latest accepted observation per item container, plus whether a full push
-- observed the container in that same state and live epoch. Without such a
-- baseline the merged item facts only prove presence, never absence.
SELECT f.container, f.state, f.observed_at, f.revision, f.live_epoch, f.snapshot_id,
  EXISTS (
    SELECT 1
    FROM snapshot_item_containers c
    JOIN snapshots s ON s.id = c.snapshot_id
    WHERE s.account_id = f.account_id
      AND s.kind = 'full'
      AND c.container = f.container
      AND c.state = f.state
      AND c.state <> 'unavailable'
      AND c.live_epoch IS NOT DISTINCT FROM f.live_epoch
  )::boolean AS has_baseline
FROM ingest_container_freshness f
WHERE f.account_id = $1
ORDER BY f.container;
