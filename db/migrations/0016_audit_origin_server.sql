-- +goose Up
-- The server acting by itself (docs/11 section 7.2): the workers' reconciler
-- restarts an outdated worker on the new image when it is paused, and audits
-- it as origin 'server', actor 'reconciler'. web is the owner's click, agent
-- what the agent did by itself (a trigger, a recurring routine).

ALTER TABLE microbot_audit DROP CONSTRAINT microbot_audit_origin;
ALTER TABLE microbot_audit
  ADD CONSTRAINT microbot_audit_origin CHECK (origin IN ('web', 'agent', 'routine', 'server'));

-- +goose Down
-- The audit is append-only: the server's rows stay, under the closest older
-- origin.
UPDATE microbot_audit SET origin = 'routine' WHERE origin = 'server';
ALTER TABLE microbot_audit DROP CONSTRAINT microbot_audit_origin;
ALTER TABLE microbot_audit
  ADD CONSTRAINT microbot_audit_origin CHECK (origin IN ('web', 'agent', 'routine'));
