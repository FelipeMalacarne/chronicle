-- +goose Up
-- Grand Exchange prices (docs/12 sections 6 and 12.3): the last price of each
-- item from the OSRS Wiki real-time prices API (/latest and /mapping), with
-- where and when it came from. Belongs to no account. high is the last
-- instant-buy price and low the last instant-sell, each with the time of
-- that trade; either is NULL when the wiki never saw one. buy_limit (the
-- GE's 4-hour limit) and members come from /mapping, NULL when it lacks the
-- item.
CREATE TABLE item_prices (
  item_id    integer PRIMARY KEY CHECK (item_id > 0),
  high       bigint CHECK (high > 0),
  high_at    timestamptz,
  low        bigint CHECK (low > 0),
  low_at     timestamptz,
  buy_limit  integer CHECK (buy_limit > 0),
  members    boolean,
  source     text NOT NULL CHECK (source IN ('wiki')),
  fetched_at timestamptz NOT NULL,
  CHECK ((high IS NULL) = (high_at IS NULL)),
  CHECK ((low IS NULL) = (low_at IS NULL)),
  CHECK (high IS NOT NULL OR low IS NOT NULL)
);

-- The membership facet (docs/06; docs/12 section 12.3): the days of
-- membership left and whether the account is a member, as Chronicle Link
-- reads them in game.
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame','membership'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame','membership'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame','membership'));

-- +goose Down
-- Refuse instead of deleting the membership facts: they are the accounts'
-- own observations. item_prices is a cache of the wiki and is dropped; the
-- next sync after an up refills it.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM observations WHERE facet = 'membership')
     OR EXISTS (SELECT 1 FROM snapshot_facts WHERE facet = 'membership')
     OR EXISTS (SELECT 1 FROM player_state WHERE facet = 'membership') THEN
    RAISE EXCEPTION 'membership facts exist; remove them before rolling back 0028';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));
DROP TABLE item_prices;
