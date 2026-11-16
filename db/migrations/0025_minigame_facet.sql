-- +goose Up
-- The minigame facet (docs/06, adendo 2026-11-16): counters a minigame shows
-- only in its interface, one fact per "<minigame>.<counter>" key, such as the
-- Mage Training Arena's pizazz points (mta.telekinetic, ...).
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position','minigame'));

-- +goose Down
DELETE FROM player_state WHERE facet = 'minigame';
DELETE FROM snapshot_facts WHERE facet = 'minigame';
DELETE FROM observations WHERE facet = 'minigame';
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));
