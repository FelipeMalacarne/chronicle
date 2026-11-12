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
| **air** | recompila e reinicia `chronicle serve` ao salvar (`.air.toml`) | `just dev` usa se estiver no PATH |
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

- O app escuta só em `127.0.0.1`: em `AUTH_MODE=dev` não há login. `CHRONICLE_BIND=0.0.0.0` no `.env`
  abre para a rede (celular, Tailscale), de propósito.
- O Postgres da stack publica só em `127.0.0.1:${POSTGRES_PORT}`, para o `just dev` usar o mesmo banco;
  as receitas `stack-*` e o `db-backup` entram pelo `docker compose exec`.
- Healthcheck: `/chronicle health`, que só faz GET em `/api/health` e falha se o banco não responde.
- A imagem (uma só: `chronicle serve` no `app`, `chronicle migrate` no `migrate`) sai do HEAD commitado (`git archive`), nunca do diretório de trabalho: os catálogos de
  entidades vão embutidos no binário e precisam bater com guias e anotações, e a extração deixa lotes
  em andamento no diretório (decisão do dono, 2026-11-04). Código ou conteúdo novo: commit e
  `just up`.

### 3.3 saradomin

O saradomin é o cluster k3s de casa, com Argo CD, CloudNativePG, Traefik e o Zitadel em
`auth.ftm.dev.br`; os manifests do cluster ficam no repositório `gielinor`
(`clusters/saradomin`). O Chronicle roda em `https://chronicle.saradomin.ftm.dev.br`, só pela tailnet,
como os outros `*.saradomin.ftm.dev.br` (certificado curinga do cert-manager, entrypoint `websecure`
do Traefik). O Zitadel é público, então o login sai do tailnet e volta.

**Neste repositório** (`deploy/k8s`): `base` com o Deployment (`chronicle migrate` num initContainer da
mesma imagem, o servidor com `/api/health` de readiness e `/api/version` de liveness, sem disco) e o
Service; `overlays/saradomin` com o Ingress, a config de `oidc` e a tag da imagem. O CI, a cada push na
`main`, publica `ghcr.io/felipemalacarne/chronicle:<sha>` (pacote público, como os dos outros apps do
saradomin; o repositório continua privado) e grava o sha no overlay (`deploy: saradomin roda <sha>`); o Argo segue o overlay na `main`, então esse
commit é o deploy. `secret.example.yaml` lista as chaves do Secret.

**Duas imagens.** O mesmo job publica também `ghcr.io/felipemalacarne/chronicle-worker:<sha>`, a dos
workers do Microbot (doc 11, seções 7.2 e 13.4): a `microbot-worker-base` do `microbot-plugins`,
fixada por digest em `deploy/worker/base.lock`, mais o binário do mesmo commit
(`deploy/worker/Dockerfile`, com `chronicle agent --worker`). No commit de deploy, o CI grava também
`WORKER_IMAGE` no overlay. Com o `base.lock` vazio, o job publica só o servidor e deixa um aviso. A
base muda por PR: quando o CI do microbot-plugins publica uma base nova, ele abre (ou atualiza) o PR
`bump/microbot-worker-base` aqui com o digest no `base.lock` (segredo `CHRONICLE_BUMP_TOKEN` de lá, um
token fine-grained só deste repositório, com Contents e Pull requests de escrita), e o merge leva a nova
base. À mão, `just worker-base-bump <tag>` resolve o digest da tag no GHCR e o grava no `base.lock`. O Deployment do servidor roda com a SA
`chronicle`, que o gielinor cria junto com o RBAC, a política de admissão, a quota, a NetworkPolicy e
o PVC dos workers (`apps/chronicle/README.md` de lá); os workers ficam desligados (`WORKERS=off`) até
o `CHRONICLE_VAULT_KEY` entrar no `chronicle-secrets`.

