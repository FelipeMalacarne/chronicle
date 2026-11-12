# CLAUDE.md

Self-hosted planner/tracker for an OSRS Ironman account. It follows two independent, comparable routes:
the Oziris Ironman Efficiency Guide (`oziris-main`, 576 steps) and BRUHsailer (`bruhsailer-main`, 226
steps). One Go binary (`chronicle`) serves the API + embedded Svelte SPA, and doubles as CLI and MCP
server; progress is auto-tracked from hiscores, Wise Old Man and a read-only Microbot (RuneLite) plugin
that lives in the separate `microbot-plugins` repository (Chronicle Link, `fm/chroniclelink`; it was
`fm/ironmanplanner` until 1.3).

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
| `just dev` | the stack's Postgres (`compose.yaml`, stops its `app`), `migrate-up`, then `air` (or `go run ./cmd/chronicle serve`) on :8080 and Vite with hot reload on :5173 together |
| `just web-dev` | Vite alone (proxied to :8080) |
| `just build` | web build + `bin/chronicle` with `-X .../internal/buildinfo.Version=$(git describe)` |
| `just test` / `just lint` / `just fmt` | `go test ./...` + `pnpm check` + `pnpm test`; golangci-lint + gofmt + `pnpm lint` |
| `just content-validate` | `chronicle content validate` against `content/schema/guide.schema.json` |
| `go run ./cmd/chronicle content migrate` | regenerate `content/guides/oziris-main` from `data/guide/normalized` plus the owner's route fixes in `data/guide/route-patches.json` (docs/05 section 12) |
| `go run ./cmd/chronicle content import-bruhsailer` | regenerate `content/guides/bruhsailer-main` from the pinned DOCX in `data/sources/bruhsailer` |
| `just migrate-up` / `migrate-status` / `migrate-new <name>` | goose against `DATABASE_URL` |
| `just account-create <rsn>` / `account-guide <id> <guide>` / `accounts` | dev accounts (id `acc_<rsn>`) and the active route, until the account screen exists |
| `just dbml` | regenerate `docs/arquitetura/modelo-de-dados.dbml` from the migrated `DATABASE_URL` |
| `go test ./internal/core/entities/catalogbuild -update` | regenerate the derived catalogs in `content/reference` (offline) |
| `python3 tools/fetch_wiki_icons.py` | refresh `web/static/icons` and `web/src/lib/icons/manifest.json` from the OSRS Wiki: items, skills, quest point, and the cockpit's prayers and spells (`data/wiki/book_icons.json`, keyed by the Microbot client's enum names); cached under `~/.cache/ironman-planner/icons` |
| `python3 tools/fetch_wiki_locations.py` | refresh `content/reference/location_maps.json`: the OSRS Wiki infobox map point of every location in `locations.json` (the Agora's "Aqui perto"); regions and instances stay unmapped; cached under `~/.cache/ironman-planner/wiki-locations` |
| `python3 tools/extract_wiki_items.py` | refresh `data/wiki/items.json` with every item page of the OSRS wiki (the only step that uses the network; cached under `~/.cache/ironman-planner`); then `-update` above and re-run both importers |
| `python3 data/wiki/scripts/build_quests.py data/samples/wiki-bucket/quest_all_p1.json 'data/samples/wiki-wikitext/quest_pages/batch*.json' <guide-list> data/wiki/quests.json` | rebuild `data/wiki/quests.json` (requirements, QP, quest XP rewards) offline from the saved bucket and wikitext; `<guide-list>` is a file of quest names to cross-check (may be empty) |
| `go run ./cmd/chronicle content mcp` | extraction MCP server on stdio (Claude Code starts it from `.mcp.json`; skill `annotate-guide`) |
| `just sqlc` | regenerate `internal/store/db` from `db/queries/*.sql` |
| `just up` / `just down` / `just logs` / `just restart` | the stack used on zaros (`compose.yaml`): images built from the committed HEAD (`git archive`), Postgres + one-shot `migrate` (goose) + app on 127.0.0.1 |
| `just stack-account-create <rsn>` / `stack-account-guide` / `stack-accounts` / `stack-psql` / `db-backup` | accounts, psql and `pg_dump` (to `backups/`) inside the stack's Postgres, which has no host port |
| `chronicle health` | GET `/api/health` of a running server (container healthcheck; never opens the database) |
| `chronicle login` / `now` / `done` / `plan` / `state` / `mcp` | API clients (docs/04 section 2.2): server from `--url`, `CHRONICLE_URL`, `~/.config/chronicle/config.json`, default `https://chronicle.saradomin.ftm.dev.br` (`CHRONICLE_URL=http://localhost:8080` for `just dev`); dev needs no login, oidc uses the Zitadel device flow; `CHRONICLE_TOKEN` overrides the stored login. `done`/`skip`/`undo` write the owner's marks: never run them on the owner's account unasked |
| `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update` | rewrite `web/src/lib/api/contract/`: one real API answer per route the web reads, as `export const fixture = {…} satisfies <web type>`; then `pnpm check`. Without `-update` the test fails when an answer changes shape |
| `just agent` / `chronicle agent` | Microbot control agent (docs/08) on the game machine: outbound link to the planner (server resolved like the CLI, default saradomin; `MICROBOT_CONTROL=on` there; no token against a dev server, which takes the link only from loopback; against oidc a Zitadel machine user with the `microbot:agent` role, `MICROBOT_AGENT_CLIENT_ID`/`_SECRET`), runs only the closed command list against the harness's dev client. Never log the client in for a test without the owner's agreement |
| `chronicle accounts connect --character <name>` / `chronicle workers list\|start\|stop\|restart` | docs/11: store an account's game session from the Microbot launcher (only that character's lines of the HWID stores; a character the launcher lists as "Not set" is `--index <n> --as <name in game>`, like `mb-account use --as`), and the workers' lifecycle over `/api/workers`. `start` and `restart` never log in, but `stop` and `restart` log the account out: never run them on the owner's account unasked |
| `chronicle progress check --account <id>` | GET `/api/progress/check` of a running server (server and login like the CLI): marks vs the mark log, player_state vs the merge of observations, verdicts/statuses/cursors vs a reevaluation; exit 1 on any difference (docs/10 section 6). The boot runs it too and only warns |

