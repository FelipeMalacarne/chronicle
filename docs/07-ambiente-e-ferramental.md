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
| Banco | `postgres:17-alpine` do `compose.yaml` (o mesmo banco para a stack e para o dev no host) | Postgres existente; database `ironman_planner` |
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
| **docker compose** | `compose.yaml` (Postgres + app; o `just dev` usa só o Postgres) | `just up/down/logs`, `just db-up/db-down` |
| **Nix flake** | dev shell reproduzível (`nix develop` / direnv) e `packages.default` do binário | `flake.nix`, `.envrc` |

Tudo que o `justfile` chama via `go run pkg@latest` (goose, sqlc) funciona sem Nix; o flake só evita
instalar cada ferramenta à mão.

## 3. Fluxos

### 3.1 Dev no host (recomendado para programar)

```
cp .env.example .env     # DATABASE_URL já aponta para localhost:5432
just setup               # pnpm install, go mod download, primeiro pnpm build
just dev                 # Postgres da stack, migrate-up, air (:8080) e Vite (:5173) juntos; Ctrl-C para os dois
```

Abrir `http://localhost:5173` (hot reload do Svelte; o air recompila Go e JSON de conteúdo). O `:8080`
entrega a build embutida de `web/dist`, que o `just dev` gera se faltar. O banco é o mesmo da stack
(decisão do dono, 2026-11-04): o `just dev` para o container `app` para liberar a porta 8080 e o
`just up` o traz de volta. As migrations do diretório de trabalho entram nesse banco; uma migration
ainda não commitada fica aplicada nele.

### 3.2 Stack no zaros (Docker)

É a forma de uso no zaros (decisão do dono, 2026-11-04): Postgres 17, um serviço `migrate` que roda
o goose e sai, e o app (imagem distroless), que só sobe depois da migração.

```
cp .env.example .env                 # POSTGRES_PASSWORD
just up                              # imagens do HEAD commitado, migrate, start
open http://localhost:8080              # primeiro acesso em /setup: RSN, modo detectado, rota
just stack-account-create "<RSN>"    # alternativa sem a tela (modo ironman, rota Oziris)
just restart                         # reinicia o app (mesma imagem)
just logs                            # logs do app; `just logs migrate` mostra a migração
just db-backup                       # pg_dump em backups/ (fora do git)
just stack-accounts / stack-account-guide <id> <guia> / stack-psql
```

- O app escuta só em `127.0.0.1`: em `AUTH_MODE=dev` não há login. `IRONMAN_BIND=0.0.0.0` no `.env`
  abre para a rede (celular, Tailscale), de propósito.
- O Postgres da stack publica só em `127.0.0.1:${POSTGRES_PORT}`, para o `just dev` usar o mesmo banco;
  as receitas `stack-*` e o `db-backup` entram pelo `docker compose exec`.
- Healthcheck: `/ironman health`, que só faz GET em `/api/health` e falha se o banco não responde.
- As imagens saem do HEAD commitado (`git archive`), nunca do diretório de trabalho: os catálogos de
  entidades vão embutidos no binário e precisam bater com guias e anotações, e a extração deixa lotes
  em andamento no diretório (decisão do dono, 2026-11-04). Código ou conteúdo novo: commit e
  `just up`.

### 3.3 saradomin

A sessão web por OIDC está pronta (doc 04, seção 2.5); falta só o deploy.

1. Criar database e usuário no Postgres existente; `DATABASE_URL` com `sslmode=require` se aplicável.
   Para levar o progresso do zaros: `just db-backup` lá e `pg_restore`/`psql` aqui, antes do primeiro
   login (ver o passo 7).
2. No console do Zitadel, **projeto** "Ironman Planner" (Projects → New):
   - em General, ligar **Assert Roles on Authentication** (as roles do projeto vão para o userinfo);
     opcional: **Check authorization on Authentication** (o próprio Zitadel barra quem não tem role
     no projeto);
   - em Roles, criar a role `owner` (é o `OIDC_REQUIRED_ROLE`; outro nome serve, desde que bata);
   - em Authorizations, dar `owner` ao usuário do dono.
