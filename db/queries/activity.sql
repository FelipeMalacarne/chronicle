-- An account's level-ups (internal/core/activity; the v7's Histórico and
-- Atividade): read-only XP observations over the snapshots that already
-- exist, of every source. Every query is bounded in time; snapshots are
-- indexed by (account_id, fetched_at).

-- name: LevelXPBaseline :many
-- Each skill's XP before from_at: the most it was seen with in
-- [lookback_at, from_at). XP never goes down, and the hiscores may lag the
-- plugin, so the largest observation is the truth, not the last one.
SELECT f.key AS skill, max((f.value->>'xp')::bigint)::bigint AS xp
FROM snapshot_facts f JOIN snapshots s ON s.id = f.snapshot_id
WHERE s.account_id = sqlc.arg(account_id)
  AND s.fetched_at >= sqlc.arg(lookback_at) AND s.fetched_at < sqlc.arg(from_at)
  AND f.facet = 'skill' AND f.value ? 'xp' AND (f.value->>'xp')::bigint >= 0
GROUP BY f.key;

-- name: LevelXPObservations :many
-- Every XP observation from from_at on, oldest first. A plugin delta lists
-- only the skills whose XP changed, so this is one row per gain, not per
-- push and skill.
SELECT s.fetched_at, f.key AS skill, (f.value->>'xp')::bigint AS xp
FROM snapshot_facts f JOIN snapshots s ON s.id = f.snapshot_id
WHERE s.account_id = sqlc.arg(account_id) AND s.fetched_at >= sqlc.arg(from_at)
  AND f.facet = 'skill' AND f.value ? 'xp' AND (f.value->>'xp')::bigint >= 0
ORDER BY s.fetched_at, f.key;
