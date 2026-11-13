-- The owner's marks on objectives (docs/09 section 5). Written only by
-- internal/store/progress; read when verdicts are overlaid for statuses.

-- name: ListObjectiveMarks :many
SELECT * FROM objective_marks WHERE account_id = $1 ORDER BY objective_key;

-- name: UpsertObjectiveMark :exec
INSERT INTO objective_marks (account_id, objective_key, note, marked_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (account_id, objective_key) DO UPDATE SET
  note      = EXCLUDED.note,
  marked_at = now();

-- name: DeleteObjectiveMark :execrows
DELETE FROM objective_marks WHERE account_id = $1 AND objective_key = $2;
