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
