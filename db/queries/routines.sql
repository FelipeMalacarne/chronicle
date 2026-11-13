-- Routines and runs on the server (docs/11 section 4.4, F6; migration 0014).
-- internal/store/routines is the only writer.

-- name: ListTriggers :many
SELECT * FROM routines WHERE account_id = $1 AND kind = 'trigger' ORDER BY event;

-- name: UpsertTrigger :exec
INSERT INTO routines (id, account_id, kind, event, enabled, spec)
VALUES ($1, $2, 'trigger', $3, $4, $5)
ON CONFLICT (account_id, kind, event) DO UPDATE
SET enabled = EXCLUDED.enabled, spec = EXCLUDED.spec, updated_at = now();

-- name: GetTrigger :one
SELECT * FROM routines WHERE account_id = $1 AND kind = 'trigger' AND event = $2;

-- name: GetRoutineRunForUpdate :one
SELECT account_id, state, trail_len FROM routine_runs WHERE id = $1 FOR UPDATE;

-- name: UpsertRoutineRun :execrows
-- A snapshot with fewer trail lines than the stored one is older: ignored.
-- A run never moves to another account.
INSERT INTO routine_runs (
  id, account_id, routine_id, agent, agent_run_id, class_name, stop, trigger, state,
  started_at, finished_at, from_pos, result, error, trail_len
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (id) DO UPDATE
SET agent       = EXCLUDED.agent,
    state       = EXCLUDED.state,
    finished_at = EXCLUDED.finished_at,
    from_pos    = EXCLUDED.from_pos,
    result      = EXCLUDED.result,
    error       = EXCLUDED.error,
    trail_len   = EXCLUDED.trail_len,
    updated_at  = now()
WHERE routine_runs.account_id = EXCLUDED.account_id
  AND routine_runs.trail_len <= EXCLUDED.trail_len;

-- name: InsertRunEvent :exec
INSERT INTO run_events (run_id, seq, at, state, text) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, seq) DO NOTHING;

-- name: ListRoutineRuns :many
SELECT * FROM routine_runs WHERE account_id = $1 ORDER BY started_at DESC, id LIMIT $2;

-- name: ListRunEvents :many
SELECT * FROM run_events WHERE run_id = ANY(sqlc.arg(run_ids)::text[]) ORDER BY run_id, seq;
