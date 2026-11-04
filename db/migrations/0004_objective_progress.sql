-- +goose Up
-- Progress over objectives (docs/09 section 5). The verdict of an objective is
-- shared by every guide; marks, derived-status history and the cursor stay per
-- guide. The derived step status is never stored: internal/core/progress
-- computes it from the step's objectives, their verdicts and the guide's marks.
-- step_progress, step_progress_events and v_step_board are replaced: the
-- owner's marks and the history are copied first.

CREATE TABLE objective_progress (
  account_id           TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  objective_key        TEXT NOT NULL,          -- canonical key (entities.ObjectiveKey), e.g. skill:agility>=70
  verdict              TEXT NOT NULL CHECK (verdict IN ('met','unmet','unknown')),
  evidence_snapshot_id TEXT REFERENCES snapshots(id), -- snapshot of the newest fact behind a met verdict
  evidence             JSONB,                  -- [{rule, verdict, observed, reason, facts:[{facet, key, source, snapshot_id, observed_at}]}]
  first_met_at         TIMESTAMPTZ,
  met_in_guide_id      TEXT,                   -- active guide when it became met; gates the implied cascade
  regressed_at         TIMESTAMPTZ,            -- the facts stopped holding after met; the verdict stays met
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, objective_key),
  CONSTRAINT objective_progress_met_has_evidence CHECK (
    verdict <> 'met' OR (
      evidence_snapshot_id IS NOT NULL AND evidence IS NOT NULL
      AND jsonb_typeof(evidence) = 'array' AND first_met_at IS NOT NULL
    )
  )
);
COMMENT ON TABLE objective_progress IS 'Verdict per account and objective, shared by every guide. A met verdict is latched with the evidence of the snapshot that proved it.';

CREATE TABLE objective_progress_events (
  id            TEXT PRIMARY KEY,
  account_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  objective_key TEXT NOT NULL,
  from_verdict  TEXT CHECK (from_verdict IN ('met','unmet','unknown')),
  to_verdict    TEXT NOT NULL CHECK (to_verdict IN ('met','unmet','unknown')),
  snapshot_id   TEXT,
  at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX objective_progress_events_account_time_idx
  ON objective_progress_events (account_id, at DESC);
COMMENT ON TABLE objective_progress_events IS 'Verdict transitions per objective (F6 for objectives).';

CREATE TABLE step_marks (
  account_id         TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  guide_id           TEXT NOT NULL,
  step_id            TEXT NOT NULL,            -- no FK: a mark survives steps removed from content
  kind               TEXT NOT NULL CHECK (kind IN ('manual_done','skipped','pinned_open','implied')),
  implied_by_step_id TEXT,                     -- step whose completion produced the implied mark
  implied_at         TIMESTAMPTZ,
  note               TEXT,
  marked_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, guide_id, step_id),
  CONSTRAINT step_marks_implied_shape CHECK (
    (kind = 'implied') = (implied_by_step_id IS NOT NULL AND implied_at IS NOT NULL)
  )
);
COMMENT ON TABLE step_marks IS 'The owner''s mark per guide; the engine never overrides it. Only the implied cascade writes kind implied.';

CREATE TABLE step_status_events (
  id          TEXT PRIMARY KEY,
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  guide_id    TEXT NOT NULL,
  step_id     TEXT NOT NULL,
  from_status TEXT CHECK (from_status IN ('pending','auto_done','manual_done','skipped','pinned_open','implied')),
  to_status   TEXT NOT NULL CHECK (to_status IN ('pending','auto_done','manual_done','skipped','pinned_open','implied')),
  cause       TEXT NOT NULL CHECK (cause IN ('engine','user','implied','migration','import')),
  snapshot_id TEXT,
  at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX step_status_events_step_time_idx
  ON step_status_events (account_id, guide_id, step_id, at DESC);
CREATE INDEX step_status_events_account_time_idx
  ON step_status_events (account_id, at DESC);
COMMENT ON TABLE step_status_events IS 'Derived step status transitions for every guide (docs/09 decision D2). The newest row per step is the last known status.';

CREATE TABLE route_cursor (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  guide_id   TEXT NOT NULL,
  step_id    TEXT NOT NULL,
  cause      TEXT NOT NULL CHECK (cause IN ('pin','auto')),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, guide_id)
);
COMMENT ON TABLE route_cursor IS 'Current step per guide; switching guides keeps the place. accounts.guide_id stays the active guide.';

INSERT INTO step_marks (account_id, guide_id, step_id, kind, note, marked_at)
SELECT account_id, guide_id, step_id,
       CASE WHEN status IN ('manual_done','skipped') THEN status ELSE 'pinned_open' END,
       user_note, COALESCE(manual_at, updated_at)
FROM step_progress
WHERE status IN ('manual_done','skipped') OR (manual_locked AND status = 'pending');

INSERT INTO step_status_events (id, account_id, guide_id, step_id, from_status, to_status, cause, snapshot_id, at)
SELECT id, account_id, guide_id, step_id, from_status, to_status, cause, snapshot_id, created_at
FROM step_progress_events;

DROP VIEW IF EXISTS v_step_board;
DROP TABLE step_progress_events;
DROP TABLE step_progress;

