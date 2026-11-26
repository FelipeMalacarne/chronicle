-- name: GetConductorState :one
SELECT * FROM conductor_state WHERE account_id = $1;

-- name: ListConductorAccounts :many
-- The accounts the conductor acts on: idle or running.
SELECT account_id FROM conductor_state WHERE state IN ('idle', 'running') ORDER BY account_id;

-- name: SetConductorState :exec
-- The owner turns the conductor on or off: whatever it was doing.
INSERT INTO conductor_state (account_id, state, job_id, reason, since)
VALUES (@account_id, @state, sqlc.narg('job_id'), @reason, now())
ON CONFLICT (account_id) DO UPDATE
SET state = EXCLUDED.state, job_id = EXCLUDED.job_id, reason = EXCLUDED.reason, since = now();

-- name: MoveConductorState :execrows
-- The conductor moves only from the state it read: a turn on or off by the
-- owner in between wins. since changes only with the state.
UPDATE conductor_state
SET state = @state, job_id = sqlc.narg('job_id'), reason = @reason,
    since = CASE WHEN state = @state THEN since ELSE now() END
WHERE account_id = @account_id AND state = @from_state AND job_id IS NOT DISTINCT FROM sqlc.narg('from_job_id');
