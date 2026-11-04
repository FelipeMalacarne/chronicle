# ironman-planner

Planner e tracker self-hosted para conta Ironman de OSRS, baseado no Oziris Ironman Efficiency Guide
(texto de OzirisLoL, versão interativa mantida por ironman.guide), com tracking automático de progresso
via hiscores, Wise Old Man, WikiSync/plugin RuneLite e dados estruturados do OSRS Wiki.

## Estado

Descoberta e design concluídos (docs 00 a 07). Implementação do MVP em andamento seguindo
`docs/04-arquitetura.md`, seção 9.5 (núcleo em Go, SPA Svelte embutida, Postgres, Zitadel depois).
Roda primeiro na estação do dono (local-first, `docker compose`, `AUTH_MODE=dev`); o deploy no
`saradomin` usa a mesma imagem com outras variáveis de ambiente.

## Como rodar (local-first)

Pré-requisitos: Docker com compose e, ou `nix develop` (traz todo o resto), ou Go 1.27, Node 22 com
pnpm 10 e `just` instalados.

```sh
nix develop              # ou: direnv allow (usa .envrc); ou instale as ferramentas acima
cp .env.example .env     # defaults já funcionam para dev local
just setup               # pnpm install, go mod download, primeiro build do web
just dev                 # sobe Postgres (compose.dev.yaml) e roda a API com live reload
# abra http://localhost:8080
```

Opcional: `just web-dev` em outro terminal (Vite com hot reload em :5173), `just migrate-up` para
aplicar migrations à mão, `just up` para a stack completa prod-like (Postgres + imagem distroless).
`just` sem argumentos lista todos os alvos.

## Ferramental

| Ferramenta | Para quê | Onde |
|---|---|---|
| Go 1.27 + golangci-lint v2 | servidor, CLI e MCP num binário; lint | `cmd/`, `internal/`, `.golangci.yml` |
| sqlc + goose + pgx | queries tipadas e migrations no Postgres 17 | `db/`, `sqlc.yaml` |
| Node 22 + pnpm 10 + Svelte/Vite | SPA estática embutida no binário | `web/` |
| just | task runner (`just` lista) | `justfile` |
| docker compose | Postgres local e stack prod-like | `compose.yaml`, `compose.dev.yaml` |
| Dockerfile multi-stage | imagem distroless não-root | `Dockerfile`, `.dockerignore` |
| Nix flake + direnv | dev shell reproduzível, `nix build` do binário | `flake.nix`, `.envrc` |
| GitHub Actions | web, go, migrations, docker build | `.github/workflows/ci.yml` |
| air | live reload de `ironman serve` | `.air.toml` |

Detalhes, variáveis de ambiente e troubleshooting em `docs/07-ambiente-e-ferramental.md`.

## Deploy no saradomin

Mesma imagem, `.env` diferente: `DATABASE_URL` do Postgres existente, `AUTH_MODE=oidc` com
`OIDC_ISSUER`, `OIDC_CLIENT_ID` e `OIDC_REDIRECT_URL` do Zitadel, `SESSION_SECRET` novo e
`PUBLIC_BASE_URL` atrás do reverse proxy. Decisões em `docs/04-arquitetura.md` (seções 8 e 9) e passo a
passo em `docs/07-ambiente-e-ferramental.md`, seção 3.3.

## Documentos

| Arquivo | Conteúdo |
|---|---|
| `docs/00-levantamento-inicial.md` | Levantamento inicial: fontes, modelo de dados, fases |
| `docs/01-sumario-executivo.md` | Consolidação da rodada de design, números e decisões pendentes |
| `docs/02-fontes-de-dados.md` | APIs verificadas na prática, formatos reais, estratégia de sync |
| `docs/03-produto-e-features.md` | Jobs-to-be-done, catálogo de features, UX tela a tela |
| `docs/04-arquitetura.md` | Stack, componentes, schema de conteúdo, banco, grafo, plugin |
| `docs/05-conteudo-e-normalizacao.md` | O que foi extraído do guia e como vira regras de auto-complete |
| `docs/06-cliente-microbot.md` | Plugin de leitura no Microbot (fork do RuneLite) como fonte principal de quests e diaries |
| `docs/07-ambiente-e-ferramental.md` | Ferramentas, modos local-first e saradomin, variáveis de ambiente, CI e troubleshooting |

## Design

Telas v0 no Claude Design (canvas privado do dono): Agora, Checklist da seção, Conta, Grafo de requisitos, Meta e versão celular, usando o design system Senntisten Shell (Catppuccin Mocha).

## Dados

- `data/guide/` guia principal extraído (7 seções, 576 passos) e versão normalizada em `data/guide/normalized/`.
- `data/raw/ironman-guide/` payloads brutos e páginas companheiras em Markdown.
- `data/wiki/` dados estruturados do OSRS Wiki (quests, requisitos, diaries).
- `data/samples/` respostas de exemplo de cada API.

## Atribuição

O conteúdo do guia pertence a OzirisLoL e ironman.guide. Uso pessoal; manter atribuição em qualquer tela que exiba os passos.
