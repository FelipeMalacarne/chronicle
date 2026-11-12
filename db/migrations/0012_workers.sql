-- +goose Up
-- Managed Microbot workers (docs/11 sections 4.2, 4.5 and 13.5): an account
-- can have one worker, a pod the server creates, which logs in with the
-- account's game session (stored encrypted, never returned by the API).
--
-- Invariants held by internal/store/workers, not by the database (they span
-- tables, and a deferred trigger for each would cost more than it protects):
--   * accounts.managed is true exactly while the account has a game_sessions
--     row (PutSession sets it, DeleteSession clears it);
--   * a worker is desired 'running' only while the account has a session,
--     and the session cannot be deleted while the worker is 'running'.
-- Every write of the repository locks the accounts row first, so these
-- checks and the writes they guard are serialized per account.

ALTER TABLE accounts ADD COLUMN managed BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN accounts.managed IS 'The account may run a managed worker; true only while a game_sessions row exists (kept by internal/store/workers).';

CREATE TABLE game_sessions (
  account_id        TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  -- the character's display name (JX_DISPLAY_NAME), shown by the panel
  character_name    TEXT NOT NULL,
  -- id of the CHRONICLE_VAULT_KEY that sealed the row: the first 8 hex of its
  -- SHA-256 (internal/vault); a row of another key is refused, not guessed
  key_id            TEXT NOT NULL CHECK (key_id ~ '^[0-9a-f]{8}$'),
  -- AES-256-GCM of the JSON agent.GameSession, with the account id as the
  -- additional data, so a row copied to another account does not open
  nonce             BYTEA NOT NULL CHECK (octet_length(nonce) = 12),
  ciphertext        BYTEA NOT NULL CHECK (octet_length(ciphertext) > 16),
  pushed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- user id that connected the session ('dev' in AUTH_MODE=dev)
  pushed_by         TEXT NOT NULL,
  last_login_at     TIMESTAMPTZ,
  -- outcome of the last login a worker made with this session ('' = none yet)
  last_login_result TEXT NOT NULL DEFAULT ''
);
COMMENT ON TABLE game_sessions IS 'Encrypted Jagex game session of an account (docs/11 section 4.2); leaves the server only through the link, to the agent of that account.';

CREATE TABLE workers (
  account_id      TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  desired_state   TEXT NOT NULL CHECK (desired_state IN ('running', 'stopped')),
  -- world to log in to later; 0 keeps the client profile's
  world           INTEGER NOT NULL DEFAULT 0 CHECK (world >= 0),
  -- chronicle-worker image the pod runs (WORKER_IMAGE when it was started)
  image           TEXT NOT NULL DEFAULT '',
  -- the worker's own ipl_ token for the Chronicle Link in the pod; revoked
  -- and replaced on every mint, cleared on revoke
  plugin_token_id TEXT REFERENCES plugin_ingest_tokens(id) ON DELETE SET NULL,
  -- last change of desired_state to 'running' / to 'stopped'
  started_at      TIMESTAMPTZ,
  stopped_at      TIMESTAMPTZ,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error      TEXT NOT NULL DEFAULT ''
);
COMMENT ON TABLE workers IS 'Managed workers only (docs/11 section 4.5): what the reconciler should run per account. Attached agents live in the link hub, not here.';

-- Which account and which agent each audited action went to (docs/11 section
-- 4.5); empty on the rows from before this migration.
ALTER TABLE microbot_audit
  ADD COLUMN account_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN agent_id   TEXT NOT NULL DEFAULT '';
CREATE INDEX microbot_audit_account_at_idx ON microbot_audit (account_id, at DESC);

-- +goose Down
DROP INDEX microbot_audit_account_at_idx;
ALTER TABLE microbot_audit DROP COLUMN agent_id, DROP COLUMN account_id;
DROP TABLE workers;
DROP TABLE game_sessions;
ALTER TABLE accounts DROP COLUMN managed;
