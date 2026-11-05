-- +goose Up
-- Tokenless plugin ingest in AUTH_MODE=dev (docs/01 decision 28, docs/04
-- section 2.5): a request from loopback carries no token, only the plugin's
-- installation id (a random UUID generated once by the plugin). The protocol
-- state of that installation (last_seq, manifest_version, account) still needs
-- a row, so plugin_ingest_tokens starts to hold installation records next to
-- the ipl_ tokens, one per (account, installation_id), as the planned
-- installation registry of item 15 does. A row is either a token (token_hash)
-- or a dev installation (installation_id without token_hash); the row lock and
-- the client_seq/409 semantics are the same for both.

ALTER TABLE plugin_ingest_tokens
  ALTER COLUMN token_hash DROP NOT NULL,
  ADD COLUMN installation_id TEXT;

ALTER TABLE plugin_ingest_tokens
  ADD CONSTRAINT plugin_ingest_tokens_credential
  CHECK (token_hash IS NOT NULL OR installation_id IS NOT NULL);

-- One dev installation record per account; tokens keep their own unique hash.
CREATE UNIQUE INDEX plugin_ingest_tokens_installation_idx
  ON plugin_ingest_tokens (account_id, installation_id)
  WHERE token_hash IS NULL;

COMMENT ON COLUMN plugin_ingest_tokens.installation_id IS
  'Plugin installation id (random UUID kept in the plugin config). Without token_hash the row is a tokenless AUTH_MODE=dev installation.';
COMMENT ON TABLE plugin_ingest_tokens IS
  'Plugin credentials and their protocol state: ipl_ bearer tokens and tokenless dev installations (docs/04 section 2.5, docs/06).';

-- +goose Down
-- Installation records have no token to fall back to; their snapshots keep
-- their rows (ingest_token_id becomes NULL).
DELETE FROM plugin_ingest_tokens WHERE token_hash IS NULL;

COMMENT ON TABLE plugin_ingest_tokens IS 'Bearer tokens used by the Microbot plugin to push snapshots (docs/06).';
DROP INDEX IF EXISTS plugin_ingest_tokens_installation_idx;
ALTER TABLE plugin_ingest_tokens DROP CONSTRAINT IF EXISTS plugin_ingest_tokens_credential;
ALTER TABLE plugin_ingest_tokens
  DROP COLUMN installation_id,
  ALTER COLUMN token_hash SET NOT NULL;
