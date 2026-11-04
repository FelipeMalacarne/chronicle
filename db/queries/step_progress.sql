-- name: UpsertStepProgress :one
-- Written by the rule engine after re-evaluation. Rows with manual_locked = true
-- are left untouched: the user pinned the status.
INSERT INTO step_progress (
  account_id, step_id, status, completed_at, evidence_snapshot_id, evidence, rule_hash, regressed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (account_id, step_id) DO UPDATE SET
  status               = EXCLUDED.status,
  completed_at         = EXCLUDED.completed_at,
  evidence_snapshot_id = EXCLUDED.evidence_snapshot_id,
  evidence             = EXCLUDED.evidence,
  rule_hash            = EXCLUDED.rule_hash,
  regressed_at         = EXCLUDED.regressed_at,
  updated_at           = now()
WHERE step_progress.manual_locked = false
RETURNING *;

-- name: SetStepManualStatus :one
-- Written by the user (UI, CLI, MCP). cause='user' event must be inserted by the caller.
INSERT INTO step_progress (
  account_id, step_id, status, completed_at, manual_locked, manual_at, user_note, updated_at
) VALUES ($1, $2, $3, $4, $5, now(), $6, now())
ON CONFLICT (account_id, step_id) DO UPDATE SET
  status        = EXCLUDED.status,
  completed_at  = EXCLUDED.completed_at,
  manual_locked = EXCLUDED.manual_locked,
  manual_at     = now(),
  user_note     = COALESCE(EXCLUDED.user_note, step_progress.user_note),
  updated_at    = now()
RETURNING *;

-- name: GetStepProgress :one
SELECT * FROM step_progress WHERE account_id = $1 AND step_id = $2;

-- name: ListStepProgressByAccount :many
SELECT * FROM step_progress WHERE account_id = $1 ORDER BY step_id;

-- name: ListStepProgressByStatus :many
SELECT * FROM step_progress
WHERE account_id = $1 AND status = $2
ORDER BY updated_at DESC;

-- name: ListStepBoard :many
-- Content steps joined with the account's progress (same shape as v_step_board,
-- restricted to one guide and one account).
SELECT cs.step_id, cs.section_id, cs.position, cs.is_manual, cs.layers, cs.modes,
       COALESCE(sp.status, 'pending')::text AS status,
       sp.completed_at, sp.manual_locked, sp.regressed_at, sp.user_note
FROM content_steps cs
LEFT JOIN step_progress sp ON sp.step_id = cs.step_id AND sp.account_id = $1
WHERE cs.guide_id = $2
ORDER BY cs.position;

-- name: CountStepProgressByStatus :many
SELECT status, count(*) AS total
FROM step_progress
WHERE account_id = $1
GROUP BY status;

-- name: InsertStepProgressEvent :exec
INSERT INTO step_progress_events (id, account_id, step_id, from_status, to_status, cause, snapshot_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListRecentStepProgressEvents :many
SELECT * FROM step_progress_events
WHERE account_id = $1 AND created_at >= $2
ORDER BY created_at DESC
LIMIT $3;
