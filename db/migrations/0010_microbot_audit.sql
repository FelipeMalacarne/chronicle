-- +goose Up
-- Audit of the Microbot control module (docs/08, Segurança item 9): every
-- action taken through it (from the web, later from routines), with who asked,
-- from where, what, and the result. Append-only; the control module never
-- writes progress.

CREATE TABLE microbot_audit (
  id      BIGSERIAL PRIMARY KEY,
  at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- user id of the session or token; 'dev' in AUTH_MODE=dev
  actor   TEXT NOT NULL,
  -- where the action came from: web, agent, routine
  origin  TEXT NOT NULL,
  -- one of the agent's closed command list (client_start, login, ...)
  action  TEXT NOT NULL,
  params  JSONB NOT NULL DEFAULT '{}'::jsonb,
  ok      BOOLEAN NOT NULL,
  error   TEXT NOT NULL DEFAULT '',
  CONSTRAINT microbot_audit_origin CHECK (origin IN ('web', 'agent', 'routine'))
);
CREATE INDEX microbot_audit_at_idx ON microbot_audit (at DESC);
COMMENT ON TABLE microbot_audit IS 'Every Microbot control action (docs/08): who, when, what, result. Append-only.';

-- +goose Down
DROP TABLE microbot_audit;
