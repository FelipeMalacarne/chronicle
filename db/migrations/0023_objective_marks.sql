-- +goose Up
-- The owner's marks on objectives (docs/09 section 5, owner decision of
-- 2026-11-13): "this objective is already satisfied", for the whole account,
-- in every guide and step. It is an input that overlays the verdicts when step
-- statuses and cursors are derived, never evidence: nothing is written to
-- objective_progress (its trigger and CHECKs require evidence for met), and it
-- never feeds the implied cascade.
CREATE TABLE objective_marks (
  account_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  objective_key TEXT NOT NULL,                   -- canonical key (entities.ObjectiveKey)
  note          TEXT NOT NULL DEFAULT '',
  marked_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, objective_key),
  CONSTRAINT objective_marks_key_shape CHECK (objective_key ~ '^(qp>=[0-9]+|[a-z_]+:.+)$')
);
COMMENT ON TABLE objective_marks IS 'The owner''s marks on objectives, account-wide: they count as met when statuses and cursors are derived, but are not evidence (objective_progress is untouched).';

-- +goose Down
DROP TABLE objective_marks;
