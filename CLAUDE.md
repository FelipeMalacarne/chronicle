# CLAUDE.md

Self-hosted planner/tracker for an OSRS Ironman account. It follows two independent, comparable routes:
the Oziris Ironman Efficiency Guide (`oziris-main`, 576 steps) and BRUHsailer (`bruhsailer-main`, 226
steps). One Go binary (`ironman`) serves the API + embedded Svelte SPA, and doubles as CLI and MCP
server; progress is auto-tracked from hiscores, Wise Old Man and a read-only Microbot (RuneLite) plugin
that lives in the separate `microbot-plugins` repository (`fm/ironmanplanner`).

Current state and the active build sequence: `docs/04-arquitetura.md` section 9. The domain model is
built around objectives (`docs/09-modelo-de-dominio.md`, approved 2026-11-04; decisions in sections
9.1 and 9.3); read it before touching progress, goals, the graph or the content schema. Open product
decisions are tracked in `docs/01-sumario-executivo.md`.

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
| `just test` / `just lint` / `just fmt` | `go test ./...` + `pnpm check` + `pnpm test`; golangci-lint + gofmt + `pnpm lint` |
| `just content-validate` | `ironman content validate` against `content/schema/guide.schema.json` |
| `go run ./cmd/ironman content migrate` | regenerate `content/guides/oziris-main` from `data/guide/normalized` |
| `go run ./cmd/ironman content import-bruhsailer` | regenerate `content/guides/bruhsailer-main` from the pinned DOCX in `data/sources/bruhsailer` |
| `just migrate-up` / `migrate-status` / `migrate-new <name>` | goose against `DATABASE_URL` |
| `just account-create <rsn>` / `account-guide <id> <guide>` / `accounts` | dev accounts (id `acc_<rsn>`) and the active route, until the account screen exists |
| `just dbml` | regenerate `docs/arquitetura/modelo-de-dados.dbml` from the migrated `DATABASE_URL` |
| `go test ./internal/core/entities/catalogbuild -update` | regenerate the derived catalogs in `content/reference` (offline) |
| `python3 tools/extract_wiki_items.py` | refresh `data/wiki/items.json` with every item page of the OSRS wiki (the only step that uses the network; cached under `~/.cache/ironman-planner`); then `-update` above and re-run both importers |
| `python3 data/wiki/scripts/build_quests.py data/samples/wiki-bucket/quest_all_p1.json 'data/samples/wiki-wikitext/quest_pages/batch*.json' <guide-list> data/wiki/quests.json` | rebuild `data/wiki/quests.json` (requirements, QP, quest XP rewards) offline from the saved bucket and wikitext; `<guide-list>` is a file of quest names to cross-check (may be empty) |
| `go run ./cmd/ironman content mcp` | extraction MCP server on stdio (Claude Code starts it from `.mcp.json`; skill `annotate-guide`) |
| `just sqlc` | regenerate `internal/store/db` from `db/queries/*.sql` |
| `just up` / `just down` / `just logs` | prod-like stack from `compose.yaml` (Postgres + app image) |

Run `just` alone to list everything. All tools also work without Nix (`go run pkg@latest`).

## Layout

```
cmd/ironman/          cobra subcommands: serve, content (migrate, validate, import-bruhsailer, mcp: the
                      offline extraction server over content/ only, docs/09 section 6; there is no
                      merge step: annotations are sidecars composed by the loader);
                      planned: now, done, plan, sync, state, mcp (account, over the API)
internal/core/        PURE domain (no DB/HTTP imports):
  content/            loader, schema validation, guide/variant/section/segment/step model
  content/derive/     deterministic bank-leg segments, bank actions and loadouts
  entities/           canonical entity registry (refs, catalogs, resolver, enrichment)
  ingest/             plugin wire contract v1 (manifest, push, item freshness) + validation
  rules/              rule engine incl. item_owned over item holdings
  planning/           next-leg readiness (ownership vs carried loadout vs bank instructions)
  progress/           objective verdicts (latched), derived step status, implied cascade, cursor
  annotate/           guide extraction: batches, name resolution, verification, sidecar/report writer
internal/mcp/         minimal stdio MCP server (JSON-RPC, tools only), shared by both MCP subcommands
internal/importer/    source importers (bruhsailer: pinned DOCX -> canonical guide)
internal/store/       repositories (ingest: tokens, pushes, holdings; progress: reevaluation, marks,
                      cursor, board, content index and step id aliases at boot; state: PlayerState
                      from player_state); internal/store/db is sqlc OUTPUT
internal/httpapi/     handlers: /api/guides/{guide}/..., /api/ingest, dev-only readiness and token bootstrap
                      planned: internal/sources (hiscores, wom, wiki), internal/client
web/                  Svelte SPA; web/embed.go embeds web/dist (`//go:embed all:dist`);
                      web/src/lib/progress: view-model (view.ts is pure and node-tested), API types;
                      web/src/lib/v4: components of the v4 screens (Pergaminho design system)
content/              guides/<id>/guide.json (generated by the importers) + annotations/ (objective
                      sidecars and decisions.json, written only by `content mcp`, docs/09 section 4.2) +
                      annotations.report.json + README + entities.report.json; schema/ (guide +
                      annotations schemas); reference/ (catalogs)
