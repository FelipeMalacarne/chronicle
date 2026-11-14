-- The owner's marks on the items of a leg's loadout (docs/09 section 5).
-- Written only by internal/store/progress; read when readiness is computed.

-- name: ListLoadoutMarks :many
SELECT * FROM loadout_marks WHERE account_id = $1 ORDER BY guide_id, segment_id, item_key;

-- name: UpsertLoadoutMark :exec
INSERT INTO loadout_marks (account_id, guide_id, segment_id, item_key, note, marked_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (account_id, guide_id, segment_id, item_key) DO UPDATE SET
  note      = EXCLUDED.note,
  marked_at = now();

-- name: DeleteLoadoutMark :execrows
DELETE FROM loadout_marks
WHERE account_id = $1 AND guide_id = $2 AND segment_id = $3 AND item_key = $4;
