-- +goose Up
-- The proven Grand Exchange sales (docs/12 section 6.2): a ge_sell's
-- termination is relative (the coins the inventory held when the sale was
-- resolved plus the least it pays), so the ingest proves it only while the
-- run's coins are still there. The server records that proof the moment it
-- sees it (GET /api/microbot/task: the run ended done, the live inventory
-- has none of the item and at least coins_at_least coins), and the plan
-- counts the sale met from then on, while the inventory does not hold the
-- item again. One row per (account, plan, task key): the latest proof. The
-- plan id is not a foreign key: a route plan has no row in plans.
-- Written only by internal/store/plans.
CREATE TABLE sale_proofs (
  account_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  plan_id        TEXT NOT NULL CHECK (length(plan_id) BETWEEN 1 AND 200),
  task_key       TEXT NOT NULL CHECK (length(task_key) BETWEEN 1 AND 300),
  -- the run whose sale was proven (agent.Run.UID)
  run_uid        TEXT NOT NULL CHECK (length(run_uid) BETWEEN 1 AND 64),
  item_id        INTEGER NOT NULL CHECK (item_id > 0),
  coins_at_least BIGINT NOT NULL CHECK (coins_at_least > 0),
  -- the coins the live inventory held when the proof was seen
  coins_seen     BIGINT NOT NULL CHECK (coins_seen >= coins_at_least),
  proven_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, plan_id, task_key)
);
COMMENT ON TABLE sale_proofs IS 'Grand Exchange sales the ingest proved (the item left the live inventory and the run''s coins reached it), by plan and task key: the plan counts them met afterwards.';

-- +goose Down
DROP TABLE sale_proofs;
