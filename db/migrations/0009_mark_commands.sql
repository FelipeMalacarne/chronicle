-- +goose Up
-- Commands as a log (docs/10 section 6, decision E2), phase 0 slice d.
--
-- guides: every guide the server ever loaded, upserted at boot by the content
-- index; progress rows reference it, so a mistyped guide id is refused.
--
-- mark_commands: append-only log of every change to step_marks: the owner's
-- commands (from the web or the plugin panel), the implied marks the engine
-- writes and the moves of step id aliases. Each command stores its effects
-- (the mark of each step before and after), so replaying the log in seq order
-- gives step_marks back: step_marks is its projection, and a mismatch is a bug
-- that `ironman progress rebuild --check` reports.

CREATE TABLE guides (
  id             TEXT PRIMARY KEY,
  title          TEXT NOT NULL DEFAULT '',
  first_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_loaded_at TIMESTAMPTZ
);
COMMENT ON TABLE guides IS 'Every guide the server loaded (upserted at boot); progress rows reference it.';

INSERT INTO guides (id)
SELECT guide_id FROM accounts
UNION SELECT guide_id FROM step_marks
UNION SELECT guide_id FROM route_cursor
UNION SELECT guide_id FROM step_status_events
UNION SELECT guide_id FROM content_steps
UNION SELECT guide_id FROM step_id_aliases
UNION SELECT unnest(ARRAY['oziris-main', 'bruhsailer-main'])
ON CONFLICT DO NOTHING;

ALTER TABLE accounts ADD CONSTRAINT accounts_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);
ALTER TABLE step_marks ADD CONSTRAINT step_marks_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);
ALTER TABLE route_cursor ADD CONSTRAINT route_cursor_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);
ALTER TABLE step_status_events ADD CONSTRAINT step_status_events_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);
ALTER TABLE content_steps ADD CONSTRAINT content_steps_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);
ALTER TABLE step_id_aliases ADD CONSTRAINT step_id_aliases_guide_fkey FOREIGN KEY (guide_id) REFERENCES guides (id);

CREATE TABLE mark_commands (
  seq        BIGSERIAL PRIMARY KEY,               -- replay order
  id         TEXT NOT NULL UNIQUE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  guide_id   TEXT NOT NULL REFERENCES guides(id),
  kind       TEXT NOT NULL CHECK (kind IN ('mark','confirm_implied','mark_before','restore','pin','implied','alias','migration')),
  actor      TEXT NOT NULL CHECK (actor IN ('owner','engine','system')),
  via        TEXT CHECK (via IN ('web','plugin_panel')), -- where an owner command came from
  step_id    TEXT,                                -- the step the command was about, when one
  revokes_id TEXT REFERENCES mark_commands(id),   -- the command an undo takes back
  effects    JSONB NOT NULL CHECK (jsonb_typeof(effects) = 'array'), -- [{step_id, from: mark|null, to: mark|null}]
  at         TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT mark_commands_owner_has_via CHECK ((actor = 'owner') = (via IS NOT NULL))
);
CREATE INDEX mark_commands_replay_idx ON mark_commands (account_id, guide_id, seq);
COMMENT ON TABLE mark_commands IS 'Append-only log of every step_marks change (owner, engine implied, alias moves) with its effects; step_marks is its projection.';

-- +goose StatementBegin
CREATE FUNCTION mark_commands_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'mark_commands is append-only'
    USING ERRCODE = 'check_violation';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER mark_commands_no_update BEFORE UPDATE ON mark_commands
  FOR EACH ROW EXECUTE FUNCTION mark_commands_append_only();

ALTER TABLE step_status_events ADD COLUMN command_id TEXT REFERENCES mark_commands(id);
COMMENT ON COLUMN step_status_events.command_id IS 'the mark command behind a user, implied or migration change';

-- Seed: the marks that exist now enter the log as one migration command per
-- account and guide, so the replay starts from them.
INSERT INTO mark_commands (id, account_id, guide_id, kind, actor, effects)
SELECT 'mc_seed_' || md5(account_id || '/' || guide_id), account_id, guide_id, 'migration', 'system',
       jsonb_agg(jsonb_build_object(
         'step_id', step_id,
         'from', NULL,
         'to', jsonb_strip_nulls(jsonb_build_object('kind', kind, 'implied_by_step_id', implied_by_step_id, 'note', note))
       ) ORDER BY step_id)
FROM step_marks
GROUP BY account_id, guide_id;

-- +goose Down
ALTER TABLE step_status_events DROP COLUMN command_id;
DROP TRIGGER mark_commands_no_update ON mark_commands;
DROP FUNCTION mark_commands_append_only();
DROP TABLE mark_commands;
ALTER TABLE step_id_aliases DROP CONSTRAINT step_id_aliases_guide_fkey;
ALTER TABLE content_steps DROP CONSTRAINT content_steps_guide_fkey;
ALTER TABLE step_status_events DROP CONSTRAINT step_status_events_guide_fkey;
ALTER TABLE route_cursor DROP CONSTRAINT route_cursor_guide_fkey;
ALTER TABLE step_marks DROP CONSTRAINT step_marks_guide_fkey;
ALTER TABLE accounts DROP CONSTRAINT accounts_guide_fkey;
DROP TABLE guides;
