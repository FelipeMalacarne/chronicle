-- name: InsertRecordedEvent :execrows
-- A resent event (same recording and seq) is already stored: nothing changes.
INSERT INTO recorded_events (account_id, recording, seq, at, kind, event, snapshot_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, recording, seq) DO NOTHING;

-- name: ListRecordedEvents :many
-- The account's events from since (inclusive) to until (exclusive), in the
-- order they happened; limit bounds the page.
SELECT event FROM recorded_events
WHERE account_id = sqlc.arg(account_id)
  AND at >= sqlc.arg(since)
  AND at < sqlc.arg(until)
ORDER BY at, recording, seq
LIMIT sqlc.arg(max_rows);

-- name: DeleteRecordedEvents :execrows
-- The owner's request: every event of the account, or of one recording.
DELETE FROM recorded_events
WHERE account_id = sqlc.arg(account_id)
  AND (sqlc.narg(recording)::text IS NULL OR recording = sqlc.narg(recording));
