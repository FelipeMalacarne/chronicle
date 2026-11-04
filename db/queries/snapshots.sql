-- name: InsertSnapshot :one
-- Fails with unique_violation (23505) when (account_id, source, raw_hash) already
-- exists; callers should check GetSnapshotByHash first or map the error to "unchanged".
INSERT INTO snapshots (id, account_id, source, kind, fetched_at, raw, raw_hash, adapter_version, client_seq)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
-- Retention job. player_state and step_progress keep FKs to snapshots, so only
-- snapshots not referenced by them are removed.
DELETE FROM snapshots s
WHERE s.account_id = $1
  AND s.fetched_at < $2
  AND NOT EXISTS (SELECT 1 FROM player_state ps WHERE ps.snapshot_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM step_progress sp WHERE sp.evidence_snapshot_id = s.id);

-- Normalized rows. Bulk inserts use pgx CopyFrom (sqlc :copyfrom).

-- name: InsertSnapshotSkills :copyfrom
INSERT INTO snapshot_skills (snapshot_id, skill, level, xp, rank, boosted)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertSnapshotActivities :copyfrom
INSERT INTO snapshot_activities (snapshot_id, activity, score, rank)
VALUES ($1, $2, $3, $4);

-- name: InsertSnapshotQuests :copyfrom
INSERT INTO snapshot_quests (snapshot_id, quest, state)
VALUES ($1, $2, $3);

-- name: InsertSnapshotDiaryTasks :copyfrom
INSERT INTO snapshot_diary_tasks (snapshot_id, area, tier, task_index, done)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertSnapshotVars :copyfrom
INSERT INTO snapshot_vars (snapshot_id, kind, var_id, value)
VALUES ($1, $2, $3, $4);

-- name: ListSnapshotSkills :many
SELECT * FROM snapshot_skills WHERE snapshot_id = $1 ORDER BY skill;

-- name: ListSnapshotQuests :many
SELECT * FROM snapshot_quests WHERE snapshot_id = $1 ORDER BY quest;
