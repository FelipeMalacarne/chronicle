-- +goose Up
-- The plugins a worker's client had on at the reconciler's last complete read
-- (docs/11 section 15.4). A pod's home is new on every boot, so what the
-- owner turned on by hand would vanish with it; the next bootstrap starts
-- these on top of the account's client profile. A JSON array of
-- agent.ProfilePlugin (source, name, class_name, start), written only by
-- internal/store/workers.
ALTER TABLE workers ADD COLUMN running_plugins JSONB NOT NULL DEFAULT '[]'
  CHECK (jsonb_typeof(running_plugins) = 'array');
COMMENT ON COLUMN workers.running_plugins IS 'Plugins the worker client had on at the last complete read; started again by the next bootstrap, beside the client profile (docs/11 section 15.4).';

-- +goose Down
ALTER TABLE workers DROP COLUMN running_plugins;
