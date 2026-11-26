-- +goose Up
-- The conductor's state per account (docs/12 section 12.5; FEL-126): what it
-- does for the account and the job it dispatched. An account without a row
-- is off. Written by internal/store/conductor: the owner turns it on or off,
-- internal/conduct moves it on the job events. The first slice has off,
-- idle, running and blocked; sessions, approvals and the escalation ladder
-- add their states later.
CREATE TABLE conductor_state (
  account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  state      TEXT NOT NULL CHECK (state IN ('off', 'idle', 'running', 'blocked')),
  -- the job the conductor dispatched: the active one while running, the last
  -- one after it ended
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  -- why it is blocked, or why it waits
  reason     TEXT NOT NULL DEFAULT '',
  since      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (state <> 'running' OR job_id IS NOT NULL)
);
COMMENT ON TABLE conductor_state IS 'The conductor''s state per account (docs/12 section 12.5): off, idle, running (its job) or blocked (and why). No row is off.';

-- +goose Down
DROP TABLE conductor_state;
