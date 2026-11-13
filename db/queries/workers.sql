-- Managed workers and their game sessions (docs/11 sections 4.2, 4.5, 13.5).
-- internal/store/workers is the only writer: every write locks the account
-- first (LockAccountForWorker) so the cross-table invariants of migration 0012
-- are checked and written under one lock per account.

-- name: LockAccountForWorker :one
SELECT id, user_id, rsn, managed FROM accounts WHERE id = $1 FOR UPDATE;

-- name: SetAccountManaged :exec
UPDATE accounts SET managed = $2, updated_at = now() WHERE id = $1;

-- name: UpsertGameSession :exec
-- A new session replaces the old one; the last login belonged to the old one.
INSERT INTO game_sessions (account_id, character_name, key_id, nonce, ciphertext, pushed_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (account_id) DO UPDATE
SET character_name    = EXCLUDED.character_name,
    key_id            = EXCLUDED.key_id,
    nonce             = EXCLUDED.nonce,
    ciphertext        = EXCLUDED.ciphertext,
    pushed_at         = now(),
    pushed_by         = EXCLUDED.pushed_by,
    last_login_at     = NULL,
    last_login_result = '';

-- name: GetGameSession :one
-- The sealed session, for Session only (never for the API).
SELECT * FROM game_sessions WHERE account_id = $1;

-- name: GetGameSessionInfo :one
-- Everything but the secret.
SELECT account_id, character_name, key_id, pushed_at, pushed_by, last_login_at, last_login_result
FROM game_sessions WHERE account_id = $1;

-- name: DeleteGameSession :execrows
DELETE FROM game_sessions WHERE account_id = $1;

-- name: RecordGameSessionLogin :execrows
UPDATE game_sessions SET last_login_at = now(), last_login_result = $2 WHERE account_id = $1;

-- name: GetWorkerForUpdate :one
SELECT * FROM workers WHERE account_id = $1 FOR UPDATE;

-- name: UpsertWorkerDesired :exec
-- started_at / stopped_at record the last change of desired_state to running
-- / stopped (a repeated state keeps them); a start clears stopped_at and
-- last_error. An empty image keeps the stored one.
INSERT INTO workers (account_id, desired_state, world, image, started_at, stopped_at)
VALUES (
  sqlc.arg(account_id), sqlc.arg(desired_state)::text, sqlc.arg(world), sqlc.arg(image),
  CASE WHEN sqlc.arg(desired_state)::text = 'running' THEN now() END,
  CASE WHEN sqlc.arg(desired_state)::text = 'stopped' THEN now() END
)
ON CONFLICT (account_id) DO UPDATE
SET desired_state = EXCLUDED.desired_state,
    world         = EXCLUDED.world,
    image         = COALESCE(NULLIF(EXCLUDED.image, ''), workers.image),
    started_at    = CASE WHEN EXCLUDED.desired_state = 'running' AND workers.desired_state <> 'running'
                         THEN now() ELSE workers.started_at END,
    stopped_at    = CASE WHEN EXCLUDED.desired_state = 'running' AND workers.desired_state <> 'running' THEN NULL
                         WHEN EXCLUDED.desired_state = 'stopped' AND workers.desired_state <> 'stopped' THEN now()
                         ELSE workers.stopped_at END,
    last_error    = CASE WHEN EXCLUDED.desired_state = 'running' AND workers.desired_state <> 'running'
                         THEN '' ELSE workers.last_error END,
    updated_at    = now();

-- name: SetRunningWorkerImage :execrows
-- The image of a worker that is meant to run, for a restart on the server's
-- current one (docs/11 section 7.2); a stopped worker is left as it is, so a
-- restart never starts it again.
UPDATE workers SET image = sqlc.arg(image), updated_at = now()
WHERE account_id = sqlc.arg(account_id) AND desired_state = 'running';

-- name: SetWorkerError :execrows
UPDATE workers SET last_error = $2, updated_at = now() WHERE account_id = $1;

-- name: SetWorkerPluginToken :exec
UPDATE workers SET plugin_token_id = $2, updated_at = now() WHERE account_id = $1;

-- name: ListWorkers :many
-- Every managed account: the ones with a worker row and the ones with only a
-- session (never started: desired 'stopped'), with the rsn and the session
-- info. account_id = '' lists all; user_id = '' does not filter by owner.
SELECT a.id AS account_id, a.user_id, a.rsn, a.managed,
       (w.account_id IS NOT NULL)::boolean AS has_worker,
       COALESCE(w.desired_state, 'stopped')::text AS desired_state,
       COALESCE(w.world, 0)::integer AS world,
       COALESCE(w.image, '')::text AS image,
       COALESCE(w.plugin_token_id, '')::text AS plugin_token_id,
       w.started_at, w.stopped_at, w.updated_at,
       COALESCE(w.last_error, '')::text AS last_error,
       (s.account_id IS NOT NULL)::boolean AS has_session,
       COALESCE(s.character_name, '')::text AS character_name,
       COALESCE(s.key_id, '')::text AS key_id,
       s.pushed_at,
       COALESCE(s.pushed_by, '')::text AS pushed_by,
       s.last_login_at,
       COALESCE(s.last_login_result, '')::text AS last_login_result
FROM accounts a
LEFT JOIN workers w ON w.account_id = a.id
LEFT JOIN game_sessions s ON s.account_id = a.id
WHERE (w.account_id IS NOT NULL OR s.account_id IS NOT NULL)
  AND (sqlc.arg(account_id)::text = '' OR a.id = sqlc.arg(account_id))
  AND (sqlc.arg(user_id)::text = '' OR a.user_id = sqlc.arg(user_id))
ORDER BY a.created_at ASC, a.id ASC;

-- name: OtherPluginSeenSince :one
-- Whether another plugin installation of the account (not the worker's own
-- token) pushed since the given time: the owner is playing on another client,
-- and starting the worker would log them out (docs/11 section 6.2).
SELECT EXISTS (
  SELECT 1 FROM plugin_ingest_tokens t
  WHERE t.account_id = sqlc.arg(account_id)
    AND t.revoked_at IS NULL
    AND t.last_seen_at > sqlc.arg(since)::timestamptz
    AND t.id <> COALESCE((SELECT w.plugin_token_id FROM workers w WHERE w.account_id = sqlc.arg(account_id)), '')
)::boolean AS seen;
