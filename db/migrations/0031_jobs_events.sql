-- +goose Up
-- Jobs and durable events (docs/12 section 13). A job is the intent (get to
-- varp 281 >= 1000, Mining 15, sell the glass) with a state and a measured
-- progress; each attempt of the agent is a run (routine_runs.job_id). The
-- server's pieces react to the events table instead of polling each other:
-- every event is written in the transaction of the change that causes it, and
-- each consumer keeps its cursor. Written by internal/store/jobs,
-- internal/store/eventlog and the writers that call it.
CREATE TABLE jobs (
  id          TEXT PRIMARY KEY CHECK (id ~ '^job_[0-9a-f]{16}$'),
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK (kind IN ('plugin', 'train', 'quest', 'travel', 'bank', 'buy', 'ge_buy', 'ge_sell')),
  -- the typed task (internal/core/execspec) the agent executes; {} for a job
  -- adopted from a run that already started
  spec        JSONB NOT NULL DEFAULT '{}',
  -- what proves the end, evaluated by the rule engine on the account's facts:
  -- an objective key (a JSON string), a rule object or {"task": ...}
  goal        JSONB NOT NULL,
  -- {kind, id|skill, target}: the number that measures the progress; NULL for
  -- a job whose progress no fact measures
  metric      JSONB,
  -- {from, current, target, at}
  progress    JSONB,
  stall_after_s INTEGER NOT NULL CHECK (stall_after_s BETWEEN 30 AND 86400),
  state       TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'stalled', 'succeeded', 'failed', 'canceled')),
  reason      TEXT NOT NULL DEFAULT '',
  -- who created it: the conductor, the owner, a routine's run, a test
  source      TEXT NOT NULL CHECK (source IN ('conductor', 'owner', 'routine', 'test')),
  -- the plan task a job executes (a ge_sell's proof is read by them)
  plan_id     TEXT,
  task_key    TEXT,
  -- when a running job with a metric stalls: the last change plus stall_after
  deadline_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at  TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  CHECK ((state IN ('succeeded', 'failed', 'canceled')) = (finished_at IS NOT NULL)),
  CHECK (deadline_at IS NULL OR state = 'running')
);
CREATE INDEX jobs_account_created_idx ON jobs (account_id, created_at DESC);
CREATE INDEX jobs_active_idx ON jobs (account_id) WHERE state IN ('queued', 'running', 'stalled');
CREATE INDEX jobs_deadline_idx ON jobs (deadline_at) WHERE deadline_at IS NOT NULL;
CREATE INDEX jobs_plan_task_idx ON jobs (account_id, plan_id, task_key, created_at DESC) WHERE task_key IS NOT NULL;
COMMENT ON TABLE jobs IS 'What the account is made to do (docs/12 section 13): an intent with a state and a progress measured from the facts; each attempt is a run.';

ALTER TABLE routine_runs ADD COLUMN job_id TEXT REFERENCES jobs(id) ON DELETE SET NULL;
CREATE INDEX routine_runs_job_idx ON routine_runs (job_id) WHERE job_id IS NOT NULL;
COMMENT ON COLUMN routine_runs.job_id IS 'The job this run attempts; NULL for the runs the owner starts by hand without an objective.';

CREATE TABLE events (
  id         BIGSERIAL PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  -- fact.changed, run.started, run.ended, job.*, membership.low
  type       TEXT NOT NULL CHECK (type ~ '^[a-z]+(\.[a-z]+)+$'),
  -- the job or run the event is about; '' for the account's facts
  subject    TEXT NOT NULL DEFAULT '',
  -- the job the event is about (a job's own events and its runs'), for the
  -- job's detail; NULL for the rest
  job_id     TEXT,
  payload    JSONB NOT NULL DEFAULT '{}',
  at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX events_account_idx ON events (account_id, id);
CREATE INDEX events_job_idx ON events (job_id, id) WHERE job_id IS NOT NULL;
CREATE INDEX events_fact_changed_at_idx ON events (at) WHERE type = 'fact.changed';
COMMENT ON TABLE events IS 'Durable, append-only log the server''s consumers react to (docs/12 section 13.4); an event says "changed", the consumer reads the state. Ids follow the inserts, not the commits: writers hold the shared advisory lock 7123001 from the insert to the commit and a reader takes it exclusively, so it never reads past an event still uncommitted.';

-- +goose StatementBegin
CREATE FUNCTION events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'events is append-only'
    USING ERRCODE = 'integrity_constraint_violation';
END
$$;
-- +goose StatementEnd
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
  FOR EACH ROW EXECUTE FUNCTION events_append_only();

CREATE TABLE event_cursors (
  consumer   TEXT PRIMARY KEY CHECK (length(consumer) BETWEEN 1 AND 64),
  last_id    BIGINT NOT NULL DEFAULT 0 CHECK (last_id >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMENT ON TABLE event_cursors IS 'The last event each consumer handled; it resumes from there after a restart.';

-- The sale proofs (0030) are the job tracker's now: a proven sale is a ge_sell
-- job that succeeded. A proof still in the table becomes one, with the goal
-- internal/core/job.SaleGoal gives (the item left the inventory, the coins
-- are in it).
INSERT INTO jobs (id, account_id, kind, goal, stall_after_s, state, reason, source, plan_id, task_key, created_at, started_at, finished_at)
SELECT 'job_' || substr(md5(account_id || plan_id || task_key || run_uid), 1, 16), account_id, 'ge_sell',
       jsonb_build_object('all', jsonb_build_array(
         jsonb_build_object('not', jsonb_build_object('type', 'item_owned', 'item_id', item_id, 'qty', 1, 'containers', jsonb_build_array('inventory'))),
         jsonb_build_object('type', 'item_owned', 'item_id', 995, 'qty', coins_at_least, 'containers', jsonb_build_array('inventory')))),
       900, 'succeeded', 'sale proven before jobs existed', 'owner', plan_id, task_key,
       proven_at, proven_at, proven_at
FROM sale_proofs;
DROP TABLE sale_proofs;

-- +goose Down
-- Jobs and events are real data (a job is what the account was made to do, the
-- events its history): refuse instead of losing them.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM jobs) OR EXISTS (SELECT 1 FROM events) THEN
    RAISE EXCEPTION 'jobs or events exist; delete them (DELETE FROM jobs; DELETE FROM events) before rolling back 0031';
  END IF;
END
$$;
-- +goose StatementEnd
CREATE TABLE sale_proofs (
  account_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  plan_id        TEXT NOT NULL CHECK (length(plan_id) BETWEEN 1 AND 200),
  task_key       TEXT NOT NULL CHECK (length(task_key) BETWEEN 1 AND 300),
  run_uid        TEXT NOT NULL CHECK (length(run_uid) BETWEEN 1 AND 64),
  item_id        INTEGER NOT NULL CHECK (item_id > 0),
  coins_at_least BIGINT NOT NULL CHECK (coins_at_least > 0),
  coins_seen     BIGINT NOT NULL CHECK (coins_seen >= coins_at_least),
  proven_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, plan_id, task_key)
);
COMMENT ON TABLE sale_proofs IS 'Grand Exchange sales the ingest proved (the item left the live inventory and the run''s coins reached it), by plan and task key: the plan counts them met afterwards.';
DROP TRIGGER events_no_update ON events;
DROP FUNCTION events_append_only();
DROP TABLE event_cursors;
DROP TABLE events;
DROP INDEX routine_runs_job_idx;
ALTER TABLE routine_runs DROP COLUMN job_id;
DROP TABLE jobs;
