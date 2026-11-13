-- Plans (migration 0018, docs/10 section 9): the goal and custom plans of an
-- account and their items; the route plan of a guide is the content and the
-- cursor, with no row. Written only by internal/store/plans.

-- name: ListPlans :many
SELECT * FROM plans WHERE account_id = $1 ORDER BY created_at, id;

-- name: GetPlan :one
SELECT * FROM plans WHERE id = $1 AND account_id = $2;

-- name: ListPlanItems :many
SELECT * FROM plan_items WHERE plan_id = ANY(@plan_ids::text[]) ORDER BY plan_id, position;

-- name: InsertPlan :exec
INSERT INTO plans (id, account_id, kind, title, guide_id, goal_id) VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertPlanItem :exec
INSERT INTO plan_items (plan_id, position, kind, guide_id, step_id, recipe_key, task)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: DeletePlan :execrows
DELETE FROM plans WHERE id = $1 AND account_id = $2;

-- name: GetActivePlan :one
SELECT active_plan_id, guide_id FROM accounts WHERE id = $1;

-- name: SetActivePlan :execrows
UPDATE accounts SET active_plan_id = sqlc.narg(plan_id), updated_at = now() WHERE id = @account_id;

-- name: CountPlans :one
SELECT count(*) FROM plans WHERE account_id = $1;

-- name: GetGoalOfAccount :one
SELECT id, objective_key, label FROM goals WHERE id = $1 AND account_id = $2;

-- name: MovePlanItemSteps :exec
-- A step id the guide renamed (step_id_aliases) moves in the plans too.
UPDATE plan_items SET step_id = @new_step_id::text
WHERE kind = 'step' AND guide_id = @guide_id::text AND step_id = @old_step_id::text;
