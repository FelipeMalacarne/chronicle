-- +goose Up
-- Recurring routines (docs/08 phase 4, delivery 4; docs/11 section 16): a
-- run the agent starts by itself when a Time Tracking clock is ready (the
-- herb patches, the trees, the bird houses), or a fixed interval after its
-- last run ended when the clock does not know the patches, the highest
-- priority first. They live next to the triggers in routines and go to the
-- agent in routines_sync; the executor stays in the agent (docs/08, R1).
--
-- A trigger is keyed by its event (one per event and account); a recurring
-- routine by its id, with a name unique in the account. Written only by
-- internal/store/routines.

ALTER TABLE routines DROP CONSTRAINT routines_kind_check;
-- a recurring routine waits for no event
ALTER TABLE routines ALTER COLUMN event DROP NOT NULL;
ALTER TABLE routines
  -- what the owner calls it ("Herb run"); recurring only
  ADD COLUMN name TEXT CHECK (char_length(name) BETWEEN 1 AND 80),
  -- the Time Tracking clock it follows (agent.Clocks: HERB, TREE,
  -- BIRD_HOUSE, ...); recurring only
  ADD COLUMN clock TEXT CHECK (clock ~ '^[A-Z_]{1,32}$'),
  -- minutes since its last run ended, when the clock does not know the
  -- patches (or it has no clock); recurring only
  ADD COLUMN interval_min INTEGER CHECK (interval_min BETWEEN 10 AND 10080),
  -- when several are due, the highest runs first
  ADD COLUMN priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100);
ALTER TABLE routines ADD CONSTRAINT routines_kind_check CHECK (
  (kind = 'trigger' AND event IS NOT NULL AND name IS NULL AND clock IS NULL AND interval_min IS NULL)
  OR (kind = 'recurring' AND event IS NULL AND name IS NOT NULL
      AND (clock IS NOT NULL OR interval_min IS NOT NULL))
);
CREATE UNIQUE INDEX routines_recurring_name_idx ON routines (account_id, name) WHERE kind = 'recurring';
COMMENT ON TABLE routines IS 'What a worker runs by itself (docs/11 section 4.4): triggers and recurring routines; the agent gets them on every connection.';

-- The last run of each routine (the interval counts from its end).
CREATE INDEX routine_runs_routine_started_idx ON routine_runs (routine_id, started_at DESC);

-- +goose Down
DROP INDEX routine_runs_routine_started_idx;
DROP INDEX routines_recurring_name_idx;
DELETE FROM routines WHERE kind = 'recurring';
ALTER TABLE routines DROP CONSTRAINT routines_kind_check;
ALTER TABLE routines DROP COLUMN name, DROP COLUMN clock, DROP COLUMN interval_min, DROP COLUMN priority;
ALTER TABLE routines ALTER COLUMN event SET NOT NULL;
ALTER TABLE routines ADD CONSTRAINT routines_kind_check CHECK (kind IN ('trigger'));
COMMENT ON TABLE routines IS 'What a worker runs by itself (docs/11 section 4.4): today the triggers; the agent gets them on every connection.';