Run `just` alone to list everything. All tools also work without Nix (`go run pkg@latest`).

## Layout

```
cmd/chronicle/          cobra subcommands: serve, content (migrate, validate, import-bruhsailer, mcp: the
                      offline extraction server over content/ only, docs/09 section 6; there is no
                      merge step: annotations are sidecars composed by the loader); API clients:
                      login, logout, accounts, use, now, done, skip, undo, plan, sync, state and mcp
                      (the account's MCP server, over the API)
internal/core/        PURE domain (no DB/HTTP imports):
  content/            loader, schema validation, guide/variant/section/segment/step model
  content/derive/     deterministic bank-leg segments, bank actions and loadouts
  entities/           canonical entity registry (refs, catalogs, resolver, enrichment)
  facts/              merge of each source's last observation into the current fact (player_state)
  ingest/             plugin wire contract v1 (manifest, push, item freshness) + validation
  rules/              rule engine incl. item_owned over item holdings
  planning/           next-leg readiness (ownership vs carried loadout vs bank instructions)
  progress/           objective verdicts (latched), derived step status, implied cascade, cursor
  panel/              route window of the plugin panel (cursor step, next steps, labels, carry)
  annotate/           guide extraction: batches, name resolution, verification, sidecar/report writer
internal/mcp/         minimal stdio MCP server (JSON-RPC, tools only), shared by both MCP subcommands
internal/importer/    source importers (bruhsailer: pinned DOCX -> canonical guide)
internal/store/       repositories (identity: OIDC users, dev-account claim, ownership; ingest: tokens, pushes, holdings; progress: reevaluation, marks,
                      cursor, board, goals, content index and step id aliases at boot; state:
                      PlayerState from player_state, and the projection observations -> player_state; syncrun: hiscores runs, backoff, scheduler); internal/store/db is sqlc OUTPUT
internal/auth/        OIDC web login (PKCE flow, sealed state cookie, policy), JWT access tokens of the native
                      client (bearer of the CLI and MCP); authtest: fake issuer for tests (incl. device flow)
internal/httpapi/     handlers: /api/guides/{guide}/..., /api/ingest, /auth/* and /api/me, account routes
                      (incl. /api/world/entity for the entity panel, /api/account/state for Conta)
                      (dev without session, oidc behind it), dev-only token bootstrap
internal/sources/     adapters of external sources: hiscores (fetch + pure normalize); planned: wom, wiki
internal/microbot/    typed client of the Microbot Agent Server and of Chronicle's control bridge (Chronicle Link plugin; Unix sockets of the
                      harness dev home, same token); agent only
internal/vault/       AES-256-GCM of the accounts' game sessions (CHRONICLE_VAULT_KEY), pure
internal/kube/        minimal Kubernetes REST client (Deployments by server-side apply, pods, TokenReview), the
                      worker Deployment of docs/11 section 13.3 and the worker pod token check
internal/workerctl/   reconciler of the managed workers (database -> cluster) and the bootstrap of a linking
                      worker agent (session, ipl_ token, world); serve only
internal/agent/       Microbot control link (docs/08): closed command list, the agent's executor and outbound
                      websocket, the server-side Link, the VNC tunnel (browser <-> server <-> agent <->
                      loopback x11vnc) and the plugin/script/config commands, whose params are validated
                      by ValidateParams on both sides; the server never talks to the game directly
internal/client/      typed API client of the CLI and `chronicle mcp`: config.json and credentials.json
                      under ~/.config/chronicle, device-flow login (RFC 8628), token refresh; never
                      imports internal/store
web/                  Svelte SPA; web/embed.go embeds web/dist (`//go:embed all:dist`);
                      web/src/lib/progress: view-model (view.ts is pure and node-tested), API types;
                      web/src/lib/v6: components of the v6 screens (Pergaminho design system, canvas page v6)