.mcp.json             declares `content mcp` (server `ironman-content`) for Claude Code;
.claude/skills/       annotate-guide: the extraction procedure and taxonomy the agent follows
db/migrations/        goose SQL for Postgres (0001 base schema, 0002 ingest freshness, 0003 guide ids,
                      0004 progress over objectives)
db/queries/           sqlc queries; sqlc.yaml at the root
data/                 original extraction: guide/ (input of `content migrate`), sources/bruhsailer (pinned DOCX)
docs/                 numbered design docs (PT-BR); tools/ has the Python extraction scripts
```

## Rules that must hold

1. **Only `ironman serve` opens the database.** CLI and MCP subcommands talk to the HTTP API with a
   token (`IRONMAN_URL`, `IRONMAN_TOKEN`). Never import `internal/store` from CLI/MCP code.
2. **Never mark a step done without evidence.** A step's `auto_done` is derived from its objectives;
   an objective is `met` only with an `evidence_snapshot_id` and an `evidence` JSON array in
   `objective_progress`, and `unknown` leaves the step `pending`. The owner's marks live in `step_marks`
   (`manual_done`, `skipped`, `pinned_open`, `implied`) and the engine never overrides them; `implied`
   is never `auto_done` (docs/09 section 5).
3. `content/` is the canonical content (`guide.json` from the importers plus `annotations/` sidecars);
   the DB only indexes it (`content_versions`, `content_steps`). Schemas live in `content/schema/`;
   `ironman content validate` covers guide and sidecars and must pass in CI.
4. `internal/core` has no DB or HTTP imports. Keep the rule engine pure and table-tested.
5. Schema changes = new goose migration (`just migrate-new`), then `just sqlc`. Never edit an applied
   migration; never hand-edit `internal/store/db`.
6. Keep attribution on any screen that shows guide steps: OzirisLoL / ironman.guide for Oziris, So Iron
   BRUH and ParasailerOSRS for BRUHsailer.
7. **Routes are independent; ids are guide-scoped.** Local step/section/segment ids may repeat across
   guides. Always address content by `(guide_id, id)`; never merge, layer or concatenate routes. In the
   web, step keys are `<guide>::<id>`.
8. **Item freshness is never upgraded.** `restored` is never treated as `live`; absence is proven only by
   a `live` container with a `full` baseline in the same live epoch; everything else is `unknown`.
   Bank actions are guide instructions and are never marked done (docs/04 section 6.4.1).
9. **Generated content is not hand-edited.** Fix the importer/normalizer and regenerate. LLM/agent
   extraction output is a proposal: it enters `content/` only after deterministic verification (exact
   quote, resolver-assigned refs, schema), per docs/09 section 6. During extraction the owner watches
   live: ask them (AskUserQuestion) whenever an objective is inferred, a name is ambiguous, the taxonomy
   does not fit or the verifier rejects something non-obvious; record answers as `confirmed_by: owner`
   and general rules in `annotations/decisions.json`. Never commit an unconfirmed inference.
10. Account-addressed routes (`/api/guides/{guide}/readiness?account=`) are registered only in
    `AUTH_MODE=dev` until session auth exists.
11. **The Claude Design canvas is the reference for every screen.** Before implementing or changing
    any screen or component in `web/`, read the canvas "Ironman Planner — Telas"
    (https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ) with the Artifact tool: `project/canvas.json`
    to find the artboards of the most recent approved page, then those artboards' `.dc.html`. Implement
    what the canvas shows (layout, states, copy, tokens of its design system) and say which artboard you
    followed. If the screen or state is not designed there, or the code must diverge from it, stop and
    ask the owner: designing it in the canvas first is the default. Never edit the canvas while
    implementing; design changes go through the design agent (`docs/ui/brief-ui-ux.md`).

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
- `cd web && pnpm check && pnpm test && pnpm lint`; CI runs web, go, migrations (goose up/down/up on
  Postgres 17) and a docker build on `main` (`.github/workflows/ci.yml`).
- `pnpm` may refuse to run scripts while a lockfile entry is younger than the `minimumReleaseAge` policy;
  that is the supply-chain policy, not a broken install.

## Docs by number

00 levantamento inicial (histórico) · 01 sumário executivo e registro de decisões · 02 fontes de dados
(APIs reais) · 03 produto e features · 04 arquitetura vigente em Go · 05 conteúdo e normalização
(mapeamento do importador na seção 10, adendo multi-guia na seção 11) · 06 cliente Microbot (plugin,
contrato de ingest) · 07 ambiente e ferramental · 08 controle do Microbot pelo planner (headless, VNC,
plugins, rotinas) · 09 modelo de domínio (objetivos, entidades, progresso compartilhado, extração por
agente; decisões em 9.1 e 9.3) · `docs/ui/brief-ui-ux.md` (brief de UI/UX).
`docs/04` section 9 is the current status and sequence; 6.4 is the ingest contract and 6.4.1 readiness.
`docs/arquitetura/types.ts` mirrors the ingest contract and readiness types in Zod, as a reference only.
`docs/arquitetura/modelo-de-dados.dbml` is the DB model in DBML: the part above the PLANNED marker is
generated from the migrations (`just migrate-up && just dbml`, needs `psql`); after every new migration,
regenerate it and drop from the planned block whatever now exists.
