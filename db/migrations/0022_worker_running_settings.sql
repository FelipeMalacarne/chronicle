-- +goose Up
-- The settings of the plugins a worker's client had on, as the reconciler
-- last read them (docs/11 section 15.6). A pod's home is new on every boot,
-- so what the owner changed in a plugin's options would vanish with it; the
-- next bootstrap writes these before it starts the plugins, over the client
-- profile. A JSON array of agent.ProfileSetting (group, key, value), written
-- only by internal/store/workers, never with a key that looks like a secret.
ALTER TABLE workers ADD COLUMN running_settings JSONB NOT NULL DEFAULT '[]'
  CHECK (jsonb_typeof(running_settings) = 'array');
COMMENT ON COLUMN workers.running_settings IS 'Explicit option values of the plugins the client had on at the last complete read; written again by the next bootstrap (docs/11 section 15.6).';

-- +goose Down
ALTER TABLE workers DROP COLUMN running_settings;