content/              guides/<id>/guide.json (generated by the importers) + annotations/ (objective
                      sidecars and decisions.json, written only by `content mcp`, docs/09 section 4.2) +
                      annotations.report.json + README + entities.report.json; schema/ (guide +
                      annotations schemas); reference/ (catalogs)
.mcp.json             declares `content mcp` (server `chronicle-content`) and `chronicle mcp` (server `chronicle`,
                      the account's progress over the API of `just dev`) for Claude Code;
.claude/skills/       annotate-guide: the extraction procedure and taxonomy the agent follows
db/migrations/        goose SQL for Postgres (0001 base schema, 0002 ingest freshness, 0003 guide ids,
                      0004 progress over objectives, 0005 snapshot facts, 0006 plugin installations,
                      0007 integrity: met latch trigger, non-empty evidence, key shape, same-account snapshots,
                      0008 observations per source, player_state as their projection,
                      0009 guides + mark_commands: append-only log of every step_marks change,
                      0010 microbot_audit: every Microbot control action, 0011 timers and position,
                      0012 workers: game_sessions (sealed), workers (desired state), accounts.managed)
db/queries/           sqlc queries; sqlc.yaml at the root
deploy/k8s/           Kubernetes manifests of the saradomin deploy (base + overlays/saradomin); CI pushes
                      the chronicle image to GHCR and writes its tag in the overlay, which Argo CD follows
                      (docs/07 section 3.3); the cluster side (namespace, database, secrets) is in gielinor
data/                 original extraction: guide/ (input of `content migrate`), sources/bruhsailer (pinned DOCX)
docs/                 numbered design docs (PT-BR); tools/ has the Python extraction scripts
```

## Rules that must hold

1. **Only `chronicle serve` and `chronicle migrate` open the database.** `migrate` only applies the
   goose migrations embedded in the binary (`db/migrations.go`); the server never migrates. CLI and MCP
   subcommands talk to the HTTP API with a token (`CHRONICLE_URL`, `CHRONICLE_TOKEN`). Never import
   `internal/store` from CLI/MCP code.
2. **Never mark a step done without evidence.** A step's `auto_done` is derived from its objectives;
   an objective is `met` only with an `evidence_snapshot_id` and an `evidence` JSON array in
   `objective_progress`, and `unknown` leaves the step `pending`. The owner's marks live in `step_marks`
   (`manual_done`, `skipped`, `pinned_open`, `implied`) and the engine never overrides them; every change
   to `step_marks` is appended to `mark_commands` with its effects in the same transaction (docs/10
   section 6), so never write `step_marks` outside `internal/store/progress`; `implied`
   is never `auto_done` (docs/09 section 5).
3. `content/` is the canonical content (`guide.json` from the importers plus `annotations/` sidecars);
   the DB only indexes it (`content_versions`, `content_steps`). Schemas live in `content/schema/`;
   `chronicle content validate` covers guide and sidecars and must pass in CI.
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
9. **Generated content is not hand-edited.** Fix the importer/normalizer and regenerate. The route is
   meant to be good, not a literal copy: owner route fixes (drop, move, insert steps) go in
   `data/guide/route-patches.json`. LLM/agent
   extraction output is a proposal: it enters `content/` only after deterministic verification (exact
   quote, resolver-assigned refs, schema), per docs/09 section 6. During extraction the owner watches
   live: ask them (AskUserQuestion) whenever an objective is inferred, a name is ambiguous, the taxonomy
   does not fit or the verifier rejects something non-obvious; record answers as `confirmed_by: owner`
   and general rules in `annotations/decisions.json`. Never commit an unconfirmed inference.
10. Account-addressed routes (`?account=`: board, marks, readiness, goals, sync, `/api/events`, ...)
    exist without a session only in `AUTH_MODE=dev`. In `oidc` they are wrapped by the session
    middleware (`Server.authed`: 401 without a session, `X-Ironman-CSRF` on writes) and
    `Server.accountParam` restricts them to accounts owned by the session user (404 otherwise). Never
    register one outside that pair (docs/04 section 2.5).
11. **The Claude Design canvas is the reference for every screen.** Before implementing or changing
    any screen or component in `web/`, read the canvas "Ironman Planner — Telas"
    (https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ) with the Artifact tool: `project/canvas.json`
    to find the artboards of the most recent approved page, then those artboards' `.dc.html`. Implement
    what the canvas shows (layout, states, copy, tokens of its design system) and say which artboard you
    followed. If the screen or state is not designed there, or the code must diverge from it, stop and
    ask the owner: designing it in the canvas first is the default. Never edit the canvas while
    implementing; design changes go through the design agent (`docs/ui/brief-ui-ux.md`).

## Environment variables

The product was "ironman" until 2026-11-11 (binary, `IRONMAN_*` variables, `~/.config/ironman`):
`internal/appenv` reads each `CHRONICLE_*` variable with the old `IRONMAN_*` name as a fallback and
moves `~/.config/ironman` to `~/.config/chronicle` on first use. `IRONMAN_*` names in the game's
varbits and the account mode `ironman` are the game's, not the product's.

Read by the Go config (defaults in parentheses): `CHRONICLE_ADDR` (`:8080`), `DATABASE_URL`, `AUTH_MODE`
(`dev` | `oidc`, default `dev`), `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URL` (`PUBLIC_BASE_URL/auth/callback`), `OIDC_ALLOWED_SUBJECTS`,
`OIDC_REQUIRED_ROLE` (oidc needs one of the two; both set = both required), `OIDC_PROJECT_ID`, `OIDC_NATIVE_CLIENT_ID`
(Zitadel native app of the device flow: its JWT access tokens are bearers on the account routes; empty = web
session only), `SESSION_SECRET` (32+ chars in oidc),
`CONTENT_DIR` (`content`), `LOG_LEVEL` (`info`), `PUBLIC_BASE_URL`, `SYNC_HISCORES_MINUTES` (`30`; `0` disables the scheduler),
`MICROBOT_CONTROL` (`off`; `on` enables the Microbot control module, docs/08; in oidc it needs `OIDC_PROJECT_ID`, the
audience of the agent's machine-user token). `chronicle agent` reads `CHRONICLE_URL`, `MICROBOT_AGENT_CLIENT_ID`,
`MICROBOT_AGENT_CLIENT_SECRET` (or `_SECRET_FILE`; oidc only), `MICROBOT_HARNESS_DIR`, `MICROBOT_HOME`, `MB_VNC_PORT` and, to override the
harness tools it runs, `MICROBOT_CLIENT_CMD` and `MICROBOT_ACCOUNT_CMD`, and `MICROBOT_AGENT_ACCOUNT` (the account an
attached agent declares). Workers (docs/11): `CHRONICLE_VAULT_KEY` (32 bytes base64; with `MICROBOT_CONTROL=on` it turns on
`/api/accounts/session` and `/api/workers`), `WORKERS` (`off`; `on` runs the reconciler, in a pod only), `WORKER_IMAGE`,
`WORKER_NAMESPACE`, `WORKER_SERVER_URL`; `chronicle agent --worker` reads `WORKER_ACCOUNT`, `CHRONICLE_URL`,
`CHRONICLE_AGENT_TOKEN_FILE` and the harness's `MB_DEV_HOME`, `MB_XMX`, `MB_HWID_STORE_DIR`. `.env.example` documents each one;
`compose.yaml` composes `DATABASE_URL` for the container from `POSTGRES_*`. Two modes:

- **local-first (workstation "zaros")**: `compose.yaml` Postgres (shared by the stack and `just dev`), `AUTH_MODE=dev`.
- **saradomin**: same image, existing Postgres + Zitadel, `AUTH_MODE=oidc` + `OIDC_*`, behind the
  existing reverse proxy (docs/04 sections 8 and 9, docs/07).

## UI design reference (read before touching `web/`)

- **Screens**: the Claude Design canvas <https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ>, page set
  "v6" only (one canvas page per product screen: 0 Marca e fundamentos, 1 Agora, 2 Rotas, 3 Mundo,
  4 Conta, 5 Automação). Artboards are `project/V6<Screen><Width>.dc.html` (`1440` desktop with the
  sidebar, `640` the window beside the game); read one with the Artifact tool (`action: read`, `path`).
  Older pages v0 to v5 were deleted on 2026-11-05; never recreate them. Implementation goes screen by
  screen after the owner approves the screen in the canvas, starting with Agora.
- **Design system**: Pergaminho <https://claude.ai/artifact/WrVkgXPtCrRJPeUSgpHWVC>. Tokens are mirrored
  in `docs/ui/pergaminho.tokens.json` (two themes, `pergaminho` light and `pedra` dark) and the brand
  book in `docs/ui/pergaminho.md`; the artifact also has React previews of the atoms (`Plaque`,
  `SkillCell`, `InventorySlot`, `StepRow`, `ObjectiveChip`, `BankCard`). In `web/` use the tokens as CSS
  variables and the same class names (`pg-*`), no React.
- **Decisions**: `docs/ui/decisoes-canvas.md` (section 5 is current) and doc 01 decisions 29 to 31. The
  product is called **Chronicle** ("Ironman Planner" is the description); Comparar is a tab of Rotas;
  sidebar option A on 1440; one bevel only (panel, plaque, controls), everything inside panels flat;
  no system text in paragraphs (explanations go to `title`); real OSRS Wiki sprites for skills and items
  with the Jagex / OSRS Wiki CC BY-NC-SA 3.0 credit; `unknown` is never shown as `unmet`.
- The sticky notes on the canvas ("Decisões com o dono", "Rota · decisões", "Consolidação") are the
  owner's record; keep them when republishing `project/canvas.json`, and re-read the live index right
  before every publish because other sessions edit the same artifact.

## Tests

- `go test ./...` (unit; rule engine uses table tests with fixtures under `internal/core/.../testdata`).
- Store tests need `DATABASE_URL` (`just db-up`); each test runs in its own throwaway schema; skip when unset.
  CI's `go` job has a Postgres 17 service, so they run there; the progress ones end with `Repository.Check`.
- Contract: `web/src/lib/progress/types.ts` is still hand-written, but `svelte-check` verifies it against the
  generated fixtures in `web/src/lib/api/contract/` (never edit those; regenerate with `-update`).
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
agente; decisões em 9.1 e 9.3) · 10 modelo de execução (proposta: fatos por fonte, log de eventos,
tarefas, receitas, planos; decisões E1 a E10 abertas) · 11 workers do Microbot (proposta: um worker por conta, no zaros ou num pod criado pelo Chronicle; sessões, perfis do cliente e rotinas no Postgres; fases F0 a F7; decisões W1 a W10) · `docs/ui/brief-ui-ux.md` (brief de UI/UX) · `docs/ui/decisoes-canvas.md` (decisões do canvas, v6 vigente) ·
`docs/ui/pergaminho.md` + `pergaminho.tokens.json` (design system).
`docs/04` section 9 is the current status and sequence; 6.4 is the ingest contract and 6.4.1 readiness.
`docs/arquitetura/types.ts` mirrors the ingest contract and readiness types in Zod, as a reference only.
`docs/arquitetura/modelo-de-dados.dbml` is the DB model in DBML: the part above the PLANNED marker is
generated from the migrations (`just migrate-up && just dbml`, needs `psql`); after every new migration,
regenerate it and drop from the planned block whatever now exists.
