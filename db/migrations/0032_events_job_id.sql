-- +goose Up
-- The job an event is about, indexed for a job's detail (docs/12 section
-- 13.8): a job's own events and its runs'. NULL for the rest.
ALTER TABLE events ADD COLUMN job_id TEXT;
COMMENT ON COLUMN events.job_id IS 'The job the event is about (its own events and its runs''), for the job''s detail; NULL for the rest.';

-- The events 0031 wrote name their job in the payload. Filling the new column
-- is not a change of history, so the append-only trigger is off for it.
ALTER TABLE events DISABLE TRIGGER events_no_update;
UPDATE events SET job_id = payload->>'job' WHERE coalesce(payload->>'job', '') <> '';
ALTER TABLE events ENABLE TRIGGER events_no_update;

DROP INDEX events_subject_idx;
CREATE INDEX events_job_idx ON events (job_id, id) WHERE job_id IS NOT NULL;
-- the tracker prunes the fact.changed events every consumer has passed
CREATE INDEX events_fact_changed_at_idx ON events (at) WHERE type = 'fact.changed';
COMMENT ON TABLE events IS 'Durable, append-only log the server''s consumers react to (docs/12 section 13.4); an event says "changed", the consumer reads the state. Ids follow the inserts, not the commits: writers hold the shared advisory lock 7123001 from the insert to the commit and a reader takes it exclusively, so it never reads past an event still uncommitted.';

-- +goose Down
COMMENT ON TABLE events IS 'Durable, append-only log the server''s consumers react to (docs/12 section 13.4); an event says "changed", the consumer reads the state.';
DROP INDEX events_fact_changed_at_idx;
DROP INDEX events_job_idx;
CREATE INDEX events_subject_idx ON events (subject, id) WHERE subject <> '';
ALTER TABLE events DROP COLUMN job_id;
