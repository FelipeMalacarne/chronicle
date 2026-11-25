-- The durable event log and its consumers' cursors (migration 0031, docs/12
-- section 13.4). internal/store/eventlog is the only writer.

-- name: InsertEvent :one
INSERT INTO events (account_id, type, subject, payload) VALUES ($1, $2, $3, $4) RETURNING id, at;

-- name: ListEventsAfter :many
SELECT * FROM events WHERE id > $1 ORDER BY id LIMIT $2;

-- name: ListJobEvents :many
-- A job's own events and its runs' (their payload names the job).
SELECT * FROM events WHERE account_id = $1 AND (subject = sqlc.arg(job_id)::text OR payload->>'job' = sqlc.arg(job_id)::text) ORDER BY id LIMIT sqlc.arg(max_rows);

-- name: GetEventCursor :one
SELECT last_id FROM event_cursors WHERE consumer = $1;

-- name: SetEventCursor :exec
INSERT INTO event_cursors (consumer, last_id) VALUES ($1, $2)
ON CONFLICT (consumer) DO UPDATE SET last_id = GREATEST(event_cursors.last_id, EXCLUDED.last_id), updated_at = now();
