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

-- Recurring routines (migration 0015): each with its last run, from which
-- the agent counts the interval and sees a failure the owner has not
-- answered.

-- name: ListRecurring :many
SELECT r.id, r.name, r.enabled, r.clock, r.interval_min, r.priority, r.spec, r.updated_at,
       COALESCE(last.state, '') AS last_state, last.started_at AS last_started_at, last.finished_at AS last_finished_at
FROM routines r
LEFT JOIN LATERAL (
  SELECT rr.state, rr.started_at, rr.finished_at FROM routine_runs rr
  WHERE rr.routine_id = r.id ORDER BY rr.started_at DESC LIMIT 1
) last ON true
WHERE r.account_id = $1 AND r.kind = 'recurring'
ORDER BY r.priority DESC, r.name, r.id;

-- name: CountRecurring :one
SELECT count(*) FROM routines WHERE account_id = $1 AND kind = 'recurring';

-- name: InsertRecurring :exec
INSERT INTO routines (id, account_id, kind, name, enabled, clock, interval_min, priority, spec)
VALUES ($1, $2, 'recurring', $3, $4, $5, $6, $7, $8);

-- name: UpdateRecurring :execrows
UPDATE routines
SET name = $3, enabled = $4, clock = $5, interval_min = $6, priority = $7, spec = $8, updated_at = now()
WHERE id = $1 AND account_id = $2 AND kind = 'recurring';

-- name: DeleteRecurring :execrows
DELETE FROM routines WHERE id = $1 AND account_id = $2 AND kind = 'recurring';

-- name: GetRecurringID :one
SELECT id FROM routines WHERE id = $1 AND account_id = $2 AND kind = 'recurring';
