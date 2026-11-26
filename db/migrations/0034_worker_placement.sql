-- +goose Up
-- Where an account's game client runs (FEL-138): 'cluster' is a pod on
-- saradomin, anything else is the host name of an attached agent with slots
-- (what os.Hostname() returns there, "zaros"). The owner chooses it; the
-- reconciler converges the clients to it, so an account runs in one place.
ALTER TABLE workers ADD COLUMN placement TEXT NOT NULL DEFAULT 'cluster'
  CHECK (placement = 'cluster' OR placement ~ '^[A-Za-z0-9][A-Za-z0-9.-]{0,62}$');
COMMENT ON COLUMN workers.placement IS '''cluster'' (a pod on saradomin) or the host name of an attached agent with slots that runs the client.';
COMMENT ON TABLE workers IS 'What the reconciler should run per account (docs/11 section 4.5): a pod on saradomin or a slot of an attached agent, per placement. Legacy attached agents that declare their own account live in the link hub, not here.';

-- +goose Down
COMMENT ON TABLE workers IS 'Managed workers only (docs/11 section 4.5): what the reconciler should run per account. Attached agents live in the link hub, not here.';
ALTER TABLE workers DROP COLUMN placement;
