# Sumário executivo — rodada de descoberta e design

Data: 2026-11-03. Consolida os documentos 00 a 05, produzidos em paralelo por quatro frentes (fontes de dados, produto, arquitetura, conteúdo).

## O que já existe no repositório

| Ativo | Onde | Estado |
|---|---|---|
| Guia Oziris completo, 7 seções, 576 steps, com metadados (quest, skillGoal, items, location, layer enhanced) | `data/guide/*.json` | extraído do payload do ironman.guide |
| Guia normalizado com regras `complete_when` e prerequisites | `data/guide/normalized/guide.json`, `stats.json` | gerado por `tools/normalize_guide.py`, validado sem erros |
| Ordem de quests do site (215 quests, 6 fases, 60 marcos) | `data/guide/normalized/quest-order.json` | gerado |
| 30 páginas companheiras do site em Markdown (gear, bosses, hcim, uim, gim, money, afk, 18 artigos) | `data/raw/ironman-guide/pages/` | extraído |
| 225 quests do wiki com requisitos de skill e quest, QP, série, aliases para WikiSync | `data/wiki/quests.json` | gerado por `data/wiki/scripts/build_quests.py` |
| 492 tasks de achievement diary por área e tier | `data/wiki/diary_tasks.json` | gerado |
| 124 respostas reais de API (hiscores, WOM, WikiSync, Bucket, chisel, TempleOSRS, RuneLite) | `data/samples/` | salvas |
| Schema de conteúdo (JSON Schema), DDL SQLite, tipos TS/Zod | `docs/arquitetura/` | rascunho compilado e validado |
| Guia BRUHsailer, rota independente (9 seções, 226 passos) | `content/guides/bruhsailer-main/` | importado dos DOCX pinados (2026-11-04) |
| Segmentos de banco, loadouts e entidades canônicas dos dois guias | `content/guides/*`, `content/reference/` | gerados e validados |
| Núcleo Go: conteúdo, regras, ingest v1, readiness; API por guia; web `/plan` e `/compare` | `internal/`, `web/` | na `main`, testado (doc 04, seção 11) |
| Plugin Microbot Ironman Planner 1.0.0 | repositório `microbot-plugins` | testado em JUnit, ainda não rodou no jogo |

## Números que importam

Cobertura de auto-tracking medida sobre os 576 steps (fonte: doc 05):

| Via | Steps | % |
|---|---|---|
| Hiscores (skill_level) | 50 | 9% |
| WikiSync ou plugin próprio (quest_state, diary_tier) | 159 | 28% |
| Manual | 367 | 64% |

A regra de cascata (concluir um step posterior implica os anteriores da mesma sequência, estado `implied`) eleva a cobertura prática para perto de 90%, mas precisa ser visualmente distinta e confirmável. O doc 03 traz estimativas um pouco mais otimistas; os números medidos acima prevalecem.

No BRUHsailer a cobertura automática hoje é **zero**: 226 passos longos e nenhuma regra. É isso que motiva o modelo de objetivos e a extração por agente do doc 09.

Quests são o sinal dominante e dependem do cliente. Sem plugin próprio o projeto fica refém do WikiSync, cuja API o wiki pede explicitamente para não usar em projetos de terceiros.

## Decisões já tomadas (pela equipe de design, revisáveis)

- Stack: **Go** como núcleo (um binário: `serve`, CLI, servidor MCP) + Postgres (pgx/sqlc) + SPA Svelte embutida, auth por OIDC no Zitadel, deploy no `saradomin`, Cytoscape.js + ELK para o grafo. Ver doc 04, seção 9. A opção SvelteKit full-stack fica registrada como alternativa.
- IDs dos steps do ironman.guide preservados; progresso nunca tem FK para conteúdo, para sobreviver a edições do guia.
- Snapshots brutos append-only mais tabela `player_state` mesclada com prioridade plugin > wikisync > hiscores > wom.
- Rule engine puro com vereditos met / unmet / unknown e evidência; auto_done é pegajoso para regras monotônicas; manual nunca é sobrescrito pelo motor.
- Skills entram no grafo como limiar de aresta, não como nó.
- Protocolo do plugin RuneLite clona o do WikiSync: manifest servido pelo nosso servidor, deltas a cada 10 s, token por cliente.

