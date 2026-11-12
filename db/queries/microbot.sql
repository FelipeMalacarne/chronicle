-- name: AppendMicrobotAudit :one
INSERT INTO microbot_audit (actor, origin, action, params, ok, error, account_id, agent_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListMicrobotAudit :many
-- account_id = '' lists every account.
SELECT * FROM microbot_audit
WHERE sqlc.arg(account_id)::text = '' OR account_id = sqlc.arg(account_id)
ORDER BY at DESC, id DESC
LIMIT sqlc.arg(row_limit);