-- content_steps is a minimal index rewritten at every boot: drop its rows
-- instead of inventing a text hash for them.
DELETE FROM content_steps;
ALTER TABLE content_steps
  DROP COLUMN rule_hash,
  DROP COLUMN is_manual,
  DROP COLUMN layers,
  DROP COLUMN modes,
  ADD COLUMN text_hash TEXT NOT NULL;
COMMENT ON COLUMN content_steps.position IS 'order of the step in the guide';
COMMENT ON COLUMN content_steps.text_hash IS 'content.StepTextHash; pins the step''s annotation';
COMMENT ON TABLE content_steps IS 'Minimal index of the loaded steps for event joins and orphan detection; written at boot.';

ALTER TABLE content_versions
  DROP COLUMN guide_version,
  DROP COLUMN step_count,
  ADD COLUMN summary JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE content_versions ALTER COLUMN summary DROP DEFAULT;
COMMENT ON COLUMN content_versions.summary IS '{guide_id: {version, steps, objectives, annotated_steps}}';

-- goals has no writer yet; F9 validates the key when it starts writing.
ALTER TABLE goals RENAME COLUMN ref TO objective_key;
COMMENT ON COLUMN goals.objective_key IS 'canonical objective key (entities.ObjectiveKey), e.g. quest:dragon_slayer_ii@finished';

DROP INDEX player_state_value_gin_idx;
ALTER INDEX pit_account_idx RENAME TO plugin_ingest_tokens_account_idx;

-- +goose Down
ALTER INDEX plugin_ingest_tokens_account_idx RENAME TO pit_account_idx;
CREATE INDEX player_state_value_gin_idx ON player_state USING gin (value jsonb_path_ops);

COMMENT ON COLUMN goals.objective_key IS NULL;
ALTER TABLE goals RENAME COLUMN objective_key TO ref;

ALTER TABLE content_versions
  DROP COLUMN summary,
  ADD COLUMN guide_version TEXT,
  ADD COLUMN step_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE content_versions ALTER COLUMN step_count DROP DEFAULT;

DELETE FROM content_steps;
ALTER TABLE content_steps
  DROP COLUMN text_hash,
  ADD COLUMN rule_hash TEXT NOT NULL,
  ADD COLUMN is_manual BOOLEAN NOT NULL,
  ADD COLUMN layers JSONB NOT NULL,
  ADD COLUMN modes JSONB NOT NULL;
COMMENT ON COLUMN content_steps.position IS NULL;
COMMENT ON TABLE content_steps IS NULL;

CREATE TABLE step_progress (
  account_id           TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  step_id              TEXT NOT NULL,
  status               TEXT NOT NULL CHECK (status IN ('pending','auto_done','manual_done','skipped')),
  completed_at         TIMESTAMPTZ,
  evidence_snapshot_id TEXT REFERENCES snapshots(id),
  evidence             JSONB,
  rule_hash            TEXT,
  manual_locked        BOOLEAN NOT NULL DEFAULT false,
  manual_at            TIMESTAMPTZ,
  regressed_at         TIMESTAMPTZ,
  user_note            TEXT,
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  guide_id             TEXT NOT NULL,
  PRIMARY KEY (account_id, guide_id, step_id)
);
CREATE INDEX step_progress_status_idx ON step_progress (account_id, guide_id, status);
COMMENT ON TABLE step_progress IS 'Status per (account, guide, local step id); identical ids in other guides are independent.';

CREATE TABLE step_progress_events (
  id          TEXT PRIMARY KEY,
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  step_id     TEXT NOT NULL,
  from_status TEXT,
  to_status   TEXT NOT NULL,
  cause       TEXT NOT NULL CHECK (cause IN ('engine','user','migration','import')),
  snapshot_id TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  guide_id    TEXT NOT NULL
);
CREATE INDEX spe_account_guide_time_idx
  ON step_progress_events (account_id, guide_id, created_at DESC);

INSERT INTO step_progress (account_id, guide_id, step_id, status, manual_locked, manual_at, user_note, completed_at, updated_at)
SELECT account_id, guide_id, step_id,
       CASE WHEN kind IN ('manual_done','skipped') THEN kind ELSE 'pending' END,
       true, marked_at, note,
       CASE WHEN kind = 'manual_done' THEN marked_at END, marked_at
FROM step_marks
WHERE kind IN ('manual_done','skipped','pinned_open');

INSERT INTO step_progress_events (id, account_id, guide_id, step_id, from_status, to_status, cause, snapshot_id, created_at)
SELECT id, account_id, guide_id, step_id, from_status, to_status, cause, snapshot_id, at
FROM step_status_events
WHERE to_status IN ('pending','auto_done','manual_done','skipped')
  AND (from_status IS NULL OR from_status IN ('pending','auto_done','manual_done','skipped'))
  AND cause IN ('engine','user','migration','import');

CREATE VIEW v_step_board AS
  SELECT cs.guide_id, cs.section_id, cs.position, cs.step_id, cs.is_manual,
         sp.account_id,
         COALESCE(sp.status, 'pending') AS status,
         sp.completed_at, sp.manual_locked, sp.regressed_at
  FROM content_steps cs
  LEFT JOIN step_progress sp
    ON sp.guide_id = cs.guide_id AND sp.step_id = cs.step_id;

DROP TABLE route_cursor;
DROP TABLE step_status_events;
DROP TABLE step_marks;
DROP TABLE objective_progress_events;
DROP TABLE objective_progress;
