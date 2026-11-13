-- +goose Up
-- Plans (docs/10 section 9, phase 3): the ordered list an account follows.
-- A route plan is one guide, one to one: its items are the guide's steps
-- and its position the account's cursor, so it needs no row until it is
-- edited (a 'route' row only names its guide and holds no items). Goal and
-- custom plans store their items. An item references a step of a guide
-- (guide_id, step_id), a recipe (its objective key) or one standalone typed
-- task (content/schema/task.schema.json), never the guide's text (CLAUDE.md
-- rule 7). accounts.active_plan_id is the plan the account follows; NULL is
-- the route of accounts.guide_id.
--
-- Written only by internal/store/plans; a step id that a guide renames
-- moves with the step's other progress (step_id_aliases).

CREATE TABLE plans (
  id         TEXT PRIMARY KEY CHECK (id ~ '^pln_[a-z0-9]{8,32}$'),
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('route','goal','custom')),
  title      TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
  guide_id   TEXT REFERENCES guides(id),       -- route plans: the guide
  goal_id    TEXT,                             -- goal plans: the goal
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (id, account_id),
  CONSTRAINT plans_kind_refs CHECK (
    (kind = 'route' AND guide_id IS NOT NULL AND goal_id IS NULL) OR
    (kind = 'goal' AND goal_id IS NOT NULL AND guide_id IS NULL) OR
    (kind = 'custom' AND guide_id IS NULL AND goal_id IS NULL))
);
-- A goal plan belongs to the goal's own account.
CREATE UNIQUE INDEX goals_id_account_idx ON goals (id, account_id);
ALTER TABLE plans ADD CONSTRAINT plans_goal_fkey
  FOREIGN KEY (goal_id, account_id) REFERENCES goals (id, account_id) ON DELETE CASCADE;
-- One route plan per guide and account.
CREATE UNIQUE INDEX plans_route_idx ON plans (account_id, guide_id) WHERE kind = 'route';
CREATE INDEX plans_account_idx ON plans (account_id, created_at);
COMMENT ON TABLE plans IS 'Plans of an account (docs/10 section 9): route (the guide, one to one), goal and custom.';

CREATE TABLE plan_items (
  plan_id    TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL CHECK (position >= 0 AND position < 500),
  kind       TEXT NOT NULL CHECK (kind IN ('step','recipe','task')),
  guide_id   TEXT REFERENCES guides(id),       -- a step item: the guide
  step_id    TEXT,                             -- a step item: the step, by its guide-scoped id
  recipe_key TEXT,                             -- a recipe item: the objective key it fulfills
  task       JSONB,                            -- a task item: one typed task (refs, numbers, ids)
  PRIMARY KEY (plan_id, position),
  CONSTRAINT plan_items_kind_refs CHECK (
    (kind = 'step' AND guide_id IS NOT NULL AND step_id IS NOT NULL AND recipe_key IS NULL AND task IS NULL) OR
    (kind = 'recipe' AND recipe_key IS NOT NULL AND guide_id IS NULL AND step_id IS NULL AND task IS NULL) OR
    (kind = 'task' AND task IS NOT NULL AND jsonb_typeof(task) = 'object' AND guide_id IS NULL AND step_id IS NULL AND recipe_key IS NULL))
);
CREATE INDEX plan_items_step_idx ON plan_items (guide_id, step_id) WHERE kind = 'step';
COMMENT ON TABLE plan_items IS 'Items of a goal or custom plan, in order: a step (guide_id, step_id), a recipe or a task; never the guide text.';

-- A route plan's items come from the content: it stores none.
-- +goose StatementBegin
CREATE FUNCTION plan_items_not_route() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM plans WHERE id = NEW.plan_id AND kind = 'route') THEN
    RAISE EXCEPTION 'plan %: a route plan stores no items (they come from the content)', NEW.plan_id
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER plan_items_not_route BEFORE INSERT OR UPDATE ON plan_items
  FOR EACH ROW EXECUTE FUNCTION plan_items_not_route();

ALTER TABLE accounts ADD COLUMN active_plan_id TEXT;
ALTER TABLE accounts ADD CONSTRAINT accounts_active_plan_fkey
  FOREIGN KEY (active_plan_id, id) REFERENCES plans (id, account_id) ON DELETE SET NULL (active_plan_id);
COMMENT ON COLUMN accounts.active_plan_id IS 'The plan the account follows (docs/10 section 9); NULL is the route of guide_id.';

-- +goose Down
ALTER TABLE accounts DROP CONSTRAINT accounts_active_plan_fkey;
ALTER TABLE accounts DROP COLUMN active_plan_id;
DROP TRIGGER plan_items_not_route ON plan_items;
DROP FUNCTION plan_items_not_route();
DROP TABLE plan_items;
DROP TABLE plans;
DROP INDEX goals_id_account_idx;
