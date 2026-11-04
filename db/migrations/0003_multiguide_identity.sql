-- +goose Up
-- Guide-local section and step ids may be reused by independent routes. Make
-- guide_id part of every persisted content/progress identity before loading a
-- second guide. Legacy rows predate multi-guide support and belong to Oziris.

DROP VIEW IF EXISTS v_step_board;

ALTER TABLE content_steps
  DROP CONSTRAINT content_steps_pkey,
  ADD PRIMARY KEY (guide_id, step_id);

ALTER TABLE step_id_aliases
  ADD COLUMN guide_id TEXT NOT NULL DEFAULT 'oziris-main';
ALTER TABLE step_id_aliases
  DROP CONSTRAINT step_id_aliases_pkey,
  ADD PRIMARY KEY (guide_id, old_step_id),
  ALTER COLUMN guide_id DROP DEFAULT;
CREATE INDEX step_id_aliases_guide_new_idx
  ON step_id_aliases (guide_id, new_step_id);

ALTER TABLE step_progress
  ADD COLUMN guide_id TEXT NOT NULL DEFAULT 'oziris-main';
ALTER TABLE step_progress
  DROP CONSTRAINT step_progress_pkey,
  ADD PRIMARY KEY (account_id, guide_id, step_id),
  ALTER COLUMN guide_id DROP DEFAULT;
DROP INDEX step_progress_status_idx;
CREATE INDEX step_progress_status_idx
  ON step_progress (account_id, guide_id, status);
COMMENT ON TABLE step_progress IS 'Status per (account, guide, local step id); identical ids in other guides are independent.';

ALTER TABLE step_progress_events
  ADD COLUMN guide_id TEXT NOT NULL DEFAULT 'oziris-main';
ALTER TABLE step_progress_events
  ALTER COLUMN guide_id DROP DEFAULT;
DROP INDEX spe_account_time_idx;
CREATE INDEX spe_account_guide_time_idx
  ON step_progress_events (account_id, guide_id, created_at DESC);

CREATE VIEW v_step_board AS
  SELECT cs.guide_id, cs.section_id, cs.position, cs.step_id, cs.is_manual,
         sp.account_id,
         COALESCE(sp.status, 'pending') AS status,
         sp.completed_at, sp.manual_locked, sp.regressed_at
  FROM content_steps cs
  LEFT JOIN step_progress sp
    ON sp.guide_id = cs.guide_id AND sp.step_id = cs.step_id;

-- +goose Down
DROP VIEW IF EXISTS v_step_board;

DROP INDEX IF EXISTS spe_account_guide_time_idx;
ALTER TABLE step_progress_events DROP COLUMN guide_id;
CREATE INDEX spe_account_time_idx
  ON step_progress_events (account_id, created_at DESC);

DROP INDEX IF EXISTS step_progress_status_idx;
ALTER TABLE step_progress DROP CONSTRAINT step_progress_pkey;
ALTER TABLE step_progress DROP COLUMN guide_id;
ALTER TABLE step_progress ADD PRIMARY KEY (account_id, step_id);
CREATE INDEX step_progress_status_idx
  ON step_progress (account_id, status);
COMMENT ON TABLE step_progress IS 'Status of each guide step per account. auto_done always carries evidence.';

DROP INDEX IF EXISTS step_id_aliases_guide_new_idx;
ALTER TABLE step_id_aliases DROP CONSTRAINT step_id_aliases_pkey;
ALTER TABLE step_id_aliases DROP COLUMN guide_id;
ALTER TABLE step_id_aliases ADD PRIMARY KEY (old_step_id);

ALTER TABLE content_steps DROP CONSTRAINT content_steps_pkey;
ALTER TABLE content_steps ADD PRIMARY KEY (step_id);

CREATE VIEW v_step_board AS
  SELECT cs.guide_id, cs.section_id, cs.position, cs.step_id, cs.is_manual,
         sp.account_id,
         COALESCE(sp.status, 'pending') AS status,
         sp.completed_at, sp.manual_locked, sp.regressed_at
  FROM content_steps cs
  LEFT JOIN step_progress sp ON sp.step_id = cs.step_id;
