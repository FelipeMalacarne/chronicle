# syntax=docker/dockerfile:1.7
#
# ironman-planner image: one static Go binary (server + CLI + MCP) with the
# Svelte SPA embedded and the guide content next to it.
#
#   docker build --build-arg VERSION=$(git describe --tags --always --dirty) -t ironman-planner .
#   docker run --rm -p 8080:8080 --env-file .env ironman-planner
#
# Healthcheck lives in compose.yaml (`/ironman health`): distroless has no
# shell or curl. `--target migrate` builds the one-shot migration image.

ARG VERSION=dev

# ---------------------------------------------------------------------------
# Stage 1: web (Svelte SPA -> web/dist)
# ---------------------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web

# pnpm 10 pinned explicitly (web/package.json has no `packageManager` field, so
# corepack alone would pick an arbitrary default). If that field is added later,
# switch to `RUN corepack enable` and the pinned version wins.
RUN npm install -g pnpm@10

# Install dependencies first so they are cached independently of source edits.
# The lockfile glob matches zero or one file; frozen install only when present.
COPY web/package.json web/pnpm-lock.yaml* web/.npmrc* ./
RUN if [ -f pnpm-lock.yaml ]; then pnpm install --frozen-lockfile; else pnpm install; fi

COPY web/ ./
RUN pnpm build

# ---------------------------------------------------------------------------
# Stage 2: go (static binary, web/dist embedded)
# ---------------------------------------------------------------------------
FROM golang:1.27-alpine AS go
ARG VERSION
WORKDIR /src

ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=mod

# Module download cached separately from source changes.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# web/dist is excluded by .dockerignore; take the fresh build from stage 1.
COPY --from=web /src/web/dist ./web/dist

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags "-s -w -X github.com/FelipeMalacarne/ironman-planner/internal/buildinfo.Version=${VERSION}" \
      -o /out/ironman ./cmd/ironman

# ---------------------------------------------------------------------------
# Stage 3: migrate (goose + db/migrations), the one-shot `migrate` service of
# compose.yaml. The server never migrates: only `ironman serve` opens the
# database, and it expects the schema in place (CLAUDE.md rules 1 and 5).
# ---------------------------------------------------------------------------
FROM golang:1.27-alpine AS goose
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    go install github.com/pressly/goose/v3/cmd/goose@v3.29.0

FROM gcr.io/distroless/static-debian12:nonroot AS migrate
COPY --from=goose /go/bin/goose /goose
COPY db/migrations /migrations
# GOOSE_DBSTRING comes from compose (the composed DATABASE_URL).
ENV GOOSE_DRIVER=postgres \
    GOOSE_MIGRATION_DIR=/migrations
USER nonroot:nonroot
ENTRYPOINT ["/goose"]
CMD ["up"]

# ---------------------------------------------------------------------------
# Stage 4: runtime (distroless, non-root); the default target
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app
COPY --from=go --chown=nonroot:nonroot /out/ironman /ironman
COPY --from=go --chown=nonroot:nonroot /src/content /app/content

ENV IRONMAN_ADDR=:8080 \
    CONTENT_DIR=/app/content

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/ironman"]
CMD ["serve"]
