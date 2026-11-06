-- +goose Up
-- The CLI and the account MCP server (docs/04 section 9, item 11) write marks
-- through the same API as the web; mark_commands records which client sent an
-- owner command, so the log tells a mark typed in the terminal or made by an
-- agent apart from one clicked in the web or the plugin panel.
ALTER TABLE mark_commands DROP CONSTRAINT mark_commands_via_check;
ALTER TABLE mark_commands ADD CONSTRAINT mark_commands_via_check
  CHECK (via IN ('web','plugin_panel','cli','mcp'));

-- +goose Down
-- mark_commands is append-only, so rows written by the CLI or MCP stay: the
-- narrower check applies to new rows only (NOT VALID).
ALTER TABLE mark_commands DROP CONSTRAINT mark_commands_via_check;
ALTER TABLE mark_commands ADD CONSTRAINT mark_commands_via_check
  CHECK (via IN ('web','plugin_panel')) NOT VALID;
