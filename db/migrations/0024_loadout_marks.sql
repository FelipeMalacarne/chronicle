-- +goose Up
-- The owner's marks on the items of a leg's loadout (docs/09 section 5, owner
-- decision of 2026-11-14): "I already have this item for this leg". A mark is
-- scoped to one (guide, segment) and one requirement, so it never leaks to
-- another leg or route. It is an input overlaid when readiness is computed,
-- never evidence: nothing is written to item holdings, snapshots or
-- player_state (item freshness is never upgraded).
CREATE TABLE loadout_marks (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  guide_id   TEXT NOT NULL,                      -- guide ids are content ids, not FKs
  segment_id TEXT NOT NULL,                      -- the leg (content.Segment.ID), local to the guide
  item_key   TEXT NOT NULL,                      -- planning.RequirementKey: id:<n>, any:<ids>, family:<f>, name:<lower>, "/noted" suffix
  note       TEXT NOT NULL DEFAULT '',
  marked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, guide_id, segment_id, item_key),
  CONSTRAINT loadout_marks_item_key_shape CHECK (item_key ~ '^(id|any|family|name):.+')
);
COMMENT ON TABLE loadout_marks IS 'The owner''s marks on the items of a leg''s loadout, per (guide, segment): they count as satisfied when readiness is computed, but are not evidence (holdings and player_state are untouched).';

-- +goose Down
DROP TABLE loadout_marks;
