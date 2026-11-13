-- +goose Up
-- Automatic login of a worker after a new pod (docs/11 section 15.5, W3):
-- an option of the account, off by default. A pod loses the game session
-- login with its memory, and the bootstrap leaves the client on the login
-- screen; with auto_login the reconciler logs in again, but only when the
-- worker had been logged in (was_logged_in).
--
-- was_logged_in is sticky on purpose: the reconciler sets it whenever it
-- reads the client logged in, and only the owner clears it (a logout from
-- the panel, or stopping the worker). A logout by a break of the client
-- leaves it set, so a pod that rolls during a break still logs in again.
ALTER TABLE workers
  ADD COLUMN auto_login    BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN was_logged_in BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN workers.auto_login IS 'Log the worker in again after a new pod, when was_logged_in (docs/11 section 15.5).';
COMMENT ON COLUMN workers.was_logged_in IS 'The client was last read logged in and the owner did not log it out since; kept by the reconciler (docs/11 section 15.5).';

-- +goose Down
ALTER TABLE workers DROP COLUMN was_logged_in, DROP COLUMN auto_login;
