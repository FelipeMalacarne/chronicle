# CLAUDE.md

Self-hosted planner/tracker for an OSRS Ironman account, built on the Oziris Ironman Efficiency Guide
(576 steps). One Go binary (`ironman`) serves the API + embedded Svelte SPA, and doubles as CLI and MCP
server; progress is auto-tracked from hiscores, Wise Old Man and a read-only Microbot (RuneLite) plugin.

## Language rules

- Docs under `docs/` and commit messages: Portuguese (PT-BR). Keep that convention.
- Code identifiers, code comments, CLAUDE.md, SQL comments and CI logs: English.
- UI strings: PT-BR (the owner's language); game terms (quest, diary names, skills) stay in English.

## Stack and toolchain

Go 1.27 (`go.mod` says `go 1.27`; pure Go, `CGO_ENABLED=0`), pgx + sqlc + goose on PostgreSQL 17,
Svelte 5 + SvelteKit (adapter-static) + Vite under `web/` (Node 22, pnpm 10), Docker distroless image,
Nix flake dev shell, `just` as task runner. Lint is golangci-lint v2 (`.golangci.yml`).

## Key commands (see `justfile`)

| Command | What it does |
|---|---|
| `just setup` | `pnpm install` in web/, `go mod download`, first `pnpm build` (go:embed needs `web/dist`) |
| `just dev` | Postgres from `compose.dev.yaml`, then `air` (or `go run ./cmd/ironman serve`) |
| `just web-dev` | Vite dev server with hot reload (proxied to :8080) |
| `just build` | web build + `bin/ironman` with `-X .../internal/buildinfo.Version=$(git describe)` |
| `just test` / `just lint` / `just fmt` | `go test ./...` + `pnpm check`; golangci-lint + gofmt + `pnpm lint` |
| `just content-validate` | `ironman content validate` against `content/schema/guide.schema.json` |
| `just migrate-up` / `migrate-status` / `migrate-new <name>` | goose against `DATABASE_URL` |
| `just sqlc` | regenerate `internal/store/db` from `db/queries/*.sql` |
| `just up` / `just down` / `just logs` | prod-like stack from `compose.yaml` (Postgres + app image) |

Run `just` alone to list everything. All tools also work without Nix (`go run pkg@latest`).

## Layout

```
cmd/ironman/          cobra subcommands: serve, now, done, plan, sync, state, mcp, content
internal/core/        PURE domain: content loader/schema, rules engine, graph, merged player state
internal/sources/     hiscores, wom, wikisync, wiki, ingest (plugin manifest + delta)
internal/store/       repositories; internal/store/db is sqlc OUTPUT (never edit by hand)
internal/httpapi/     handlers, OIDC/dev auth, sessions, tokens, SSE
internal/client/      Go client of the API, used by CLI and MCP
internal/mcp/         MCP tools over internal/client
web/                  Svelte SPA; web/embed.go embeds web/dist (`//go:embed all:dist`)
content/              guides/<id>/*.json (source of truth), schema/guide.schema.json, reference/
db/migrations/        goose SQL (0001_init.sql ports docs/arquitetura/schema.sql to Postgres)
db/queries/           sqlc queries; sqlc.yaml at the root
data/                 original extraction (read-only input for `ironman content migrate`)
docs/                 numbered design docs (PT-BR); tools/ has the Python extraction scripts
```

## Rules that must hold

1. **Only `ironman serve` opens the database.** CLI and MCP subcommands talk to the HTTP API with a
   token (`IRONMAN_URL`, `IRONMAN_TOKEN`). Never import `internal/store` from CLI/MCP code.
2. **Never mark a step done without evidence.** `auto_done` requires an `evidence_snapshot_id` and
   an `evidence` JSON array in `step_progress`; a rule that cannot be evaluated leaves the step
   `pending`. Manual completion is `manual_done` with `manual_locked` and the engine never overrides it.
3. `content/` is the canonical content; the DB only indexes it (`content_versions`, `content_steps`).
   The schema is `content/schema/guide.schema.json`; `ironman content validate` must pass in CI.
4. `internal/core` has no DB or HTTP imports. Keep the rule engine pure and table-tested.
5. Schema changes = new goose migration (`just migrate-new`), then `just sqlc`. Never edit an applied
   migration; never hand-edit `internal/store/db`.
6. Keep attribution to OzirisLoL / ironman.guide on any screen that shows guide steps.

## Environment variables

Read by the Go config (defaults in parentheses): `IRONMAN_ADDR` (`:8080`), `DATABASE_URL`, `AUTH_MODE`
(`dev` | `oidc`, default `dev`), `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URL`, `SESSION_SECRET`,
`CONTENT_DIR` (`content`), `LOG_LEVEL` (`info`), `PUBLIC_BASE_URL`. `.env.example` documents each one;
`compose.yaml` composes `DATABASE_URL` for the container from `POSTGRES_*`. Two modes:

- **local-first (workstation "zaros")**: `compose.yaml`/`compose.dev.yaml` Postgres, `AUTH_MODE=dev`.
- **saradomin**: same image, existing Postgres + Zitadel, `AUTH_MODE=oidc` + `OIDC_*`, behind the
  existing reverse proxy (docs/04 sections 8 and 9, docs/07).

## Tests

- `go test ./...` (unit; rule engine uses table tests with fixtures under `internal/core/.../testdata`).
- Store tests need `DATABASE_URL` pointing at a scratch Postgres (`just db-up`); skip when unset.
- `cd web && pnpm check && pnpm lint`; CI runs web, go, migrations (goose up/down/up on Postgres 17)
  and a docker build on `main` (`.github/workflows/ci.yml`).

## Docs by number

00 levantamento inicial · 01 sumário executivo · 02 fontes de dados (APIs reais) · 03 produto e
features · 04 arquitetura (seções 8 e 9 são as decisões atuais: Postgres, Zitadel, Go) · 05 conteúdo
e normalização · 06 cliente Microbot (plugin, contrato de ingest) · 07 ambiente e ferramental · 08 controle do
Microbot pelo planner (headless, VNC, plugins, rotinas).
`docs/arquitetura/` holds `schema.sql` (SQLite original), `guide.schema.json`, `types.ts`.