**No gielinor** (`clusters/saradomin`): o namespace `chronicle`; o role `chronicle` e o `Database`
`chronicle` do `postgres-main` (`apps/postgres`); e `apps/chronicle` com a `Application` do Argo, a
deploy key de leitura do repositório (privado) para o Argo e o `chronicle-secrets`, cifrados com SOPS.

Uma vez:

1. **Zitadel** (`https://auth.ftm.dev.br/ui/console`), um **projeto** "Chronicle":
   - em General, ligar **Assert Roles on Authentication**;
   - em Roles, criar `owner` (é o `OIDC_REQUIRED_ROLE`) e as do plugin: `ingest`, `items:bank`,
     `items:inventory`, `items:equipment`, `marks:write`;
   - em Authorizations, dar todas ao usuário do dono;
   - aplicação **Web**, método **PKCE** (sem secret): Redirect URI
     `https://chronicle.saradomin.ftm.dev.br/auth/callback`, Post Logout URI
     `https://chronicle.saradomin.ftm.dev.br/`, Development Mode desligado; em Token Settings,
     **User roles inside ID Token** e **User Info inside ID Token**. O Client ID vai em `OIDC_CLIENT_ID`
     (não o id da aplicação, que é outro número). No Zitadel do saradomin, as roles só vieram no ID
     token depois de ligar "User roles inside ID Token" também nas configurações do projeto: sem
     isso o login cai em "Acesso negado" com `roles=""` no log;
   - aplicação **Native**, grants **Device Code** e **Refresh Token**; em Token Settings, **Auth Token
     Type: JWT** e **User roles inside Access Token** (o servidor lê as roles do access token, não do
     userinfo; sem isso o plugin leva 403 e a CLI fica sem `owner`). O Client
     ID vai em `OIDC_NATIVE_CLIENT_ID` (CLI, `chronicle mcp` e o plugin);
   - o id do projeto vai em `OIDC_PROJECT_ID`; o id do usuário do dono (Users, 18 dígitos) em
     `OIDC_ALLOWED_SUBJECTS`.
2. **GHCR**: depois do primeiro push, o pacote `chronicle` em Public (Package settings → Change
   visibility): o k3s baixa sem login.
3. **Segredos** no gielinor: `chronicle-secrets` com `DATABASE_URL` (a senha gerada para o role
   `chronicle`), `SESSION_SECRET` (`openssl rand -hex 32`) e os quatro valores do passo 1.
4. **Banco do zaros**: depois de o `Database` existir e antes de o app subir, `pg_dump --no-owner
   --no-acl` do banco do zaros restaurado no `chronicle` como o role `chronicle`. O dump traz a
   `goose_db_version`, então a migrate do initContainer só aplica o que faltar.
5. **Ligar**: descomentar `apps/chronicle` em `clusters/saradomin/kustomization.yaml` (merge na `main`
   do gielinor). O `saradomin-root` cria a `Application`, e ela sobe o app.
6. **Primeiro login**: o dono entra pela web antes de qualquer outro usuário; o primeiro usuário OIDC
   recebe as contas do usuário dev (log `first OIDC login claimed the dev accounts`; doc 04, seção 2.5).
7. **Plugin**: Server URL `https://chronicle.saradomin.ftm.dev.br`, Credential "Login (Chronicle
   panel)" e Entrar no painel (doc 06, adendo de 2026-11-11). **CLI**: `chronicle login --url
   https://chronicle.saradomin.ftm.dev.br`.
8. Conferir: `/` sem sessão leva ao Zitadel e volta; `GET /api/me` mostra as contas; o painel do plugin
   mostra a rota; `kubectl --context saradomin -n chronicle get pods` pronto.

O controle do Microbot (`MICROBOT_CONTROL=on`) está ligado no saradomin: o agent do zaros entra com a
conta de máquina do Zitadel (role `microbot:agent`) e o Cockpit pede `microbot:control` (doc 08).

