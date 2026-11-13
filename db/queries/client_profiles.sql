-- Client profiles of the managed workers (docs/11 section 4.3, phase F5).
-- internal/store/workers is the only writer: a profile write locks the users
-- row first (LockUserForProfiles), an account write the accounts row
-- (LockAccountForWorker), so the cross-row invariants of migration 0013 are
-- checked and written under one lock.

-- name: LockUserForProfiles :one
SELECT id FROM users WHERE id = $1 FOR UPDATE;

-- name: GetClientProfileByName :one
SELECT * FROM client_profiles WHERE user_id = $1 AND name = $2;

-- name: GetClientProfile :one
SELECT * FROM client_profiles WHERE id = $1;

-- name: ListClientProfiles :many
SELECT * FROM client_profiles WHERE user_id = $1 ORDER BY name;

-- name: InsertClientProfile :exec
INSERT INTO client_profiles (id, user_id, name, parent_id) VALUES ($1, $2, $3, $4);

-- name: SetClientProfileParent :exec
UPDATE client_profiles SET parent_id = $2, updated_at = now() WHERE id = $1;

-- name: DeleteClientProfile :execrows
DELETE FROM client_profiles WHERE id = $1;

-- name: CountClientProfileChildren :one
SELECT count(*) FROM client_profiles WHERE parent_id = $1;

-- name: ListClientProfileAccounts :many
-- The accounts that use the profile, for the list and the delete refusal.
SELECT account_id FROM account_client WHERE profile_id = $1 ORDER BY account_id;

-- name: DeleteClientProfilePlugins :exec
DELETE FROM client_profile_plugins WHERE profile_id = $1;

-- name: InsertClientProfilePlugin :exec
INSERT INTO client_profile_plugins (profile_id, position, source, name, version, class_name, start_on_boot)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListClientProfilePlugins :many
SELECT * FROM client_profile_plugins WHERE profile_id = $1 ORDER BY position;

-- name: DeleteClientProfileSettings :exec
DELETE FROM client_profile_settings WHERE profile_id = $1;

-- name: InsertClientProfileSetting :exec
INSERT INTO client_profile_settings (profile_id, config_group, config_key, value) VALUES ($1, $2, $3, $4);

-- name: ListClientProfileSettings :many
SELECT * FROM client_profile_settings WHERE profile_id = $1 ORDER BY config_group, config_key;

-- name: GetAccountClient :one
SELECT * FROM account_client WHERE account_id = $1;

-- name: UpsertAccountClient :exec
INSERT INTO account_client (account_id, profile_id) VALUES ($1, $2)
ON CONFLICT (account_id) DO UPDATE SET profile_id = EXCLUDED.profile_id, updated_at = now();

-- name: DeleteAccountClientSettings :exec
DELETE FROM account_client_settings WHERE account_id = $1;

-- name: InsertAccountClientSetting :exec
INSERT INTO account_client_settings (account_id, config_group, config_key, value) VALUES ($1, $2, $3, $4);

-- name: ListAccountClientSettings :many
SELECT * FROM account_client_settings WHERE account_id = $1 ORDER BY config_group, config_key;
