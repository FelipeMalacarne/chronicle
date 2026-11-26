-- Jobs (migration 0031, docs/12 section 13). internal/store/jobs is the
-- only writer; routines.RecordRun inserts through it for the runs it adopts.

-- name: InsertJob :exec
INSERT INTO jobs (id, account_id, kind, spec, goal, metric, stall_after_s, source, plan_id, task_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1 AND account_id = $2;

-- name: GetJobForUpdate :one
SELECT * FROM jobs WHERE id = $1 FOR UPDATE;

-- name: ListJobs :many
SELECT * FROM jobs
WHERE account_id = $1 AND (sqlc.narg(state)::text IS NULL OR state = sqlc.narg(state)::text)
ORDER BY created_at DESC, id
LIMIT $2;

-- name: ListActiveJobs :many
SELECT * FROM jobs WHERE account_id = $1 AND state IN ('queued', 'running', 'stalled') ORDER BY created_at, id;

-- name: ListActiveJobAccounts :many
SELECT DISTINCT account_id FROM jobs WHERE state IN ('running', 'stalled') ORDER BY account_id;

-- name: TransitionJob :execrows
-- The state machine's check is core/job's; the from-state guard keeps two
-- writers from both moving the same job.
UPDATE jobs SET
  state       = sqlc.arg(to_state)::text,
  reason      = sqlc.arg(reason)::text,
  progress    = COALESCE(sqlc.narg(progress)::jsonb, progress),
  deadline_at = sqlc.narg(deadline_at)::timestamptz,
  started_at  = COALESCE(started_at, sqlc.narg(started_at)::timestamptz),
  finished_at = sqlc.narg(finished_at)::timestamptz
WHERE id = sqlc.arg(id)::text AND state = sqlc.arg(from_state)::text;

-- name: UpdateJobProgress :execrows
UPDATE jobs SET progress = $2, deadline_at = $3 WHERE id = $1 AND state = 'running';

-- name: NextJobDeadline :one
SELECT min(deadline_at)::timestamptz AS at FROM jobs WHERE state = 'running' AND deadline_at IS NOT NULL;

-- name: ListDueJobs :many
SELECT * FROM jobs WHERE state = 'running' AND deadline_at IS NOT NULL AND deadline_at <= $1 ORDER BY deadline_at;

-- name: LatestPlanJobs :many
-- The newest job of each task of a plan that was not canceled: a new run of a
-- task replaces the earlier one's proof.
SELECT DISTINCT ON (task_key) task_key, id, state FROM jobs
WHERE account_id = $1 AND plan_id = $2 AND kind = 'ge_sell' AND state <> 'canceled'
ORDER BY task_key, created_at DESC, id;

-- name: ListRunsToStopForSucceededJobs :many
-- The runs still going whose job the tracker proved: the agent is told to
-- stop them (run_until). Runs older than the longest wait_until are gone.
SELECT r.id AS run_uid, r.account_id, j.id AS job_id, j.goal
FROM routine_runs r JOIN jobs j ON j.id = r.job_id
WHERE j.state = 'succeeded' AND r.state NOT IN ('done', 'failed', 'canceled')
  AND r.started_at > now() - interval '13 hours'
ORDER BY r.started_at;

-- name: ListActiveRunUIDsOfJob :many
SELECT id FROM routine_runs WHERE job_id = $1 AND state NOT IN ('done', 'failed', 'canceled') ORDER BY started_at;

-- name: ListQueuedJobsWithActiveRun :many
-- A queued job whose run is going: run.started was missed (the safety tick
-- moves it to running).
SELECT j.* FROM jobs j
WHERE j.state = 'queued'
  AND EXISTS (SELECT 1 FROM routine_runs r WHERE r.job_id = j.id AND r.state NOT IN ('done', 'failed', 'canceled'))
ORDER BY j.created_at, j.id;

-- name: ListStalledJobsPastCutoff :many
-- Stalled jobs with no run going whose last run ended before the cutoff (or
-- that never had one): nothing is left to prove them.
SELECT j.* FROM jobs j
WHERE j.state = 'stalled'
  AND NOT EXISTS (SELECT 1 FROM routine_runs r WHERE r.job_id = j.id AND r.state NOT IN ('done', 'failed', 'canceled'))
  AND COALESCE((SELECT max(COALESCE(r.finished_at, r.started_at)) FROM routine_runs r WHERE r.job_id = j.id), j.started_at, j.created_at) < $1
ORDER BY j.created_at, j.id;

-- name: LastRunStateOfJob :one
SELECT state FROM routine_runs WHERE job_id = $1 ORDER BY started_at DESC, id DESC LIMIT 1;

-- name: FindQueuedTaskJob :one
-- The job the server queued for a task run whose agent does not carry the job
-- id (it predates the job_id feature): the newest queued one of the task, with
-- no run yet.
SELECT j.id FROM jobs j
WHERE j.account_id = $1 AND j.plan_id = $2 AND j.task_key = $3 AND j.kind = $4 AND j.state = 'queued'
  AND NOT EXISTS (SELECT 1 FROM routine_runs r WHERE r.job_id = j.id)
ORDER BY j.created_at DESC, j.id
LIMIT 1;

-- name: GetJobAccount :one
SELECT account_id FROM jobs WHERE id = $1;