### 3.4 O `chronicle` instalado na estação (CLI, MCP e agent)

O flake do repositório expõe o binário como pacote (`packages.<system>.chronicle`) e como overlay
(`overlays.default`, que dá `pkgs.chronicle`). É o que o nix-config referencia para ter `chronicle
login`, `accounts connect`, `workers`, `mcp` e `agent` no PATH:

```nix
# flake.nix do nix-config
inputs.chronicle.url = "github:FelipeMalacarne/chronicle";
# home-manager
home.packages = [ inputs.chronicle.packages.${pkgs.system}.chronicle ];
```

O repositório é público (decisão 16 do doc 01), então nada pede credencial. Sem nix-config:
`nix profile install github:FelipeMalacarne/chronicle`. O pacote
leva o placeholder de `web/dist` (o flake só vê arquivos rastreados), então serve para CLI, MCP e
agent; o servidor com a web continua sendo a imagem Docker. O `vendorHash` do `flake.nix` muda quando
o `go.sum` muda: rode `nix build .#chronicle` e cole o `got:` do erro.

## 4. Variáveis de ambiente

Lidas pelo binário (`chronicle serve` é o único comando que abre o banco):

| Variável | Default | Função |
|---|---|---|
| `CHRONICLE_ADDR` | `:8080` | endereço de escuta HTTP |
| `DATABASE_URL` | — | `postgres://user:senha@host:5432/db?sslmode=...` |
| `AUTH_MODE` | `dev` | `dev` (usuário local fixo, sem login) ou `oidc` |
| `OIDC_ISSUER` | — | URL do Zitadel (obrigatório em `oidc`) |
| `OIDC_CLIENT_ID` | — | client id da aplicação Web (PKCE, sem secret) |
| `OIDC_REDIRECT_URL` | `PUBLIC_BASE_URL` + `/auth/callback` em `oidc` | deve bater com o registrado no Zitadel |
| `OIDC_ALLOWED_SUBJECTS` | — | `sub` do Zitadel aceitos, separados por vírgula; em `oidc`, este e/ou `OIDC_REQUIRED_ROLE` (com os dois configurados, o login precisa dos dois) |
| `OIDC_REQUIRED_ROLE` | — | role do projeto no Zitadel que dá acesso (claims `urn:zitadel:iam:org:project[:<id>]:roles`) |
| `OIDC_PROJECT_ID` | — | id do projeto do planner no Zitadel: só a claim `urn:zitadel:iam:org:project:<id>:roles` dele conta; vazio aceita qualquer projeto |
| `SESSION_SECRET` | — | 32+ caracteres aleatórios (obrigatório em `oidc`); cifra o cookie do estado do login (state, nonce, PKCE); a sessão é um token aleatório na tabela `sessions` |
| `CONTENT_DIR` | `content` | diretório dos guias; `/app/content` na imagem |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PUBLIC_BASE_URL` | — | usada em links e no manifest do plugin Microbot; em `oidc` é obrigatória, dá o redirect padrão e a volta do logout, e com https o cookie de sessão ganha `Secure` |
| `SYNC_HISCORES_MINUTES` | `30` | cadência do sync do hiscores por conta (jitter de ±5 min); `0` desliga o scheduler, o botão Sincronizar continua |

Só para o compose: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`,
`CHRONICLE_PORT`, `VERSION`. O `compose.yaml` monta o `DATABASE_URL` do container com host `postgres`,
então o mesmo `.env` serve para dev no host (localhost) e para o compose.

## 5. CI (`.github/workflows/ci.yml`)

| Job | Quando | O que roda |
|---|---|---|
| `web` | push em `main` e PRs | `pnpm install --frozen-lockfile`, `pnpm check`, `pnpm build`, publica `web/dist` como artefato |
| `go` | depois de `web` | baixa `web/dist`, `go build ./...`, `go vet`, golangci-lint (latest, v2), `go test -race`, `chronicle content validate` |
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
