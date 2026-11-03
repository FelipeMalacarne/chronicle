# ironman-planner

Planner e tracker self-hosted para conta Ironman de OSRS, baseado no Oziris Ironman Efficiency Guide
(texto de OzirisLoL, versão interativa mantida por ironman.guide), com tracking automático de progresso
via hiscores, Wise Old Man, WikiSync/plugin RuneLite e dados estruturados do OSRS Wiki.

## Estado

Descoberta e design concluídos (docs 00 a 05). Nenhum código de aplicação ainda; a sequência de MVP está em `docs/04-arquitetura.md`, seção 7.4.

## Documentos

| Arquivo | Conteúdo |
|---|---|
| `docs/00-levantamento-inicial.md` | Levantamento inicial: fontes, modelo de dados, fases |
| `docs/01-sumario-executivo.md` | Consolidação da rodada de design, números e decisões pendentes |
| `docs/02-fontes-de-dados.md` | APIs verificadas na prática, formatos reais, estratégia de sync |
| `docs/03-produto-e-features.md` | Jobs-to-be-done, catálogo de features, UX tela a tela |
| `docs/04-arquitetura.md` | Stack, componentes, schema de conteúdo, banco, grafo, plugin |
| `docs/05-conteudo-e-normalizacao.md` | O que foi extraído do guia e como vira regras de auto-complete |

## Dados

- `data/guide/` guia principal extraído (7 seções, 576 passos) e versão normalizada em `data/guide/normalized/`.
- `data/raw/ironman-guide/` payloads brutos e páginas companheiras em Markdown.
- `data/wiki/` dados estruturados do OSRS Wiki (quests, requisitos, diaries).
- `data/samples/` respostas de exemplo de cada API.

## Atribuição

O conteúdo do guia pertence a OzirisLoL e ironman.guide. Uso pessoal; manter atribuição em qualquer tela que exiba os passos.
