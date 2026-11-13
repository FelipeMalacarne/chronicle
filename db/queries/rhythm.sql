-- An account's rhythm (internal/core/rhythm, the v7's slice V5): read-only
-- series over the snapshots that already exist. Every query is bounded in
-- time; snapshots are indexed by (account_id, fetched_at).

-- name: SessionStart :one
-- The latest plugin session: its first snapshot (after a pause longer than
-- gap_s seconds, or the first since from_at) and its last one.
WITH pushes AS (
  SELECT s.fetched_at, lag(s.fetched_at) OVER (ORDER BY s.fetched_at) AS prev
  FROM snapshots s
  WHERE s.account_id = sqlc.arg(account_id) AND s.source = 'plugin' AND s.fetched_at >= sqlc.arg(from_at)
)
SELECT
  (SELECT p.fetched_at FROM pushes p
   WHERE p.prev IS NULL OR p.fetched_at - p.prev > sqlc.arg(gap_s)::int * interval '1 second'
   ORDER BY p.fetched_at DESC LIMIT 1)::timestamptz AS started_at,
  (SELECT max(p.fetched_at) FROM pushes p)::timestamptz AS last_at;

-- name: SkillXPLastBefore :many
-- Each skill's XP at to_at: the most it was seen with in (from_at, to_at].
-- XP never goes down, and the hiscores may lag the plugin, so the largest
-- observation is the latest truth, not the last one.
SELECT f.key AS skill, max((f.value->>'xp')::bigint)::bigint AS xp
FROM snapshot_facts f JOIN snapshots s ON s.id = f.snapshot_id
WHERE s.account_id = sqlc.arg(account_id) AND s.fetched_at > sqlc.arg(from_at) AND s.fetched_at <= sqlc.arg(to_at)
  AND f.facet = 'skill' AND f.value ? 'xp' AND (f.value->>'xp')::bigint >= 0
GROUP BY f.key;

-- name: SkillXPFirstAfter :many
-- Each skill's XP at its first observation after from_at.
SELECT DISTINCT ON (f.key) f.key AS skill, (f.value->>'xp')::bigint AS xp
FROM snapshot_facts f JOIN snapshots s ON s.id = f.snapshot_id
WHERE s.account_id = sqlc.arg(account_id) AND s.fetched_at > sqlc.arg(from_at)
  AND f.facet = 'skill' AND f.value ? 'xp' AND (f.value->>'xp')::bigint >= 0
ORDER BY f.key, s.fetched_at ASC;

-- name: SkillPoints :many
-- Every XP observation since from_at, oldest first.
SELECT s.fetched_at, f.key AS skill, (f.value->>'xp')::bigint AS xp
FROM snapshot_facts f JOIN snapshots s ON s.id = f.snapshot_id
WHERE s.account_id = sqlc.arg(account_id) AND s.fetched_at >= sqlc.arg(from_at)
  AND f.facet = 'skill' AND f.value ? 'xp' AND (f.value->>'xp')::bigint >= 0
ORDER BY s.fetched_at, f.key;

-- name: CoinObservations :many
-- Coins (995) and platinum tokens (13204) in the bank and the inventory as
-- each snapshot saw them, from the last full observation of each container
-- at or before before_at onward: full observations always (an absent item is
-- 0), deltas only when they list one of the two.
WITH last_full AS (
  SELECT c.container, max(s.fetched_at) AS at
  FROM snapshots s JOIN snapshot_item_containers c ON c.snapshot_id = s.id
  WHERE s.account_id = sqlc.arg(account_id) AND s.kind = 'full' AND c.state <> 'unavailable'
    AND c.container IN ('bank', 'inventory') AND s.fetched_at <= sqlc.arg(before_at)
  GROUP BY c.container
)
-- coins and platinum are -1 when the snapshot does not list them.
SELECT s.fetched_at, (s.kind = 'full') AS full, c.container,
  COALESCE((SELECT (f.value->>'qty')::bigint FROM snapshot_facts f
   WHERE f.snapshot_id = s.id AND f.facet = 'item' AND f.key = c.container || ':995'), -1)::bigint AS coins,
  COALESCE((SELECT (f.value->>'qty')::bigint FROM snapshot_facts f
   WHERE f.snapshot_id = s.id AND f.facet = 'item' AND f.key = c.container || ':13204'), -1)::bigint AS platinum
FROM snapshots s
JOIN snapshot_item_containers c ON c.snapshot_id = s.id
LEFT JOIN last_full lf ON lf.container = c.container
WHERE s.account_id = sqlc.arg(account_id) AND c.state <> 'unavailable' AND c.container IN ('bank', 'inventory')
  AND s.fetched_at >= COALESCE(lf.at, '-infinity'::timestamptz)
  AND (s.kind = 'full' OR EXISTS (
    SELECT 1 FROM snapshot_facts f
    WHERE f.snapshot_id = s.id AND f.facet = 'item' AND f.key IN (c.container || ':995', c.container || ':13204')))
ORDER BY s.fetched_at, c.container;
