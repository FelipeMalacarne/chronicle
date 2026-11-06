-- name: GetPlayerStateFact :one
SELECT * FROM player_state WHERE account_id = $1 AND facet = $2 AND key = $3;

-- name: ListPlayerState :many
SELECT * FROM player_state WHERE account_id = $1 ORDER BY facet, key;

-- name: ListPlayerStateByFacet :many
SELECT * FROM player_state WHERE account_id = $1 AND facet = $2 ORDER BY key;

-- name: ListPlayerStateBySource :many
SELECT * FROM player_state WHERE account_id = $1 AND source = $2 ORDER BY facet, key;

-- name: ListCurrentSkills :many
SELECT * FROM v_current_skills WHERE account_id = $1 ORDER BY skill;

-- name: ListCurrentQuests :many
SELECT * FROM v_current_quests WHERE account_id = $1 ORDER BY quest;

-- name: DeletePlayerStateByAccount :exec
DELETE FROM player_state WHERE account_id = $1;

-- Observations per source (docs/10 section 5). Writers use these; only the
-- projection below writes player_state.

-- name: UpsertObservation :exec
-- The last observation of one source for one fact. An older observation of the
-- same source never replaces a newer one.
INSERT INTO observations (account_id, facet, key, source, value, snapshot_id, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, facet, key, source) DO UPDATE SET
  value       = EXCLUDED.value,
  snapshot_id = EXCLUDED.snapshot_id,
  observed_at = EXCLUDED.observed_at
WHERE EXCLUDED.observed_at >= observations.observed_at;

-- name: PatchObservation :exec
-- Skill deltas carry only some fields (xp, boosted): merge them into the
-- source's last observation instead of erasing level.
INSERT INTO observations (account_id, facet, key, source, value, snapshot_id, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, facet, key, source) DO UPDATE SET
  value       = observations.value || EXCLUDED.value,
  snapshot_id = EXCLUDED.snapshot_id,
  observed_at = EXCLUDED.observed_at
WHERE EXCLUDED.observed_at >= observations.observed_at;

-- name: DeleteObservation :exec
DELETE FROM observations WHERE account_id = $1 AND facet = $2 AND key = $3 AND source = $4;

-- name: DeleteObservationFacet :exec
-- A full push of a source replaces that source's facet; other sources stay.
DELETE FROM observations WHERE account_id = $1 AND facet = $2 AND source = $3;

-- name: DeleteObservationItemsByContainer :exec
DELETE FROM observations
WHERE account_id = sqlc.arg(account_id)
  AND facet = 'item'
  AND source = sqlc.arg(source)
  AND key LIKE sqlc.arg(container)::text || ':%';

-- name: ListObservationsByFacet :many
SELECT * FROM observations WHERE account_id = $1 AND facet = $2 ORDER BY key, source;

-- name: ListObservedFacets :many
SELECT DISTINCT o.facet FROM observations o WHERE o.account_id = sqlc.arg(account_id)::text
UNION
SELECT DISTINCT p.facet FROM player_state p WHERE p.account_id = sqlc.arg(account_id)::text;

-- name: ObservationFreshness :many
-- Newest observation per source, for the "stale" indicator.
SELECT source, max(observed_at)::timestamptz AS last_observed_at, count(*) AS facts
FROM observations
WHERE account_id = $1
GROUP BY source;

-- name: DeletePlayerStateFacetProjection :exec
DELETE FROM player_state WHERE account_id = $1 AND facet = $2;

-- name: InsertPlayerStateProjection :copyfrom
INSERT INTO player_state (account_id, facet, key, value, source, snapshot_id, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);
