-- +goose Up
-- Ingest contract v1: effective token scopes and explicit item-container
-- provenance. A restored cache deliberately has no live_epoch, so it cannot be
-- confused with an ItemContainerChanged observation from the current login.

ALTER TABLE plugin_ingest_tokens
  ALTER COLUMN scopes SET DEFAULT '["state:write"]'::jsonb;

UPDATE plugin_ingest_tokens t
SET scopes = (
  SELECT COALESCE(jsonb_agg(mapped.scope ORDER BY mapped.scope), '["state:write"]'::jsonb)
  FROM (
    SELECT DISTINCT CASE
      WHEN old_scope IN ('vars', 'skills', 'collection_log') THEN 'state:write'
      WHEN old_scope IN ('bank', 'inventory', 'equipment') THEN 'items:' || old_scope
      ELSE old_scope
    END AS scope
    FROM jsonb_array_elements_text(t.scopes) AS old(old_scope)
  ) AS mapped
);

ALTER TABLE plugin_ingest_tokens
  ADD CONSTRAINT plugin_ingest_tokens_scopes_array
  CHECK (jsonb_typeof(scopes) = 'array');

ALTER TABLE snapshots
  ADD COLUMN ingest_token_id TEXT REFERENCES plugin_ingest_tokens(id) ON DELETE SET NULL;

-- Plugin idempotency is sequence-based. The old account/source/raw_hash
-- constraint could reject a legitimate identical payload from a second token.
ALTER TABLE snapshots
  DROP CONSTRAINT snapshots_account_id_source_raw_hash_key;
CREATE UNIQUE INDEX snapshots_nonplugin_raw_hash_idx
  ON snapshots (account_id, source, raw_hash)
  WHERE source <> 'plugin';

-- client_seq belongs to one client token, not to the account: an account may
-- legitimately have desktop and laptop installs whose counters both start at 1.
CREATE UNIQUE INDEX snapshots_plugin_client_seq_idx
  ON snapshots (ingest_token_id, client_seq)
  WHERE source = 'plugin' AND ingest_token_id IS NOT NULL AND client_seq IS NOT NULL;

ALTER TABLE snapshot_items
  ADD CONSTRAINT snapshot_items_qty_nonnegative CHECK (qty >= 0);

CREATE TABLE snapshot_item_containers (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  container   TEXT NOT NULL CHECK (container IN ('bank','inventory','equipment')),
  state       TEXT NOT NULL CHECK (state IN ('unavailable','restored','live')),
  observed_at TIMESTAMPTZ NOT NULL,
  revision    BIGINT,
  live_epoch  TEXT,
  PRIMARY KEY (snapshot_id, container),
  CONSTRAINT snapshot_item_containers_freshness CHECK (
    (state = 'unavailable' AND revision IS NULL AND live_epoch IS NULL) OR
    (state = 'restored' AND revision IS NOT NULL AND revision >= 0 AND live_epoch IS NULL) OR
    (state = 'live' AND revision IS NOT NULL AND revision >= 0 AND length(live_epoch) > 0)
  )
);
COMMENT ON TABLE snapshot_item_containers IS 'Immutable per-snapshot availability and provenance for item containers.';

CREATE TABLE ingest_container_freshness (
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  container   TEXT NOT NULL CHECK (container IN ('bank','inventory','equipment')),
  state       TEXT NOT NULL CHECK (state IN ('unavailable','restored','live')),
  observed_at TIMESTAMPTZ NOT NULL,
  revision    BIGINT,
  live_epoch  TEXT,
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, container),
  CONSTRAINT ingest_container_freshness_shape CHECK (
    (state = 'unavailable' AND revision IS NULL AND live_epoch IS NULL) OR
    (state = 'restored' AND revision IS NOT NULL AND revision >= 0 AND live_epoch IS NULL) OR
    (state = 'live' AND revision IS NOT NULL AND revision >= 0 AND length(live_epoch) > 0)
  )
);
COMMENT ON TABLE ingest_container_freshness IS 'Latest accepted item-container observation; restored is never promoted to live.';

-- +goose Down
DROP TABLE IF EXISTS ingest_container_freshness;
DROP TABLE IF EXISTS snapshot_item_containers;
ALTER TABLE snapshot_items DROP CONSTRAINT IF EXISTS snapshot_items_qty_nonnegative;
DROP INDEX IF EXISTS snapshots_plugin_client_seq_idx;
DROP INDEX IF EXISTS snapshots_nonplugin_raw_hash_idx;
ALTER TABLE snapshots
  ADD CONSTRAINT snapshots_account_id_source_raw_hash_key UNIQUE (account_id, source, raw_hash);
ALTER TABLE snapshots DROP COLUMN IF EXISTS ingest_token_id;
ALTER TABLE plugin_ingest_tokens DROP CONSTRAINT IF EXISTS plugin_ingest_tokens_scopes_array;
ALTER TABLE plugin_ingest_tokens
  ALTER COLUMN scopes SET DEFAULT '["vars","skills","collection_log"]'::jsonb;
