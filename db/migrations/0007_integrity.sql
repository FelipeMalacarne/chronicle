-- +goose Up
-- Integrity of progress in the database itself (docs/10 sections 4 and 6,
-- phase 0 slice a). Until now these invariants were held by Go only:
--   * a met verdict is latched with its evidence (CLAUDE.md rule 2);
--   * a met verdict has at least one evidence item;
--   * objective keys have the canonical shape (entities.ObjectiveKey);
--   * the snapshot behind a verdict, a fact or a container freshness row
--     belongs to the same account.
-- New constraints are added NOT VALID and validated only when the existing
-- rows already satisfy them, so an old inconsistency is reported (NOTICE)
-- instead of blocking the deploy; new rows are always checked.

-- +goose StatementBegin
CREATE FUNCTION objective_progress_keep_met_latched() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.verdict = 'met' AND (
       NEW.verdict <> 'met'
       OR NEW.evidence_snapshot_id IS DISTINCT FROM OLD.evidence_snapshot_id
       OR NEW.evidence IS DISTINCT FROM OLD.evidence
       OR NEW.first_met_at IS DISTINCT FROM OLD.first_met_at
     ) THEN
    RAISE EXCEPTION 'objective % of account % is met and latched; only regressed_at may change',
      OLD.objective_key, OLD.account_id
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER objective_progress_met_latch
  BEFORE UPDATE ON objective_progress
  FOR EACH ROW EXECUTE FUNCTION objective_progress_keep_met_latched();

ALTER TABLE objective_progress
  ADD CONSTRAINT objective_progress_met_evidence_not_empty CHECK (
    CASE WHEN verdict = 'met'
         THEN jsonb_typeof(evidence) = 'array' AND jsonb_array_length(evidence) > 0
         ELSE true END
  ) NOT VALID;

ALTER TABLE objective_progress
  ADD CONSTRAINT objective_progress_key_shape CHECK (objective_key ~ '^(qp>=[0-9]+|[a-z_]+:.+)$') NOT VALID;
ALTER TABLE objective_progress_events
  ADD CONSTRAINT objective_progress_events_key_shape CHECK (objective_key ~ '^(qp>=[0-9]+|[a-z_]+:.+)$') NOT VALID;
ALTER TABLE goals
  ADD CONSTRAINT goals_key_shape CHECK (objective_key ~ '^(qp>=[0-9]+|[a-z_]+:.+)$') NOT VALID;

ALTER TABLE snapshots ADD CONSTRAINT snapshots_id_account_key UNIQUE (id, account_id);
ALTER TABLE objective_progress
  ADD CONSTRAINT objective_progress_evidence_same_account
  FOREIGN KEY (evidence_snapshot_id, account_id) REFERENCES snapshots (id, account_id) NOT VALID;
ALTER TABLE player_state
  ADD CONSTRAINT player_state_snapshot_same_account
  FOREIGN KEY (snapshot_id, account_id) REFERENCES snapshots (id, account_id) NOT VALID;
ALTER TABLE ingest_container_freshness
  ADD CONSTRAINT ingest_container_freshness_snapshot_same_account
  FOREIGN KEY (snapshot_id, account_id) REFERENCES snapshots (id, account_id) NOT VALID;

-- +goose StatementBegin
DO $$
DECLARE
  c RECORD;
BEGIN
  FOR c IN
    SELECT conrelid::regclass AS tbl, conname
    FROM pg_constraint
    WHERE NOT convalidated
      AND connamespace = current_schema()::regnamespace
      AND conname IN (
        'objective_progress_met_evidence_not_empty', 'objective_progress_key_shape',
        'objective_progress_events_key_shape', 'goals_key_shape',
        'objective_progress_evidence_same_account', 'player_state_snapshot_same_account',
        'ingest_container_freshness_snapshot_same_account')
  LOOP
    BEGIN
      EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', c.tbl, c.conname);
    EXCEPTION WHEN check_violation OR foreign_key_violation THEN
      RAISE NOTICE 'constraint % on % left NOT VALID: existing rows violate it', c.conname, c.tbl;
    END;
  END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE ingest_container_freshness DROP CONSTRAINT ingest_container_freshness_snapshot_same_account;
ALTER TABLE player_state DROP CONSTRAINT player_state_snapshot_same_account;
ALTER TABLE objective_progress DROP CONSTRAINT objective_progress_evidence_same_account;
ALTER TABLE snapshots DROP CONSTRAINT snapshots_id_account_key;
ALTER TABLE goals DROP CONSTRAINT goals_key_shape;
ALTER TABLE objective_progress_events DROP CONSTRAINT objective_progress_events_key_shape;
ALTER TABLE objective_progress DROP CONSTRAINT objective_progress_key_shape;
ALTER TABLE objective_progress DROP CONSTRAINT objective_progress_met_evidence_not_empty;
DROP TRIGGER objective_progress_met_latch ON objective_progress;
DROP FUNCTION objective_progress_keep_met_latched();
