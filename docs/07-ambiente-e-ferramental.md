# Ambiente e ferramental

Data: 2026-11-04. Complementa o doc 04 (seções 1 e 8). Descreve o que cada ferramenta faz, os dois
modos de execução (local-first no `zaros` e deploy no `saradomin`), as variáveis de ambiente, o CI e
os problemas mais comuns.

## 1. Decisão: local-first primeiro

O MVP roda na estação do dono (`zaros`) com `docker compose` (Postgres 17 + app) e `AUTH_MODE=dev`,
sem login. O deploy no `saradomin` (Postgres e Zitadel já existentes) vem depois, **com a mesma imagem**
e só variáveis de ambiente diferentes. Nada no código distingue os dois ambientes além da configuração.

| | local-first (`zaros`) | `saradomin` |
|---|---|---|
| Banco | `postgres:17-alpine` do `compose.yaml` (ou `compose.dev.yaml` para dev no host) | Postgres existente; database `ironman_planner` |
| Auth | `AUTH_MODE=dev` (um usuário local fixo) | `AUTH_MODE=oidc` + `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URL` (Zitadel, PKCE) |
| Rede | `localhost:8080` | atrás do reverse proxy existente, `PUBLIC_BASE_URL=https://planner.<domínio>` |
| Imagem | `just up` builda local | mesma imagem publicada no registry do servidor |
| Migrations | `just migrate-up` do host | `goose up` no deploy (mesmo comando, `DATABASE_URL` do servidor) |
| Backup | volume `pgdata` + `pg_dump` manual | `pg_dump` na rotina existente do servidor |

## 2. Ferramentas e para que servem

| Ferramenta | Função | Como entra |
|---|---|---|
| **Go 1.27** | núcleo: servidor, CLI, MCP, rule engine (`go.mod` exige `go 1.27`) | `nix develop` ou instalação local |
| **gopls, gotools, delve** | editor, `goimports`, debugger | dev shell |
| **golangci-lint v2** | lint (`.golangci.yml`, `version: "2"`) | `just lint`; CI usa `golangci-lint-action@v8` |
| **air** | recompila e reinicia `ironman serve` ao salvar (`.air.toml`) | `just dev` usa se estiver no PATH |
| **sqlc** | gera `internal/store/db` a partir de `db/queries/*.sql` + `db/migrations` (`sqlc.yaml`) | `just sqlc` (`go run ...@latest`) |
| **goose** | migrations SQL em `db/migrations` (`-- +goose Up/Down`) | `just migrate-up/down/status/new` |
| **Node 22 + pnpm 10** | SPA Svelte em `web/`, build para `web/dist` (embutido no binário) | `just setup`, `just web-dev`, `just web-build` |
| **PostgreSQL 17** | banco; no dev shell só o cliente (`psql`, `pg_dump`) | `just db-shell` |
| **just** | task runner; `just` lista tudo | `justfile` na raiz |
| **docker compose** | `compose.yaml` (Postgres + app) e `compose.dev.yaml` (só Postgres) | `just up/down/logs`, `just db-up/db-down` |
| **Nix flake** | dev shell reproduzível (`nix develop` / direnv) e `packages.default` do binário | `flake.nix`, `.envrc` |

Tudo que o `justfile` chama via `go run pkg@latest` (goose, sqlc) funciona sem Nix; o flake só evita
instalar cada ferramenta à mão.

## 3. Fluxos

### 3.1 Dev no host (recomendado para programar)

```
cp .env.example .env     # DATABASE_URL já aponta para localhost:5432
just setup               # pnpm install, go mod download, primeiro pnpm build
just dev                 # sobe Postgres (compose.dev.yaml) e roda air / go run ./cmd/ironman serve
just web-dev             # em outro terminal: Vite em :5173 com proxy para :8080
just migrate-up          # aplica db/migrations (se o serve não aplicar sozinho no boot)
```

Sem `just web-dev`, o servidor entrega a última build embutida de `web/dist`.

### 3.2 Stack no zaros (Docker)

É a forma de uso no zaros (decisão do dono, 2026-11-04): Postgres 17, um serviço `migrate` que roda
o goose e sai, e o app (imagem distroless), que só sobe depois da migração.

```
cp .env.example .env                 # POSTGRES_PASSWORD
just up                              # imagens do HEAD commitado, migrate, start
just stack-account-create "<RSN>"    # uma vez: conta acc_<rsn> (modo ironman, rota Oziris)
open http://localhost:8080/?account=acc_<rsn>
just restart                         # reinicia o app (mesma imagem)
just logs                            # logs do app; `just logs migrate` mostra a migração
just db-backup                       # pg_dump em backups/ (fora do git)
just stack-accounts / stack-account-guide <id> <guia> / stack-psql
```

- O app escuta só em `127.0.0.1`: em `AUTH_MODE=dev` não há login. `IRONMAN_BIND=0.0.0.0` no `.env`
  abre para a rede (celular, Tailscale), de propósito.
- O Postgres da stack não tem porta no host, então não conflita com o `compose.dev.yaml`; as receitas
  `stack-*` e o `db-backup` entram pelo `docker compose exec`.
- Healthcheck: `/ironman health`, que só faz GET em `/api/health` e falha se o banco não responde.
- As imagens saem do HEAD commitado (`git archive`), nunca do diretório de trabalho: os catálogos de
  entidades vão embutidos no binário e precisam bater com guias e anotações, e a extração deixa lotes
  em andamento no diretório (decisão do dono, 2026-11-04). Código ou conteúdo novo: commit e
  `just up`.

