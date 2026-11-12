-- Tokens used by the Microbot plugin (docs/06). The plaintext 'ipl_...' token is
-- shown once; only its sha256 is stored.

-- name: CreatePluginToken :one
INSERT INTO plugin_ingest_tokens (id, account_id, token_hash, label, scopes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPluginTokenByHash :one
-- Authentication path for POST /api/ingest and GET /api/ingest/manifest.
SELECT t.*, a.user_id
FROM plugin_ingest_tokens t
JOIN accounts a ON a.id = t.account_id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND a.active = true;

-- name: GetPluginTokenByHashForUpdate :one
-- Serializes pushes for one token so client_seq comparison and advancement are
-- atomic with the snapshot and merged-state writes.
SELECT t.*, a.user_id
FROM plugin_ingest_tokens t
JOIN accounts a ON a.id = t.account_id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND a.active = true
FOR UPDATE OF t;

-- name: ListPluginTokensByAccount :many
SELECT id, account_id, label, scopes, manifest_version, last_seq, last_seen_at, last_ip, created_at, revoked_at
FROM plugin_ingest_tokens
WHERE account_id = $1
ORDER BY created_at DESC;

-- name: TouchPluginToken :exec
-- Called after each accepted push: records the client sequence and manifest version.
UPDATE plugin_ingest_tokens
SET last_seq         = GREATEST(last_seq, $2),
    manifest_version = GREATEST(manifest_version, $3),
    last_seen_at     = now(),
    last_ip          = $4
WHERE id = $1;

-- name: UpdatePluginTokenAfterPush :exec
UPDATE plugin_ingest_tokens
SET last_seq         = $2,
    manifest_version = $3,
    last_seen_at     = now(),
    last_ip          = $4
WHERE id = $1;

-- name: UpdatePluginTokenScopes :exec
UPDATE plugin_ingest_tokens SET scopes = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokePluginToken :execrows
UPDATE plugin_ingest_tokens
SET revoked_at = now()
WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL;

-- name: EnsurePluginInstallation :exec
-- Tokenless AUTH_MODE=dev ingest: the installation's record for one account,
-- created on its first push (migration 0006).
INSERT INTO plugin_ingest_tokens (id, account_id, installation_id, label, scopes)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, installation_id) WHERE token_hash IS NULL DO NOTHING;

-- name: GetPluginInstallationForUpdate :one
-- Serializes the pushes of one dev installation for one account, like
-- GetPluginTokenByHashForUpdate does for a token.
SELECT t.*, a.user_id
FROM plugin_ingest_tokens t
JOIN accounts a ON a.id = t.account_id
WHERE t.account_id = $1 AND t.installation_id = $2 AND t.token_hash IS NULL
  AND t.revoked_at IS NULL AND a.active = true
FOR UPDATE OF t;

-- name: GetLatestPluginInstallation :one
-- The record an installation pushed through most recently: the account its
-- panel shows and the manifest version it last used. With user_id (a plugin
-- signed in through OIDC) only that user's accounts count.
SELECT t.*, a.user_id
FROM plugin_ingest_tokens t
JOIN accounts a ON a.id = t.account_id
WHERE t.installation_id = sqlc.arg(installation_id) AND t.token_hash IS NULL
  AND t.revoked_at IS NULL AND a.active = true
  AND (sqlc.narg(user_id)::text IS NULL OR a.user_id = sqlc.narg(user_id))
ORDER BY t.last_seen_at DESC NULLS LAST, t.created_at DESC
LIMIT 1;
