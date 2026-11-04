-- Sync runs (docs/04 section 6): one row per attempt of a source for an
-- account. The scheduler and POST /api/sync write them; the UI reads them.

-- name: InsertSyncRun :one
INSERT INTO sync_runs (id, account_id, source, trigger_kind, status, started_at)
VALUES ($1, $2, $3, $4, 'running', now())
RETURNING *;

-- name: FinishSyncRun :one
UPDATE sync_runs SET
  status          = @status,
  finished_at     = now(),
  http_status     = @http_status,
  error           = @error,
  snapshot_id     = @snapshot_id,
  steps_changed   = @steps_changed,
  next_allowed_at = @next_allowed_at
WHERE id = @id
RETURNING *;

-- name: ListSyncRuns :many
SELECT * FROM sync_runs
WHERE account_id = $1 AND source = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListAccountsToSync :many
-- Active accounts with a name, with their newest run of the source (if any).
SELECT a.id, a.rsn, a.mode,
       r.created_at AS last_run_at, r.next_allowed_at
FROM accounts a
LEFT JOIN LATERAL (
  SELECT created_at, next_allowed_at FROM sync_runs
  WHERE account_id = a.id AND source = @source
  ORDER BY created_at DESC LIMIT 1
) r ON true
WHERE a.active AND a.rsn <> ''
ORDER BY a.created_at;