### 3.3 saradomin

1. Criar database e usuário no Postgres existente; `DATABASE_URL` com `sslmode=require` se aplicável.
2. No Zitadel, criar aplicação Web (PKCE, sem secret) com redirect
   `https://planner.<domínio>/auth/callback`; copiar issuer e client id para `OIDC_*`.
3. `.env` com `AUTH_MODE=oidc`, `SESSION_SECRET` novo (`openssl rand -hex 32`), `PUBLIC_BASE_URL`.
4. Usar só o serviço `app` do `compose.yaml` (remover `postgres` e o `depends_on`) ou declarar o
   serviço no compose do servidor com a mesma imagem. Health em `GET /api/health`.
5. `goose up` com o `DATABASE_URL` do servidor antes de subir a nova versão.

## 4. Variáveis de ambiente

Lidas pelo binário (`ironman serve` é o único comando que abre o banco):

| Variável | Default | Função |
|---|---|---|
| `IRONMAN_ADDR` | `:8080` | endereço de escuta HTTP |
| `DATABASE_URL` | — | `postgres://user:senha@host:5432/db?sslmode=...` |
| `AUTH_MODE` | `dev` | `dev` (usuário local fixo, sem login) ou `oidc` |
| `OIDC_ISSUER` | — | URL do Zitadel (obrigatório em `oidc`) |
| `OIDC_CLIENT_ID` | — | client id da aplicação Web |
| `OIDC_REDIRECT_URL` | — | deve bater com o registrado no Zitadel |
| `SESSION_SECRET` | — | 32+ bytes aleatórios; assina o cookie de sessão |
| `CONTENT_DIR` | `content` | diretório dos guias; `/app/content` na imagem |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PUBLIC_BASE_URL` | — | usada em links e no manifest do plugin Microbot |
| `SYNC_HISCORES_MINUTES` | `30` | cadência do sync do hiscores por conta (jitter de ±5 min); `0` desliga o scheduler, o botão Sincronizar continua |

Só para o compose: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`,
`IRONMAN_PORT`, `VERSION`. O `compose.yaml` monta o `DATABASE_URL` do container com host `postgres`,
então o mesmo `.env` serve para dev no host (localhost) e para o compose.

## 5. CI (`.github/workflows/ci.yml`)

| Job | Quando | O que roda |
|---|---|---|
| `web` | push em `main` e PRs | `pnpm install --frozen-lockfile`, `pnpm check`, `pnpm build`, publica `web/dist` como artefato |
| `go` | depois de `web` | baixa `web/dist`, `go build ./...`, `go vet`, golangci-lint (latest, v2), `go test -race`, `ironman content validate` |
| `migrations` | paralelo | Postgres 17 como service; `goose up`, `status`, `down-to 0`, `up`; `sqlc compile` |
| `docker` | só push em `main` | `docker build` da imagem (sem push), cache GHA |

Go via `go-version-file: go.mod`; cache de módulos pelo `setup-go`, cache do pnpm pelo `setup-node`.

## 6. Troubleshooting

**`nix build` falha com "hash mismatch"**: esperado na primeira vez. `vendorHash` em `flake.nix` está
como `lib.fakeHash`; copiar o valor de `got: sha256-...` para o arquivo. Repetir sempre que `go.sum`
mudar. Se falhar com "pattern all:dist: no matching files", `web/dist` não está na árvore que o Nix vê:
rodar `just web-build` e usar `nix build path:.` (o flake de um checkout git só enxerga arquivos
versionados). O caminho reproduzível para imagem é o `Dockerfile`.

**`pnpm install --frozen-lockfile` falha** (CI ou Docker): `web/pnpm-lock.yaml` está desatualizado em
relação ao `package.json`. Rodar `cd web && pnpm install` e commitar o lockfile. O Dockerfile cai para
`pnpm install` simples só quando o lockfile não existe.

**`pnpm/action-setup` erra "multiple versions"**: o workflow fixa `version: 10` porque
`web/package.json` não tem `packageManager`. Ao adicionar o campo, remover o `version:` do workflow.

**`go build` falha com `pattern all:dist`**: `web/dist` vazio. `just setup` (ou `just web-build`) gera a
primeira build; o `postbuild` do web cria `web/dist/.gitkeep` para o embed compilar.

**Compose: `app` nunca sobe / `dependency failed to start`**: o healthcheck do Postgres usa
`pg_isready -U $POSTGRES_USER -d $POSTGRES_DB`; se `POSTGRES_PASSWORD` não estiver no `.env`, o compose
aborta com `set POSTGRES_PASSWORD in .env`. Volume antigo com outra senha: `just nuke` apaga os volumes.

**App em distroless sem healthcheck**: a imagem não tem shell nem curl. Quando o binário ganhar um
subcomando `health`, descomentar o bloco em `compose.yaml`.

**`golangci-lint` local reclama "Go language version lower than targeted"**: o binário local foi
compilado com Go anterior a 1.27. Atualizar (`nix develop` traz o par certo) ou usar a versão `latest`
como o CI.

**`goose` lento na primeira vez**: `go run ...@latest` compila o CLI com todos os drivers. É só na
primeira execução; o cache de build do Go reaproveita depois.

**Porta 5432 ocupada**: mudar `POSTGRES_PORT` no `.env`; o `compose.dev.yaml` e o `DATABASE_URL`
leem da mesma variável (ajustar a URL).
