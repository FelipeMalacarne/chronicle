-- The durable event log and its consumers' cursors (migration 0031, docs/12
-- section 13.4). internal/store/eventlog is the only writer.

-- name: LockEventLogShared :exec
-- A writer holds it from its insert to its commit (internal/store/eventlog).
SELECT pg_advisory_xact_lock_shared(7123001);

-- name: TryLockEventLog :one
-- The reader takes it exclusively: no event is uncommitted while it holds it.
SELECT pg_try_advisory_xact_lock(7123001)::boolean AS locked;

-- name: InsertEvent :one
INSERT INTO events (account_id, type, subject, job_id, payload) VALUES ($1, $2, $3, $4, $5) RETURNING id, at;

-- name: ListEventsAfter :many
SELECT * FROM events WHERE id > $1 ORDER BY id LIMIT $2;

-- name: ListJobEvents :many
-- A job's own events and its runs'.
SELECT * FROM events WHERE account_id = $1 AND job_id = $2 ORDER BY id LIMIT $3;

-- name: LastEventOfType :one
SELECT * FROM events WHERE account_id = $1 AND type = $2 ORDER BY id DESC LIMIT 1;

-- name: GetEventCursor :one
SELECT last_id FROM event_cursors WHERE consumer = $1;

-- name: SetEventCursor :exec
INSERT INTO event_cursors (consumer, last_id) VALUES ($1, $2)
ON CONFLICT (consumer) DO UPDATE SET last_id = GREATEST(event_cursors.last_id, EXCLUDED.last_id), updated_at = now();

-- name: PruneFactChangedEvents :execrows
-- The fact.changed events older than the cutoff that every consumer has read.
DELETE FROM events e
WHERE e.type = 'fact.changed' AND e.at < $1
  AND NOT EXISTS (SELECT 1 FROM event_cursors c WHERE e.id > c.last_id);
