-- +goose Up
-- Demonstration recording (docs/10 section 11, phase 2; decision E8): the
-- events the Chronicle Link plugin records while the owner plays with
-- "Record demonstration" on, sent in data.recording of the ingest pushes.
-- The ingest is the only writer; the owner reads them (GET
-- /api/account/recording) to turn a stretch into proposed tasks, and deletes
-- them on request (DELETE /api/account/recording). Each (recording, seq) is
-- stored once: a push resent after a timeout or a 409 inserts nothing twice.
-- The raw snapshot does not keep data.recording, so a delete removes the
-- recording for good.
CREATE TABLE recorded_events (
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  recording   TEXT NOT NULL CHECK (recording ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  seq         BIGINT NOT NULL CHECK (seq > 0),
  at          TIMESTAMPTZ NOT NULL,              -- when the plugin recorded it, bounded by the server clock
  kind        TEXT NOT NULL CHECK (kind IN ('move','npc','object','dialogue','shop_buy','bank_open','bank_withdraw','bank_deposit','item_use','item_op')),
  event       JSONB NOT NULL CHECK (jsonb_typeof(event) = 'object'), -- the validated event as sent
  snapshot_id TEXT NOT NULL,                     -- the push that carried it first
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, recording, seq),
  CONSTRAINT recorded_events_snapshot_fk FOREIGN KEY (snapshot_id, account_id)
    REFERENCES snapshots (id, account_id) ON DELETE CASCADE
);
CREATE INDEX recorded_events_account_time_idx ON recorded_events (account_id, at, recording, seq);
COMMENT ON TABLE recorded_events IS 'Demonstration recording of the plugin (docs/10 section 11): one row per recorded event, written only by the ingest.';

-- +goose Down
DROP TABLE recorded_events;
