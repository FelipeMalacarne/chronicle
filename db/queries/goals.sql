-- Goals (F9): a goal is an objective the owner pins; objective_key is the
-- canonical key (entities.ObjectiveKey). The plan comes from the content and
-- the requirements graph, never from this table.

-- name: ListGoalsByAccount :many
SELECT g.*, COALESCE(op.verdict, 'unknown')::text AS verdict, op.regressed_at AS objective_regressed_at
FROM goals g
LEFT JOIN objective_progress op ON op.account_id = g.account_id AND op.objective_key = g.objective_key
WHERE g.account_id = $1 AND g.status <> 'abandoned'
ORDER BY g.priority, g.created_at;

-- name: InsertGoal :one
INSERT INTO goals (id, account_id, objective_key, label, priority)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: FindActiveGoal :one
SELECT * FROM goals WHERE account_id = $1 AND objective_key = $2 AND status <> 'abandoned' LIMIT 1;

-- name: DeleteGoal :execrows
DELETE FROM goals WHERE id = $1 AND account_id = $2;

-- name: AchieveGoals :exec
-- A goal is achieved when its objective becomes met (with evidence).
UPDATE goals SET status = 'achieved', achieved_at = now()
WHERE account_id = @account_id AND status = 'active' AND objective_key = ANY(@keys::text[]);
