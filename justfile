# ironman-planner task runner. `just` lists targets; `just <target>`.
# Works from the Nix dev shell or from any machine with go, node/pnpm, docker.

set dotenv-load := true
set shell := ["bash", "-euo", "pipefail", "-c"]

# Version embedded in the binary and used as image tag.
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`

# `docker compose` (plugin) with fallback to the standalone binary.
compose := if `docker compose version >/dev/null 2>&1 && echo yes || echo no` == "yes" { "docker compose" } else { "docker-compose" }

# Tools run through `go run` so they work without Nix. Pin here when needed.
goose := "go run github.com/pressly/goose/v3/cmd/goose@latest"
sqlc := "go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest"

# Falls back to the compose.dev.yaml defaults when .env is absent.
database_url := env("DATABASE_URL", "postgres://ironman:ironman@localhost:5432/ironman_planner?sslmode=disable")

# Version symbol is internal/buildinfo.Version (falls back to the VCS revision when unset).
ldflags := "-s -w -X github.com/FelipeMalacarne/ironman-planner/internal/buildinfo.Version=" + version

# List targets
default:
    @just --list --unsorted

# --- setup -----------------------------------------------------------------

# Install web deps, download Go modules and build web/dist once (go:embed needs it)
setup:
    cd web && pnpm install
    go mod download
    cd web && pnpm build
    @test -f .env || (cp .env.example .env && echo "created .env from .env.example; review it")

# --- dev -------------------------------------------------------------------

# Start Postgres (compose.dev.yaml) and run the API with live reload (air) or `go run`
dev: db-up
    @echo "DATABASE_URL={{database_url}}"
    @echo "SPA with hot reload: run 'just web-dev' in another terminal (Vite on :5173, proxied to :8080)."
    @echo "Without it the server serves the last 'pnpm build' embedded from web/dist."
    if command -v air >/dev/null 2>&1; then air; else go run ./cmd/ironman serve; fi

# Vite dev server for the SPA
web-dev:
    cd web && pnpm dev

# Only Postgres from compose.dev.yaml, detached
db-up:
    {{compose}} -f compose.dev.yaml up -d --wait

# Stop the dev Postgres (data kept in the volume)
db-down:
    {{compose}} -f compose.dev.yaml down

# psql against DATABASE_URL
db-shell:
    psql "{{database_url}}"

# --- build -----------------------------------------------------------------

# Build the SPA into web/dist
web-build:
    cd web && pnpm build

# Build web then the Go binary into bin/ironman (version from git describe)
build: web-build
    CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}}" -o bin/ironman ./cmd/ironman
    @echo "built bin/ironman ({{version}})"

# Build the Docker image (tagged with the git version and latest)
docker-build:
    docker build --build-arg VERSION={{version}} -t ironman-planner:{{version}} -t ironman-planner:latest .

# --- quality ---------------------------------------------------------------

# go test ./... plus `pnpm check` and `pnpm test` when the web project defines them
test:
    go test ./...
    if [ -f web/package.json ] && grep -q '"check"' web/package.json; then (cd web && pnpm check); fi
    if [ -f web/package.json ] && grep -q '"test"' web/package.json; then (cd web && pnpm test); fi

# golangci-lint, gofmt check and `pnpm lint` when defined
lint:
    golangci-lint run ./...
    @unformatted="$(gofmt -l $(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './web/*'))"; \
    if [ -n "$unformatted" ]; then echo "gofmt needed:"; echo "$unformatted"; exit 1; fi
    if [ -f web/package.json ] && grep -q '"lint"' web/package.json; then (cd web && pnpm lint); fi

# gofmt + goimports in place
fmt:
    gofmt -w $(git ls-files '*.go')
    if command -v goimports >/dev/null 2>&1; then goimports -w -local github.com/FelipeMalacarne/ironman-planner $(git ls-files '*.go'); fi

# --- content ---------------------------------------------------------------

# Validate content/ against content/schema/guide.schema.json
content-validate:
    go run ./cmd/ironman content validate

# Migrate data/guide/normalized into content/guides (idempotent)
content-migrate:
    go run ./cmd/ironman content migrate

# --- database --------------------------------------------------------------

# Apply pending migrations (goose) to DATABASE_URL
migrate-up:
    {{goose}} -dir db/migrations postgres "{{database_url}}" up

# Roll back the last migration
migrate-down:
    {{goose}} -dir db/migrations postgres "{{database_url}}" down

# Show migration status
migrate-status:
    {{goose}} -dir db/migrations postgres "{{database_url}}" status

# Regenerate docs/arquitetura/modelo-de-dados.dbml from DATABASE_URL (run after migrate-up)
dbml:
    DATABASE_URL="{{database_url}}" python3 tools/schema_to_dbml.py

# Create an account for AUTH_MODE=dev until the account screen exists (id: acc_<rsn>): just account-create "Iron Man"
account-create rsn mode="ironman" guide="oziris-main" layer="enhanced":
    @test -f "content/guides/{{guide}}/guide.json" || { echo "unknown guide {{guide}}"; exit 1; }
    psql "{{database_url}}" -v ON_ERROR_STOP=1 -v rsn="{{rsn}}" -v mode="{{mode}}" -v guide="{{guide}}" -v layer="{{layer}}" -f db/scripts/account_create.sql

# Switch the active route of an account (applies from the next reevaluation): just account-guide acc_iron_man bruhsailer-main
account-guide id guide:
    @test -f "content/guides/{{guide}}/guide.json" || { echo "unknown guide {{guide}}"; exit 1; }
    psql "{{database_url}}" -v ON_ERROR_STOP=1 -v id="{{id}}" -v guide="{{guide}}" -f db/scripts/account_guide.sql

# List accounts
accounts:
    psql "{{database_url}}" -c "SELECT id, rsn, mode, guide_id, layer, active FROM accounts ORDER BY created_at"

# Create a new migration file: just migrate-new add_goal_notes
migrate-new name:
    {{goose}} -dir db/migrations -s create {{name}} sql

# Generate Go code from db/queries into internal/store/db
sqlc:
    {{sqlc}} generate

# --- compose (local stack on zaros) ------------------------------------------

# psql inside the stack's Postgres container (it has no host port)
stack_psql := compose + " exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\" \"$@\"' psql"

# Build, migrate (one-shot `migrate` service) and start Postgres + app from compose.yaml
up:
    VERSION={{version}} {{compose}} up -d --build

# Restart the app: picks up content/ changes (mounted read-only) without a rebuild
restart:
    {{compose}} restart app

# Stop the compose stack (volumes kept)
down:
    {{compose}} down

# Follow logs (default: app). Example: just logs migrate
logs service="app":
    {{compose}} logs -f {{service}}

# Create an account in the stack (id: acc_<rsn>): just stack-account-create "Iron Man"
stack-account-create rsn mode="ironman" guide="oziris-main" layer="enhanced":
    @test -f "content/guides/{{guide}}/guide.json" || { echo "unknown guide {{guide}}"; exit 1; }
    {{stack_psql}} -v rsn="{{rsn}}" -v mode="{{mode}}" -v guide="{{guide}}" -v layer="{{layer}}" < db/scripts/account_create.sql

# Switch the active route of an account in the stack
stack-account-guide id guide:
    @test -f "content/guides/{{guide}}/guide.json" || { echo "unknown guide {{guide}}"; exit 1; }
    {{stack_psql}} -v id="{{id}}" -v guide="{{guide}}" < db/scripts/account_guide.sql

# List the stack's accounts
stack-accounts:
    {{stack_psql}} -c "SELECT id, rsn, mode, guide_id, layer, active FROM accounts ORDER BY created_at"

# Interactive psql in the stack's Postgres
stack-psql:
    {{compose}} exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'

# pg_dump of the stack's database to backups/ (custom format; restore with pg_restore)
db-backup:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p backups
    out="backups/ironman_planner-$(date +%Y%m%d-%H%M%S).dump"
    {{compose}} exec -T postgres sh -c 'pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$out"
    echo "wrote $out ($(du -h "$out" | cut -f1))"

# Remove containers AND volumes of both stacks (destroys local data)
nuke:
    {{compose}} down -v
    {{compose}} -f compose.dev.yaml down -v
