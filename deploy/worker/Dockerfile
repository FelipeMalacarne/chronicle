# syntax=docker/dockerfile:1.7
#
# chronicle-worker image (docs/11 sections 7.2 and 13.4): chronicle-worker-base (JDK, Xvfb, x11vnc,
# the pinned Microbot client, the plugins and the harness of microbot/; deploy/worker/build-base.sh)
# plus the chronicle binary of this commit running `chronicle agent --worker`. Built from the
# repository root:
#
#   base="$(deploy/worker/build-base.sh | tail -n1)"
#   docker build -f deploy/worker/Dockerfile --build-arg BASE="$base" \
#     --build-arg VERSION=$(git describe --tags --always --dirty) -t chronicle-worker .
#
# The web and go stages are the server's (Dockerfile at the root), so the binary is the same one
# and, on the same builder, the layers are shared. Keep them in step with it.

# ghcr.io/felipemalacarne/chronicle-worker-base:<Microbot version>-<sha7> (CI: the tag of
# `build-base.sh --tag`, built when GHCR lacks it). No default.
ARG BASE
ARG VERSION=dev

# ---------------------------------------------------------------------------
# Stage 1: web (Svelte SPA -> web/dist), as in the server's Dockerfile
# ---------------------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web

RUN npm install -g pnpm@10

COPY web/package.json web/pnpm-lock.yaml* web/.npmrc* ./
RUN if [ -f pnpm-lock.yaml ]; then pnpm install --frozen-lockfile; else pnpm install; fi

COPY web/ ./
RUN pnpm build

# ---------------------------------------------------------------------------
# Stage 2: go (static binary, web/dist embedded), as in the server's Dockerfile
# ---------------------------------------------------------------------------
FROM golang:1.27-alpine AS go
ARG VERSION
WORKDIR /src

ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=mod

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
COPY --from=web /src/web/dist ./web/dist

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags "-s -w -X github.com/FelipeMalacarne/chronicle/internal/buildinfo.Version=${VERSION}" \
      -o /out/chronicle ./cmd/chronicle

# ---------------------------------------------------------------------------
# Stage 3: the worker (the base plus the binary)
# ---------------------------------------------------------------------------
FROM ${BASE} AS worker

# Static binary (CGO_ENABLED=0): it needs nothing from the base. Owned by root, run by 65532.
COPY --from=go /out/chronicle /usr/local/bin/chronicle

# The base is a Nix dockerTools image: its PATH has the harness (/opt/microbot/bin) and the Nix
# profile, not necessarily /usr/local/bin.
ENV PATH=/usr/local/bin:${PATH}

USER 65532:65532
ENTRYPOINT ["chronicle", "agent", "--worker"]
