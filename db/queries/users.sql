-- Users are keyed by the OIDC subject. In AUTH_MODE=dev the server upserts a
-- single user with subject 'dev'.

-- name: UpsertUserBySubject :one
INSERT INTO users (id, subject, issuer, username, email, display_name, last_login_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (subject) DO UPDATE SET
  issuer        = EXCLUDED.issuer,
  username      = EXCLUDED.username,
  email         = EXCLUDED.email,
  display_name  = EXCLUDED.display_name,
  last_login_at = now()
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserBySubject :one
SELECT * FROM users WHERE subject = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- AUTH_MODE=oidc (docs/04 section 8). A user is (issuer, subject): the
-- update only happens for the same issuer, so a subject that already belongs
-- to the dev user or to another issuer returns no row and the login fails.
-- username is kept from the first login (it is UNIQUE; preferred_username may
-- change at the issuer).

-- name: UpsertOIDCUser :one
INSERT INTO users (id, subject, issuer, username, email, display_name, last_login_at)
VALUES (@id, @subject, @issuer::text, @username, @email, @display_name, now())
ON CONFLICT (subject) DO UPDATE SET
  email         = EXCLUDED.email,
  display_name  = EXCLUDED.display_name,
  last_login_at = now()
WHERE users.issuer = EXCLUDED.issuer
RETURNING *;

-- name: UsernameTaken :one
SELECT EXISTS (SELECT 1 FROM users WHERE username = $1);

-- name: CountOIDCUsersExcept :one
-- OIDC users other than this subject (the first-login account claim).
SELECT count(*) FROM users WHERE issuer IS NOT NULL AND subject <> $1;

-- name: ClaimDevAccounts :execrows
-- First OIDC login: the dev user's accounts move to the new user.
UPDATE accounts
SET user_id = @user_id, updated_at = now()
WHERE user_id IN (SELECT id FROM users WHERE subject = 'dev' AND issuer IS NULL);

-- name: AccountOwnedBy :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE id = @account_id AND user_id = @user_id);
