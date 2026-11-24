-- +goose Up
-- A regular (non-ironman) account, docs/12 section 3: it follows a custom plan
-- and may later use the Grand Exchange. The mode check is the only place the
-- database enumerates the modes.
ALTER TABLE accounts DROP CONSTRAINT accounts_mode_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_mode_check
  CHECK (mode IN ('ironman','hcim','uim','gim','regular'));

-- +goose Down
-- A regular account has no ironman mode to fall back to, and its rows hang off
-- accounts through cascades.
DELETE FROM accounts WHERE mode = 'regular';
ALTER TABLE accounts DROP CONSTRAINT accounts_mode_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_mode_check
  CHECK (mode IN ('ironman','hcim','uim','gim'));
