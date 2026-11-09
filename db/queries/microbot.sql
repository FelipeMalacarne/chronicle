-- name: AppendMicrobotAudit :one
INSERT INTO microbot_audit (actor, origin, action, params, ok, error)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListMicrobotAudit :many
SELECT * FROM microbot_audit ORDER BY at DESC, id DESC LIMIT $1;