## Decisões de 2026-11-04 (dono)

- BRUHsailer entra como **rota independente**, nunca como camada do Oziris; comparação em faixas paralelas ligadas só por marcos compartilhados.
- **Progresso compartilhado entre rotas**: o objetivo (entidade canônica + estado-alvo) é a unidade rastreável; marca manual e posição ficam por guia (doc 09).
- **Extração estruturada dos textos dos guias por agente no Claude Code**, acompanhada pelo dono, via MCP stdio local (`ironman content mcp`) e uma skill. É uma operação única: o resultado é JSON commitado e verificado por código determinístico, e a app sobe com esse conteúdo (doc 09, seção 6).

## Decisões que ficam com o dono

1. Resolvida: Go no backend com CLI e MCP no mesmo binário, Svelte só no front (doc 04, seção 9).
2. Resolvida: o plugin próprio será feito no Microbot (fork do RuneLite que o dono já usa) e entra no MVP; WikiSync vira fallback opcional. Ver `docs/06-cliente-microbot.md`.
3. Cascata `implied` já no MVP (recomendado) ou na v1.
4. Grafo visual na v1 junto com a lista em árvore, ou só a lista primeiro.
5. Modos iniciais: ironman + HCIM; UIM e GIM ficam fora até alguém pedir.
6. Mobile como PWA instalável (recomendado).
7. Metas pré-definidas iniciais: títulos das 7 seções, Barrows gloves, Fire cape, Quest cape, Bowfa.
8. Repositório público ou privado. Público exige resolver a licença do texto do Oziris/ironman.guide antes.
9. Nome da conta ironman para testar o sync real desde a primeira sessão de código.
10. Resolvida: o dono acompanha a extração e o agente o consulta sempre que precisar; só entra no conteúdo o que for explícito ou confirmado pelo dono (doc 09, seção 6.4).
11. Tipos `unlock` e `location` já nesta revisão do modelo ou depois do MVP? (doc 09, 9.2)

## Modo de execução

Local-first: o MVP roda no `zaros` com Docker Compose (Postgres + binário) e `AUTH_MODE=dev`. O deploy no `saradomin` com Zitadel vem depois do marco "em uso", com a mesma imagem. Toolchain: Go 1.27, Node 22, pnpm, Nix flake, Docker. Ver doc 04, seção 10, e doc 07.

## Controle do Microbot

Além do tracking somente leitura (doc 06), o dono decidiu controlar o cliente Microbot pelo planner: headless no zaros, VNC no navegador, plugins do Hub e rotinas agendadas. Módulo separado e desligado por padrão, com risco aceito conscientemente. Ver doc 08.

## Próximo passo proposto

Seguir a sequência do doc 04, seção 11. Primeiro fechar as decisões 10 e 11 e o modelo de objetivos do
doc 09. Depois vêm o schema, o catálogo de itens gerado e o piloto da extração por agente. Só então a API
de progresso sobre objetivos e os hiscores. O marco "em uso no zaros" inclui o teste do plugin no jogo.

## Discrepâncias e ressalvas

- O doc 03 estimou cobertura por inspeção dos campos; o doc 05 mediu após inferir regras do texto. Em caso de conflito, vale o doc 05.
- 7 nomes de quests do guia diferem do wiki (ex.: Vampire Slayer vs Vampyre Slayer, Garden of Tranquility vs Tranquillity); o normalizador já renomeia e `quests.json` carrega aliases.
- Dois steps iniciam quest sem step explícito de conclusão (Barcrawl, Fairytale II); comportamento esperado do guia original.
- O guia nunca cita kill count, então boss_kc só será útil em metas e no 2.0 com regras escritas à mão.
