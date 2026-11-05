-- +goose Up
-- Item 7 (docs/04 section 9): the eight typed snapshot_* tables collapse into
-- snapshot_facts, which has the shape of player_state (owner decision,
-- 2026-11-04), and player_state gains the 'diary' facet (a whole tier,
-- derived from the reference varbits of content/reference/varbits.json).
-- snapshot_item_containers stays: it is container provenance, not a fact.

CREATE TABLE snapshot_facts (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  facet       TEXT NOT NULL CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item')),
  key         TEXT NOT NULL,                   -- same keys as player_state: 'agility' | 'X Marks the Spot' | 'ardougne:easy' | '4458' | 'bank:4151'
  value       JSONB NOT NULL,                  -- same values as player_state; an item delta tombstone is {"qty":0}
  PRIMARY KEY (snapshot_id, facet, key)
);
COMMENT ON TABLE snapshot_facts IS 'Normalized facts of one snapshot, with the shape of player_state (history for charts and diffs).';
COMMENT ON TABLE snapshots IS 'Immutable raw snapshots per source. Normalized facts live in snapshot_facts.';

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'skill', skill,
       jsonb_strip_nulls(jsonb_build_object('level', level, 'xp', xp, 'rank', rank, 'boosted', boosted))
FROM snapshot_skills;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'activity', activity, jsonb_strip_nulls(jsonb_build_object('score', score, 'rank', rank))
FROM snapshot_activities;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'quest', quest, jsonb_build_object('state', state)
FROM snapshot_quests;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'diary_task', area || ':' || tier || ':' || task_index, jsonb_build_object('done', done)
FROM snapshot_diary_tasks;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'combat_achievement', ca_id::text, jsonb_build_object('done', done)
FROM snapshot_combat_achievements;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'collection_log', item_id::text, jsonb_build_object('qty', qty)
FROM snapshot_collection_log;

INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, kind, var_id::text, jsonb_build_object('value', value)
FROM snapshot_vars;

-- 'poh' was never written by the ingest; the container domain is now
-- bank | inventory | equipment everywhere, as in snapshot_item_containers.
INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
SELECT snapshot_id, 'item', container || ':' || item_id, jsonb_build_object('qty', qty)
FROM snapshot_items
WHERE container IN ('bank', 'inventory', 'equipment');

DROP TABLE snapshot_items;
DROP TABLE snapshot_vars;
DROP TABLE snapshot_collection_log;
DROP TABLE snapshot_combat_achievements;
DROP TABLE snapshot_diary_tasks;
DROP TABLE snapshot_quests;
DROP TABLE snapshot_activities;
DROP TABLE snapshot_skills;

ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item'));

-- +goose Down
-- Diary tiers have no typed table; the next push derives them again from the
-- varbits, so dropping them loses nothing.
DELETE FROM player_state WHERE facet = 'diary';
ALTER TABLE player_state DROP CONSTRAINT player_state_facet_check;
ALTER TABLE player_state ADD CONSTRAINT player_state_facet_check
  CHECK (facet IN ('skill','activity','quest','diary_task','combat_achievement','collection_log','varbit','varp','item'));

CREATE TABLE snapshot_skills (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  skill       TEXT NOT NULL,
  level       INTEGER NOT NULL,
  xp          BIGINT,
  rank        BIGINT,
  boosted     INTEGER,
  PRIMARY KEY (snapshot_id, skill)
);

CREATE TABLE snapshot_activities (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  activity    TEXT NOT NULL,
  score       BIGINT NOT NULL,
  rank        BIGINT,
  PRIMARY KEY (snapshot_id, activity)
);

CREATE TABLE snapshot_quests (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  quest       TEXT NOT NULL,
  state       TEXT NOT NULL CHECK (state IN ('NOT_STARTED','IN_PROGRESS','FINISHED')),
  PRIMARY KEY (snapshot_id, quest)
);

CREATE TABLE snapshot_diary_tasks (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  area        TEXT NOT NULL,
  tier        TEXT NOT NULL CHECK (tier IN ('easy','medium','hard','elite')),
  task_index  INTEGER NOT NULL,
  done        BOOLEAN NOT NULL,
  PRIMARY KEY (snapshot_id, area, tier, task_index)
);

CREATE TABLE snapshot_combat_achievements (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  ca_id       INTEGER NOT NULL,
  done        BOOLEAN NOT NULL,
  PRIMARY KEY (snapshot_id, ca_id)
);

CREATE TABLE snapshot_collection_log (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  item_id     INTEGER NOT NULL,
  qty         INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (snapshot_id, item_id)
);

CREATE TABLE snapshot_vars (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK (kind IN ('varbit','varp')),
  var_id      INTEGER NOT NULL,
  value       INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, kind, var_id)
);

CREATE TABLE snapshot_items (
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  container   TEXT NOT NULL CHECK (container IN ('bank','inventory','equipment','poh')),
  item_id     INTEGER NOT NULL,
  qty         INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, container, item_id),
  CONSTRAINT snapshot_items_qty_nonnegative CHECK (qty >= 0)
);

INSERT INTO snapshot_skills (snapshot_id, skill, level, xp, rank, boosted)
SELECT snapshot_id, key, (value->>'level')::integer, (value->>'xp')::bigint,
       (value->>'rank')::bigint, (value->>'boosted')::integer
FROM snapshot_facts WHERE facet = 'skill' AND value->>'level' IS NOT NULL;

INSERT INTO snapshot_activities (snapshot_id, activity, score, rank)
SELECT snapshot_id, key, (value->>'score')::bigint, (value->>'rank')::bigint
FROM snapshot_facts WHERE facet = 'activity' AND value->>'score' IS NOT NULL;

INSERT INTO snapshot_quests (snapshot_id, quest, state)
SELECT snapshot_id, key, value->>'state'
FROM snapshot_facts WHERE facet = 'quest';

INSERT INTO snapshot_diary_tasks (snapshot_id, area, tier, task_index, done)
SELECT snapshot_id, split_part(key, ':', 1), split_part(key, ':', 2),
       split_part(key, ':', 3)::integer, (value->>'done')::boolean
FROM snapshot_facts WHERE facet = 'diary_task';

INSERT INTO snapshot_combat_achievements (snapshot_id, ca_id, done)
SELECT snapshot_id, key::integer, (value->>'done')::boolean
FROM snapshot_facts WHERE facet = 'combat_achievement';

INSERT INTO snapshot_collection_log (snapshot_id, item_id, qty)
SELECT snapshot_id, key::integer, COALESCE((value->>'qty')::integer, 1)
FROM snapshot_facts WHERE facet = 'collection_log';

INSERT INTO snapshot_vars (snapshot_id, kind, var_id, value)
SELECT snapshot_id, facet, key::integer, (value->>'value')::integer
FROM snapshot_facts WHERE facet IN ('varbit', 'varp');

INSERT INTO snapshot_items (snapshot_id, container, item_id, qty)
SELECT snapshot_id, split_part(key, ':', 1), split_part(key, ':', 2)::integer, (value->>'qty')::integer
FROM snapshot_facts WHERE facet = 'item';

DROP TABLE snapshot_facts;
COMMENT ON TABLE snapshots IS 'Immutable raw snapshots per source. Normalized rows live in snapshot_* tables.';
