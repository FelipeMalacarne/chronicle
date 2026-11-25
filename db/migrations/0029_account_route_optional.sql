-- +goose Up
-- An account may follow no route (docs/11 decision W10, docs/12 section
-- 12.3): a regular account follows its active custom or goal plan and has no
-- guide. NULL guide_id is "no route"; a NULL active_plan_id then means the
-- account follows nothing yet.
ALTER TABLE accounts ALTER COLUMN guide_id DROP NOT NULL;
COMMENT ON COLUMN accounts.guide_id IS 'The route the account follows (content/); NULL: the account has no route.';
COMMENT ON COLUMN accounts.active_plan_id IS 'The plan the account follows (docs/10 section 9); NULL is the route of guide_id, or nothing without a route.';

-- +goose Down
-- There is no guide to give an account without a route. Refuse instead of
-- inventing one: the route drives the cursor and the implied cascade.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM accounts WHERE guide_id IS NULL) THEN
    RAISE EXCEPTION 'accounts without a route exist; give them a guide or delete them before rolling back 0029';
  END IF;
END
$$;
-- +goose StatementEnd
COMMENT ON COLUMN accounts.guide_id IS NULL;
COMMENT ON COLUMN accounts.active_plan_id IS 'The plan the account follows (docs/10 section 9); NULL is the route of guide_id.';
ALTER TABLE accounts ALTER COLUMN guide_id SET NOT NULL;
