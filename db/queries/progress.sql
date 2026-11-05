-- Progress over objectives (docs/09 section 5). The engine computes the
-- derived step status; these queries only store verdicts, marks, history and
-- cursors. Every step is addressed by (guide_id, step_id).

-- name: ListObjectiveProgress :many
SELECT * FROM objective_progress WHERE account_id = $1 ORDER BY objective_key;

-- name: UpsertObjectiveProgress :exec
-- A verdict change: the evidence is replaced.
INSERT INTO objective_progress (
  account_id, objective_key, verdict, evidence_snapshot_id, evidence,
  first_met_at, met_in_guide_id, regressed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (account_id, objective_key) DO UPDATE SET
  verdict              = EXCLUDED.verdict,
  evidence_snapshot_id = EXCLUDED.evidence_snapshot_id,
  evidence             = EXCLUDED.evidence,
  first_met_at         = EXCLUDED.first_met_at,
  met_in_guide_id      = EXCLUDED.met_in_guide_id,
  regressed_at         = EXCLUDED.regressed_at,
  updated_at           = now();

-- name: SetObjectiveRegressed :exec
-- A latched met keeps its verdict and evidence; only regressed_at moves.
UPDATE objective_progress SET regressed_at = $3, updated_at = now()
WHERE account_id = $1 AND objective_key = $2 AND verdict = 'met';

-- name: InsertObjectiveProgressEvent :exec
INSERT INTO objective_progress_events (id, account_id, objective_key, from_verdict, to_verdict, snapshot_id, at)
VALUES ($1, $2, $3, $4, $5, $6, clock_timestamp());

-- name: ListStepMarksByAccount :many
SELECT * FROM step_marks WHERE account_id = $1 ORDER BY guide_id, step_id;

-- name: ListStepMarksByGuide :many
SELECT * FROM step_marks WHERE account_id = $1 AND guide_id = $2 ORDER BY step_id;

-- name: UpsertStepMark :exec
-- Owner writes only (marks, confirming implied); the engine uses InsertImpliedMark.
INSERT INTO step_marks (account_id, guide_id, step_id, kind, implied_by_step_id, implied_at, note, marked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (account_id, guide_id, step_id) DO UPDATE SET
  kind               = EXCLUDED.kind,
  implied_by_step_id = EXCLUDED.implied_by_step_id,
  implied_at         = EXCLUDED.implied_at,
  note               = EXCLUDED.note,
  marked_at          = now();

-- name: InsertImpliedMark :execrows
-- The implied cascade's only write (docs/09 section 9.1). It never replaces a
-- mark: the owner's mark always wins, so a conflict leaves the row as it is
-- (CLAUDE.md rule 2). Owner writes use UpsertStepMark.
INSERT INTO step_marks (account_id, guide_id, step_id, kind, implied_by_step_id, implied_at, marked_at)
VALUES (@account_id, @guide_id, @step_id, 'implied', @implied_by_step_id, @implied_at, now())
ON CONFLICT (account_id, guide_id, step_id) DO NOTHING;

-- name: DeleteStepMark :exec
DELETE FROM step_marks WHERE account_id = $1 AND guide_id = $2 AND step_id = $3;

-- name: InsertStepStatusEvent :exec
-- clock_timestamp(), not now(): several events of one transaction must keep
-- their order, because the newest event of a step is its last known status.
INSERT INTO step_status_events (id, account_id, guide_id, step_id, from_status, to_status, cause, snapshot_id, at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, clock_timestamp());

-- name: ListLastStepStatuses :many
SELECT DISTINCT ON (guide_id, step_id) guide_id, step_id, to_status
FROM step_status_events
WHERE account_id = $1
ORDER BY guide_id, step_id, at DESC;

-- name: ListLastStepStatusesByGuide :many
SELECT DISTINCT ON (step_id) step_id, to_status
FROM step_status_events
WHERE account_id = $1 AND guide_id = $2
ORDER BY step_id, at DESC;

-- name: ListRecentStepStatusEvents :many
-- With the source of the snapshot that caused an engine change (hiscores,
-- plugin), for "what changed in this session".
SELECT e.id, e.guide_id, e.step_id, e.from_status, e.to_status, e.cause, e.snapshot_id, e.at,
       COALESCE(s.source, '')::text AS source
FROM step_status_events e
LEFT JOIN snapshots s ON s.id = e.snapshot_id
WHERE e.account_id = $1 AND e.guide_id = $2
ORDER BY e.at DESC
LIMIT $3;

-- name: ListRecentObjectiveProgressEvents :many
SELECT e.id, e.objective_key, e.from_verdict, e.to_verdict, e.snapshot_id, e.at,
       COALESCE(s.source, '')::text AS source
FROM objective_progress_events e
LEFT JOIN snapshots s ON s.id = e.snapshot_id
WHERE e.account_id = $1
ORDER BY e.at DESC
LIMIT $2;

-- name: SetAccountGuide :exec
UPDATE accounts SET guide_id = $2, updated_at = now() WHERE id = $1;

-- name: ListRouteCursors :many
SELECT * FROM route_cursor WHERE account_id = $1 ORDER BY guide_id;

-- name: GetRouteCursor :one
SELECT * FROM route_cursor WHERE account_id = $1 AND guide_id = $2;

-- name: UpsertRouteCursor :exec
INSERT INTO route_cursor (account_id, guide_id, step_id, cause, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (account_id, guide_id) DO UPDATE SET
  step_id    = EXCLUDED.step_id,
  cause      = EXCLUDED.cause,
  updated_at = now();

-- name: DeleteRouteCursor :exec
DELETE FROM route_cursor WHERE account_id = $1 AND guide_id = $2;

-- name: ListOrphanStepMarks :many
-- Marks whose step is not in the loaded content of the guide (docs/04 section
-- 3.2, rule 4): kept for the owner to decide, never deleted by the engine.
SELECT m.* FROM step_marks m
WHERE m.account_id = $1 AND m.guide_id = $2
  AND NOT EXISTS (
    SELECT 1 FROM content_steps cs WHERE cs.guide_id = m.guide_id AND cs.step_id = m.step_id
  )
ORDER BY m.step_id;

-- name: LockAccountProgress :one
-- Serializes reevaluations and owner writes of one account.
SELECT id, guide_id, layer, mode FROM accounts WHERE id = $1 FOR UPDATE;

-- Applying a step id alias (docs/04 section 3.2, rule 2). A mark already at
-- the new id wins; the old one is dropped.

-- name: MoveStepMark :execrows
UPDATE step_marks m SET step_id = @new_step_id
WHERE m.guide_id = @guide_id AND m.step_id = @old_step_id
  AND NOT EXISTS (
    SELECT 1 FROM step_marks n
    WHERE n.account_id = m.account_id AND n.guide_id = m.guide_id AND n.step_id = @new_step_id
  );

-- name: ListMovedStepMarks :many
SELECT account_id, kind FROM step_marks WHERE guide_id = $1 AND step_id = $2;

-- name: DeleteStepMarksByStep :exec
DELETE FROM step_marks WHERE guide_id = $1 AND step_id = $2;

-- name: MoveStepStatusEvents :exec
UPDATE step_status_events SET step_id = @new_step_id
WHERE guide_id = @guide_id AND step_id = @old_step_id;

-- name: MoveRouteCursors :exec
UPDATE route_cursor SET step_id = @new_step_id, updated_at = now()
WHERE guide_id = @guide_id AND step_id = @old_step_id;
