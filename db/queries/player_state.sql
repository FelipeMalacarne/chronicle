-- name: UpsertPlayerStateFact :exec
-- "Last observation wins": the row is only replaced when the new observation is
-- not older than the stored one. Source priority on ties is resolved by the
-- caller (plugin > wikisync > hiscores > wom) before calling this.
INSERT INTO player_state (account_id, facet, key, value, source, snapshot_id, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, facet, key) DO UPDATE SET
  value       = EXCLUDED.value,
  source      = EXCLUDED.source,
  snapshot_id = EXCLUDED.snapshot_id,
  observed_at = EXCLUDED.observed_at
WHERE EXCLUDED.observed_at >= player_state.observed_at;

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

-- name: GetPlayerStateFreshness :many
-- Newest observation per source, for the "stale" indicator.
SELECT source, max(observed_at)::timestamptz AS last_observed_at, count(*) AS facts
FROM player_state
WHERE account_id = $1
GROUP BY source;

-- name: DeletePlayerStateByAccount :exec
DELETE FROM player_state WHERE account_id = $1;
