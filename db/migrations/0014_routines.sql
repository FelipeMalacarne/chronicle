-- +goose Up
-- Routines and runs on the server (docs/11 section 4.4, phase F6): the
-- triggers and the runs' history leave the agent's disk, so a worker's pod,
-- whose disk goes with it, keeps its triggers and its history. The executor
-- stays in the agent (docs/08, R1): the server sends the account's triggers
-- on every connection (routines_sync), and the agent sends every run as it
-- changes (TypeEvent), which the server applies idempotently.
--
-- Written only by internal/store/routines. A run is keyed by the agent's
-- uid for it; a snapshot older than the stored one (a shorter trail) is
-- ignored, and a run never moves to another account.

CREATE TABLE routines (
  id         TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  -- 'trigger': a game event starts the run; recurring routines come later
  kind       TEXT NOT NULL CHECK (kind IN ('trigger')),
  -- the game event a trigger waits for (agent.Events)
  event      TEXT NOT NULL,
  enabled    BOOLEAN NOT NULL DEFAULT false,
  -- the run it starts: agent.RunParams
  spec       JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (account_id, kind, event)
);
COMMENT ON TABLE routines IS 'What a worker runs by itself (docs/11 section 4.4): today the triggers; the agent gets them on every connection.';

CREATE TABLE routine_runs (
  -- the agent's uid for the run (its "run-N" starts again in a new pod)
  id           TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{8,64}$'),
  account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  -- the trigger that started it, when one did and it is still there
  routine_id   TEXT REFERENCES routines(id) ON DELETE SET NULL,
  -- where it ran: the worker's pod or the attached agent's host
  agent        TEXT NOT NULL,
  agent_run_id TEXT NOT NULL,
  class_name   TEXT NOT NULL,
  stop         JSONB NOT NULL DEFAULT '[]',
  -- the game event that started it; '' when the owner did
  trigger      TEXT NOT NULL DEFAULT '',
  state        TEXT NOT NULL,
  started_at   TIMESTAMPTZ NOT NULL,
  finished_at  TIMESTAMPTZ,
  -- where the character stood when it began, and the run plugin's result
  from_pos     JSONB,
  result       JSONB,
  error        TEXT NOT NULL DEFAULT '',
  -- how many lines of the trail the stored state has: an older snapshot
  -- (fewer lines) never overwrites a newer one
  trail_len    INTEGER NOT NULL DEFAULT 0 CHECK (trail_len >= 0),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX routine_runs_account_started_idx ON routine_runs (account_id, started_at DESC);

-- The trail of a run, one row per line, in order.
CREATE TABLE run_events (
  run_id TEXT NOT NULL REFERENCES routine_runs(id) ON DELETE CASCADE,
  seq    INTEGER NOT NULL CHECK (seq >= 0),
  at     TIMESTAMPTZ NOT NULL,
  state  TEXT NOT NULL,
  text   TEXT NOT NULL,
  PRIMARY KEY (run_id, seq)
);

-- +goose Down
DROP TABLE run_events;
DROP TABLE routine_runs;
DROP TABLE routines;
