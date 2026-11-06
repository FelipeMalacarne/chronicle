-- +goose Up
-- Facts per source (docs/10 section 5, decision E1). observations keeps the
-- last value each source reported for a fact; player_state becomes a
-- projection that internal/core/facts.Merge computes from it (the greatest
-- value for facts that only grow, the newest for the rest). Writers never
-- touch player_state directly any more: they write observations and project
-- the facets they touched, and `ironman serve` re-projects every account at
-- boot.

CREATE TABLE observations (
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  facet       TEXT NOT NULL CHECK (facet IN ('skill','activity','quest','diary','diary_task','combat_achievement','collection_log','varbit','varp','item')),
  key         TEXT NOT NULL,
  source      TEXT NOT NULL CHECK (source IN ('hiscores','wom','wikisync','plugin','import')),
  value       JSONB NOT NULL,
  snapshot_id TEXT NOT NULL,
  observed_at TIMESTAMPTZ NOT NULL,             -- bounded by the server clock (docs/10 section 5)
  PRIMARY KEY (account_id, facet, key, source),
  CONSTRAINT observations_snapshot_same_account
    FOREIGN KEY (snapshot_id, account_id) REFERENCES snapshots (id, account_id)
);
CREATE INDEX observations_source_time_idx ON observations (account_id, source, observed_at DESC);
COMMENT ON TABLE observations IS 'Last observation of each source per fact; player_state is its merge (internal/core/facts).';
COMMENT ON TABLE player_state IS 'Projection: the merge of observations per fact (internal/core/facts.Merge). Written only by the projection.';

-- Seed: every current fact is the last observation of its source.
INSERT INTO observations (account_id, facet, key, source, value, snapshot_id, observed_at)
SELECT account_id, facet, key, source, value, snapshot_id, observed_at FROM player_state;

-- "Last write wins" dropped the hiscores values the plugin overwrote; the
-- newest hiscores snapshot of each account still has them.
INSERT INTO observations (account_id, facet, key, source, value, snapshot_id, observed_at)
SELECT s.account_id, f.facet, f.key, s.source, f.value, s.id, s.fetched_at
FROM snapshots s
JOIN snapshot_facts f ON f.snapshot_id = s.id
WHERE s.id IN (
  SELECT DISTINCT ON (account_id) id FROM snapshots
  WHERE source = 'hiscores'
  ORDER BY account_id, fetched_at DESC, id DESC
)
ON CONFLICT (account_id, facet, key, source) DO NOTHING;

-- +goose Down
COMMENT ON TABLE player_state IS 'Current merged fact per (account, facet, key). Rewritten after every snapshot.';
DROP TABLE observations;
