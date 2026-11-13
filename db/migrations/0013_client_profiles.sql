-- +goose Up
-- Client profiles of the managed workers (docs/11 section 4.3, phase F5):
-- what a worker's client runs and how it is configured, kept here because a
-- pod loses its disk. A profile belongs to a user; a model is a profile that
-- other profiles name as parent. The effective profile of an account is the
-- model, then the profile, then the account's own settings
-- (account_client_settings), merged by agent.MergeProfiles and sent to the
-- worker in its bootstrap.
--
-- Invariants held by internal/store/workers, not by the database (they span
-- rows): a model has no parent of its own (one level) and belongs to the
-- same user; an account's profile belongs to the account's user; plugins and
-- settings pass agent.ValidateProfile (the groups and keys config_set takes).
-- Every profile write locks the users row first, and every account write the
-- accounts row, so these checks and the writes they guard are serialized.

CREATE TABLE client_profiles (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- the handle in the CLI and the API (/api/client-profiles/{name})
  name       TEXT NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9_-]{0,47}$'),
  -- the model this profile builds on; a model cannot be deleted while used
  parent_id  TEXT REFERENCES client_profiles(id) ON DELETE RESTRICT CHECK (parent_id <> id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, name)
);
COMMENT ON TABLE client_profiles IS 'Client profiles of the managed workers (docs/11 section 4.3): plugins and RuneLite settings, applied by the agent when the client comes up.';

CREATE TABLE client_profile_plugins (
  profile_id    TEXT NOT NULL REFERENCES client_profiles(id) ON DELETE CASCADE,
  -- start order
  position      INTEGER NOT NULL CHECK (position >= 0),
  source        TEXT NOT NULL CHECK (source IN ('hub', 'own', 'core')),
  -- the Hub's internal name (hub) or the fully qualified class (own, core)
  name          TEXT NOT NULL,
  -- a pinned Hub version; '' is the Hub's latest (hub only)
  version       TEXT NOT NULL DEFAULT '',
  -- the class a Hub plugin starts as (hub only)
  class_name    TEXT NOT NULL DEFAULT '',
  start_on_boot BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (profile_id, source, name),
  UNIQUE (profile_id, position)
);

CREATE TABLE client_profile_settings (
  profile_id   TEXT NOT NULL REFERENCES client_profiles(id) ON DELETE CASCADE,
  config_group TEXT NOT NULL,
  config_key   TEXT NOT NULL,
  value        TEXT NOT NULL,
  PRIMARY KEY (profile_id, config_group, config_key)
);

-- The account's profile; no row (or a null profile) is no profile: the
-- worker applies its fixed settings and plugins only.
CREATE TABLE account_client (
  account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  profile_id TEXT REFERENCES client_profiles(id) ON DELETE RESTRICT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The account's own settings over its profile (the overrides of docs/11
-- section 4.3, as rows rather than jsonb so they get the same checks).
CREATE TABLE account_client_settings (
  account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  config_group TEXT NOT NULL,
  config_key   TEXT NOT NULL,
  value        TEXT NOT NULL,
  PRIMARY KEY (account_id, config_group, config_key)
);

-- +goose Down
DROP TABLE account_client_settings;
DROP TABLE account_client;
DROP TABLE client_profile_settings;
DROP TABLE client_profile_plugins;
DROP TABLE client_profiles;