3. No mesmo projeto, **aplicação** Web (Applications → New → Web), método de autenticação **PKCE**
   (sem client secret):
   - Redirect URIs: `https://planner.<domínio>/auth/callback`;
   - Post Logout URIs: `https://planner.<domínio>/`;
   - Development Mode desligado (ligar só para testar com `http://localhost:8080/auth/callback`);
   - em Token Settings, ligar **User roles inside ID Token** e **User Info inside ID Token** (sem eles
     o planner busca roles e perfil no userinfo, uma chamada a mais no login);
   - copiar o **Client ID** para `OIDC_CLIENT_ID`. O issuer é o domínio da instância
     (`https://auth.<domínio>`; conferir em `/.well-known/openid-configuration`).
   - O `sub` do dono é o **ID** do usuário em Users (número de 18 dígitos), para
     `OIDC_ALLOWED_SUBJECTS`.
   Para o plugin (decisão 28 do doc 01, item 15 do doc 04): criar também uma aplicação **nativa**
   pública com o grant Device Code, e as roles do projeto `ingest`, `items:bank`, `items:inventory`,
   `items:equipment` e `marks:write`, atribuídas ao usuário do dono; o access token deve ser JWT com as
   roles no token. O planner informa o `client_id` dela ao plugin, que só precisa da URL.
4. `.env` com `AUTH_MODE=oidc`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `PUBLIC_BASE_URL=https://planner.<domínio>`
   (https liga o `Secure` do cookie; `OIDC_REDIRECT_URL` pode ficar vazio e vira
   `PUBLIC_BASE_URL/auth/callback`), `SESSION_SECRET` novo (`openssl rand -hex 32`; o valor do
   `.env.example` é recusado) e pelo menos um de `OIDC_ALLOWED_SUBJECTS` e `OIDC_REQUIRED_ROLE` (basta
   passar em um). Sem nenhum dos dois, ou sem `DATABASE_URL`, o `serve` não sobe.
5. Usar só o serviço `app` do `compose.yaml` (remover `postgres` e o `depends_on`) ou declarar o
   serviço no compose do servidor com a mesma imagem. Health em `GET /api/health`. O proxy reverso
   termina o TLS e repassa cookies; para o SSE (`/api/events`), sem buffer e sem timeout curto de
   leitura (o servidor já manda `X-Accel-Buffering: no`).
6. `goose up` com o `DATABASE_URL` do servidor antes de subir a nova versão (a tabela `sessions` já
   vem da migration 0001).
7. Primeiro login: o primeiro usuário OIDC do banco recebe todas as contas do usuário dev (regra no
   doc 04, seção 2.5; o log diz `first OIDC login claimed the dev accounts`). Por isso o dono entra
   antes de qualquer outro usuário. Os tokens `ipl_` do plugin continuam valendo até o item 15.
8. Conferir: `/` sem sessão leva ao Zitadel e volta à mesma tela; `GET /api/me` mostra as contas;
   um usuário sem a role vê "Acesso negado".

## 4. Variáveis de ambiente

Lidas pelo binário (`ironman serve` é o único comando que abre o banco):

| Variável | Default | Função |
|---|---|---|
| `IRONMAN_ADDR` | `:8080` | endereço de escuta HTTP |
| `DATABASE_URL` | — | `postgres://user:senha@host:5432/db?sslmode=...` |
| `AUTH_MODE` | `dev` | `dev` (usuário local fixo, sem login) ou `oidc` |
| `OIDC_ISSUER` | — | URL do Zitadel (obrigatório em `oidc`) |
| `OIDC_CLIENT_ID` | — | client id da aplicação Web (PKCE, sem secret) |
| `OIDC_REDIRECT_URL` | `PUBLIC_BASE_URL` + `/auth/callback` em `oidc` | deve bater com o registrado no Zitadel |
| `OIDC_ALLOWED_SUBJECTS` | — | `sub` do Zitadel aceitos, separados por vírgula; em `oidc`, este ou `OIDC_REQUIRED_ROLE` (basta passar em um) |
| `OIDC_REQUIRED_ROLE` | — | role do projeto no Zitadel que dá acesso (claims `urn:zitadel:iam:org:project[:<id>]:roles`) |
| `SESSION_SECRET` | — | 32+ caracteres aleatórios (obrigatório em `oidc`); cifra o cookie do estado do login (state, nonce, PKCE); a sessão é um token aleatório na tabela `sessions` |
| `CONTENT_DIR` | `content` | diretório dos guias; `/app/content` na imagem |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PUBLIC_BASE_URL` | — | usada em links e no manifest do plugin Microbot; em `oidc` é obrigatória, dá o redirect padrão e a volta do logout, e com https o cookie de sessão ganha `Secure` |
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

**Porta 5432 ocupada**: mudar `POSTGRES_PORT` no `.env`; o `compose.yaml` lê essa variável e o
`DATABASE_URL` precisa da mesma porta (ajustar a URL).
