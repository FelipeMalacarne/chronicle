# ironman-planner

Planner e tracker self-hosted para conta Ironman de OSRS. Segue duas rotas independentes e comparáveis:
o Oziris Ironman Efficiency Guide (texto de OzirisLoL, versão interativa mantida por ironman.guide) e o
BRUHsailer (So Iron BRUH e ParasailerOSRS). O progresso é rastreado automaticamente via hiscores, Wise
Old Man, um plugin próprio no Microbot (RuneLite) e dados estruturados do OSRS Wiki.

## Estado

- **Na `main`**:
  - conteúdo multi-guia (802 passos), segmentos de banco e entidades canônicas;
  - ingest v1 do plugin com frescor por container;
  - readiness do próximo trecho;
  - API por guia;
  - web `/plan` e `/compare` sobre a API real.
- **Plugin Microbot** 1.0.0 (somente leitura) no repositório `microbot-plugins`, ainda não testado no jogo.
- **Modelo de domínio aprovado** (`docs/09-modelo-de-dominio.md`): o objetivo é a unidade rastreável,
  com progresso compartilhado entre rotas; a API de progresso é construída sobre ele.
- **Estado detalhado e sequência vigente**: `docs/04-arquitetura.md`, seção 9. Decisões: `docs/01`.

Roda primeiro na estação do dono (local-first, `docker compose`, `AUTH_MODE=dev`). O deploy no
`saradomin` usa a mesma imagem com outras variáveis de ambiente.

## Como rodar (local-first)

Pré-requisitos: Docker com compose e, ou `nix develop` (traz todo o resto), ou Go 1.27, Node 22 com
pnpm 10 e `just` instalados.

```sh
nix develop              # ou: direnv allow (usa .envrc); ou instale as ferramentas acima
cp .env.example .env     # defaults já funcionam para dev local
just setup               # pnpm install, go mod download, primeiro build do web
just dev                 # Postgres da stack, migrations, API (air) e Vite juntos, com hot reload
# abra http://localhost:5173
```

Opcional: `just migrate-up` para aplicar migrations à mão, `just up` para a stack completa prod-like
(Postgres + imagem distroless), que usa o mesmo banco do `just dev`.
`just` sem argumentos lista todos os alvos.

## Ferramental

| Ferramenta | Para quê | Onde |
|---|---|---|
| Go 1.27 + golangci-lint v2 | servidor, CLI e MCP num binário; lint | `cmd/`, `internal/`, `.golangci.yml` |
| sqlc + goose + pgx | queries tipadas e migrations no Postgres 17 | `db/`, `sqlc.yaml` |
| Node 22 + pnpm 10 + Svelte/Vite | SPA estática embutida no binário | `web/` |
| just | task runner (`just` lista) | `justfile` |
| docker compose | Postgres local e stack prod-like | `compose.yaml` |
| Dockerfile multi-stage | imagem distroless não-root | `Dockerfile`, `.dockerignore` |
| Nix flake + direnv | dev shell reproduzível, `nix build` do binário | `flake.nix`, `.envrc` |
| GitHub Actions | web, go, migrations, docker build | `.github/workflows/ci.yml` |
| air | live reload de `chronicle serve` | `.air.toml` |

Detalhes, variáveis de ambiente e troubleshooting em `docs/07-ambiente-e-ferramental.md`.

## Deploy no saradomin

Mesma imagem, `.env` diferente: `DATABASE_URL` do Postgres existente, `AUTH_MODE=oidc` com
`OIDC_ISSUER`, `OIDC_CLIENT_ID` e `OIDC_REDIRECT_URL` do Zitadel, `SESSION_SECRET` novo e
`PUBLIC_BASE_URL` atrás do reverse proxy. Decisões em `docs/04-arquitetura.md` (seções 1 e 8) e passo a
passo em `docs/07-ambiente-e-ferramental.md`, seção 3.3.

## Documentos

| Arquivo | Conteúdo |
|---|---|
| `docs/00-levantamento-inicial.md` | Levantamento inicial (histórico, superado pelos docs 02 a 04) |
| `docs/01-sumario-executivo.md` | Sumário executivo e registro de decisões (abertas e resolvidas) |
| `docs/02-fontes-de-dados.md` | APIs verificadas na prática, formatos reais, estratégia de sync |
| `docs/03-produto-e-features.md` | Jobs-to-be-done, catálogo de features, UX tela a tela |
| `docs/04-arquitetura.md` | Arquitetura vigente em Go: componentes, conteúdo, banco, grafo, ingest, estado e sequência (seção 9) |
| `docs/05-conteudo-e-normalizacao.md` | O que foi extraído do guia e como vira regras de auto-complete |
| `docs/06-cliente-microbot.md` | Plugin de leitura no Microbot (fork do RuneLite) como fonte principal de quests, diaries e itens |
| `docs/07-ambiente-e-ferramental.md` | Ferramentas, modos local-first e saradomin, variáveis de ambiente, CI e troubleshooting |
| `docs/08-controle-microbot.md` | Controle do cliente Microbot pelo planner: agent no zaros, VNC, plugins, rotinas |
| `docs/09-modelo-de-dominio.md` | Objetivos, entidades e progresso compartilhado; extração dos guias por agente (aprovado; decisões em 9.1 e 9.3) |
| `docs/ui/brief-ui-ux.md` | Brief de UI/UX para o agente de design |
| `docs/arquitetura/types.ts` | Contrato de ingest e tipos de readiness em Zod, só como referência para clientes TS |
| `docs/arquitetura/modelo-de-dados.dbml` | Modelo do banco em DBML (abre no dbdiagram.io): atual, gerado por `just dbml`, mais as tabelas planejadas do doc 09 |

## Design

Telas v0 no Claude Design (canvas privado do dono): Agora, Checklist da seção, Conta, Grafo de requisitos, Meta e versão celular, usando o design system Senntisten Shell (Catppuccin Mocha).

## Dados

- `content/guides/` conteúdo canônico: `oziris-main` (7 seções, 576 passos) e `bruhsailer-main` (9 seções, 226 passos), com README de proveniência e relatório de entidades.
- `content/reference/` catálogos de entidades (quests, itens, skills, diaries, combat achievements).
- `data/guide/` guia do Oziris extraído e versão normalizada em `data/guide/normalized/` (entrada do `content migrate`).
- `data/sources/bruhsailer/` DOCX pinados do BRUHsailer com checksums (entrada do `content import-bruhsailer`).
- `data/raw/ironman-guide/` payloads brutos e páginas companheiras em Markdown.
- `data/wiki/` dados estruturados do OSRS Wiki (quests, requisitos, diaries).
- `data/samples/` respostas de exemplo de cada API.

## Atribuição

O Oziris pertence a OzirisLoL e ironman.guide; o BRUHsailer, a So Iron BRUH e ParasailerOSRS. Nenhum dos dois declara licença: uso pessoal e self-hosted, sem redistribuição pública, mantendo a atribuição em qualquer tela que exiba os passos.
