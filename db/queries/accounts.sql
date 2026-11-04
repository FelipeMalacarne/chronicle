-- name: CreateAccount :one
INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer, wom_player_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByRSN :one
SELECT * FROM accounts WHERE user_id = $1 AND rsn_normalized = $2;

-- name: ListAccountsByUser :many
SELECT * FROM accounts
WHERE user_id = $1
ORDER BY active DESC, created_at ASC;

-- name: ListActiveAccounts :many
-- Used by the scheduler: every account that should be synced.
SELECT * FROM accounts WHERE active = true ORDER BY created_at ASC;

-- name: UpdateAccountRSN :one
-- Name change detected: store the new name and append to name_history.
UPDATE accounts
SET rsn            = $2,
    rsn_normalized = $3,
    name_history   = name_history || $4::jsonb,
    updated_at     = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountSettings :one
UPDATE accounts
SET mode       = $2,
    guide_id   = $3,
    layer      = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetAccountWOMPlayerID :exec
UPDATE accounts SET wom_player_id = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountActive :exec
UPDATE accounts SET active = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = $1;
