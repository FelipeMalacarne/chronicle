-- +goose Up
-- Two plugin facets (docs/06, adendo 2026-11-10): the Time Tracking clocks
-- (timer, one fact per tab with its ready_at; decision 31 of doc 01) and the
-- character's position (position, the last one only, opt-in on the client).
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item','timer','position'));

-- +goose Down
DELETE FROM player_state WHERE facet IN ('timer', 'position');
DELETE FROM snapshot_facts WHERE facet IN ('timer', 'position');
DELETE FROM observations WHERE facet IN ('timer', 'position');
ALTER TABLE observations DROP CONSTRAINT observations_facet_check;
ALTER TABLE observations ADD CONSTRAINT observations_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item'));
ALTER TABLE snapshot_facts DROP CONSTRAINT snapshot_facts_facet_check;
ALTER TABLE snapshot_facts ADD CONSTRAINT snapshot_facts_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item'));
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item'));
