# 09 — Modelo de domínio: objetivos, entidades e progresso compartilhado

Data: 2026-11-04. Estado: **aprovado pelo dono** e revisado criticamente no mesmo dia (as decisões da
revisão estão na seção 9.3). Este documento vem antes da API de progresso, porque ela é a primeira peça
que grava estado da conta sobre o modelo de conteúdo.

## 1. Por que revisar agora

Com dois guias carregados, o passo (`step`) como única unidade rastreável deixa de funcionar:

| | Oziris | BRUHsailer |
|---|---|---|
| Passos | 576 | 226 |
| Tamanho médio do texto | 53 caracteres | 1097 caracteres (maior: 7492) |
| Passos com `complete_when` | 209 (36%) | **0** |
| Passos com `location` | 573 | **0** |
| Metadados estruturados da fonte | quest, skillGoal, items | "GP after step", "Total time", "Items needed", "Skills / quests: yes" |

Consequências do modelo atual:

- **Sem auto-tracking no BRUHsailer.** Um passo dele é um parágrafo com dezenas de ações, e nenhuma vira
  regra.
- **Trocar de rota perde o progresso.** `step_progress` é por `(conta, guia, passo)`. Cook's Assistant
  concluído seguindo o Oziris não aparece no BRUHsailer.
- **Três identidades para a mesma coisa.**
  - As regras usam o nome (`"quest": "X Marks the Spot"`).
  - As menções usam o ref canônico (`quest:x_marks_the_spot`).
  - O `$defs/ref` do schema, o grafo do doc 04 e o comentário de `goals.ref` usam um terceiro formato
    (`quest:Dragon Slayer II`, `skill:agility:70`), e as fixtures da web um quarto (`quest:cooks-assistant`).
- **Metas (F9), grafo (F10) e marcos do `/compare` resolvem o mesmo conceito de três jeitos.** O conceito
  é "quest concluída", "skill ≥ 70" ou "item possuído".
- **Fatos de quest e diary não chegam ao `player_state`.** O contrato de ingest v1 não tem faceta de
  quest, o manifest só pede varbits que apareçam em regras `varbit` do conteúdo (hoje nenhuma) e o
  normalizador grava só `varbit`, `varp`, `skill`, `collection_log` e `item`. As 170 regras `quest_state`
  do Oziris nunca saem de `unknown`. Corrigido pela seção 3.2 e pelo item 7 da sequência.
- **Requisitos de item genéricos não cabem no loadout.** Exemplos: "Axe", "Pickaxe", "prayer potions",
  "food", "1k arrow shafts". O loadout só aceita um `item_id` exato. No Oziris, 11 itens ficaram ambíguos e
  96 sem resolução; o catálogo de itens tem cerca de 130 ids curados à mão.
- **Ids do BRUHsailer são hash do texto.** O id inclui todos os bullets e metadados do passo, então
  qualquer retoque de redação muda o id e deixa o progresso órfão. A tabela `step_id_aliases` existe, mas
  o importador não gera aliases e o loader não os aplica.

O resto do desenho continua válido e não muda:

- conteúdo em arquivos;
- núcleo puro;
- regras de três valores com evidência;
- identidade escopada por guia;
- ingest com frescor;
- separação entre posse, setup pronto e ação de banco (doc 04, seção 6.4.1).

## 2. Princípios

1. **O mundo é compartilhado e as rotas são independentes.** Fatos da conta (quests, levels, itens,
   diaries, unlocks) valem para todos os guias. Ordem, marcação manual e posição pertencem a cada guia.
2. **O objetivo é a unidade rastreável e o passo é a unidade de navegação.** O passo lista objetivos; o
   status automático do passo deriva deles.
3. **Um id por coisa.** Toda referência é um `Ref` canônico do registry (`internal/core/entities`), e o
   nome é só exibição.
4. **Nunca concluir sem evidência** (regra 2 do CLAUDE.md). Um objetivo só fica `met` com evidência
   observada. Inferências do texto não entram no conteúdo sem confirmação do dono durante a extração
   (seção 6.4).
5. **Extração por LLM é proposta, nunca autoridade.** Código determinístico verifica cada proposta antes
   de ela entrar em `content/` (seção 6).
6. **Todo alvo de objetivo tem uma fonte de fato definida.** Um alvo sem faceta e sem fonte que a
   preencha não entra no modelo (seção 3.2).

## 3. Conceitos

```
 MUNDO (compartilhado)                        ROTAS (independentes)
 ┌─────────────────────────────┐             ┌──────────────────────────────────┐
 │ Entity     (catálogo)       │             │ Guide → Variant → Section         │
 │  quest item skill diary ca  │             │   → Segment (bank_leg | sequence, │
 │  boss unlock* location*     │             │      bank_actions, loadout)       │
 │ Objective = Ref + alvo      │◀── papéis ──│   → Step (texto, objectives[])    │
 │ Requires   (grafo, wiki)    │             └────────────────┬─────────────────┘
 └──────────────┬──────────────┘                              │
                │ avaliado contra player_state                │
 CONTA          ▼                                             ▼
 ┌────────────────────────────────────────────────────────────────────────────┐
 │ objective_progress (1× por conta)   step_marks (manual/skip/pin/implied    │
 │ route_cursor (conta × guia)          por guia)      goals → Objective       │
 └────────────────────────────────────────────────────────────────────────────┘
   status do passo = derivado(objetivos `completes`) ⊕ marca da rota
                                                         * tipos novos
```

### 3.1 Entity

Uma coisa do mundo, com id estável no catálogo (`content/reference/`):

- **Tipos atuais**: `quest`, `item`, `skill`, `diary`, `ca`, `boss`.
- **Tipos novos** (decididos; entram já nesta revisão):
  - `unlock`: fairy rings, spellbooks, teleportes, acesso a áreas. São requisitos frequentes no grafo e
    raramente uma quest. Id: slug (`unlock:fairy_rings`); catálogo `content/reference/unlocks.json`, com
    `wiki_key` e, quando houver, o varbit ou varp que prova o unlock.
  - `location`: Draynor, Port Sarim. Serve para agrupar passos, "itens dos próximos passos no mesmo lugar"
    (F2) e "próximo banco". Id: slug (`location:draynor_village`); catálogo
    `content/reference/locations.json`. O BRUHsailer não tem `location` em nenhum passo hoje, então a
    extração (seção 6) é pré-requisito do F2 nessa rota.

Os dois tipos entram em `entities.Kinds`, ganham `Validate` e `HasCatalog`, e o verificador da
extração só aceita refs conhecidos. Entradas novas nascem pela tool `propose_entity` com confirmação do
dono (seção 6.2), nunca pela wiki inteira.

### 3.2 Objective

O estado verificável de uma entidade. A identidade do objetivo é derivada do ref e do alvo, então o mesmo
objetivo em dois guias é literalmente a mesma chave. A chave é a forma canônica abaixo, implementada
como `entities.ObjectiveKey` (parse, validação e `String()`), e é o que `objective_progress` e `goals`
armazenam.

```
 chave      := ref alvo
 alvo       := "@" estado | ">=" inteiro | "x" inteiro
 estado     := started | finished            (quest)
             | complete                      (diary)
             | done                          (unlock, ca)
 ">=" N     := skill (nível), boss (kill count), qp (quest points)
 "x" N      := item exato; N ≥ 1 sempre explícito na chave (item:952x1, nunca item:952)
```

Exemplos: `quest:x_marks_the_spot@finished`, `quest:rune_mysteries@started`, `skill:agility>=70`,
`diary:ardougne:easy@complete`, `item:960x120`, `unlock:fairy_rings@done`, `boss:zulrah>=50`,
`ca:574@done`, `qp>=107`.

Cada alvo mapeia para uma regra do motor e para a faceta do `player_state` que a sustenta. A fonte de
cada faceta é parte do modelo (princípio 6):

| Alvo | Regra | Faceta | Fonte do fato |
|---|---|---|---|
| `quest@started` / `quest@finished` | `quest_state` | `quest` | plugin, faceta `data.quest` do contrato v1 (item 7 da sequência); hiscores não sabe |
| `skill>=N` | `skill_level` | `skill` | plugin (`level`) ou hiscores |
| `item xN` | `item_owned` | `item` | plugin, containers opt-in (doc 04, seção 6.4.1) |
| `diary@complete` | `diary_tier` | `diary` | plugin, varbits `DIARY_<ÁREA>_<TIER>_COMPLETED` listados em `content/reference/varbits.json` e pedidos no manifest |
| `unlock@done` | `varbit`/`varp` do catálogo, senão só manual | `varbit`/`varp` | plugin, varbit do catálogo de unlocks no manifest |
| `boss>=N` | `boss_kc` | `activity` | hiscores (plugin não vê KC) |
| `ca@done` | `combat_achievement` | `combat_achievement` | plugin, varps do manifest (fase posterior; fora do piloto) |
| `qp>=N` | `varp` 101 (`QUEST_POINTS`) | `varp` | plugin; fallback: soma dos QP das quests concluídas pelo catálogo |

O manifest do ingest passa a ser construído a partir dessas tabelas de referência (varbits de diary,
varbits de unlock, varp 101) mais o que as regras do conteúdo pedirem, e não só das regras.

O objetivo é avaliado **uma vez por conta** e não por guia. Requisitos de item não exatos (`any_of`,
`family`, seção 3.4) **não têm chave de objetivo**: vivem só em loadouts e no papel `uses`, e são avaliados
pela readiness e pela posse, nunca persistidos em `objective_progress`.

### 3.3 Papel do objetivo no passo

| Papel | Significado | Efeito no status do passo |
|---|---|---|
| `completes` | o passo conclui o objetivo | todos `met` ⇒ passo `auto_done` |
| `starts` | o passo inicia (quest começada, por exemplo) | conta como `completes` do estado `started` |
| `requires` | precisa estar cumprido antes | não conclui; alimenta o grafo e o "bloqueado" do F2 |
| `uses` | consome ou usa (item, runa) | não conclui; alimenta "não venda/não drope" (F13) |

Hoje o `complete_when` do Oziris corresponde a `completes`/`starts` e o `prerequisites` a `requires`. A
migração é quase direta (seção 8). Objetivos de qualquer papel são avaliados: `requires` precisa de
veredito para o "bloqueado" do F2 e para o fechamento das metas (F9).

### 3.4 ItemRequirement (loadout e `uses`)

Substitui o `LoadoutItem` de id único:

```
 exact     { item: item:2347 }                           Hammer
 any_of    { any_of: [item:1359, item:1357, …] }         "Axe": qualquer um; a lista vem em ordem de
                                                         preferência (o primeiro é o melhor), decidida na
                                                         extração, não por um tier no catálogo
 family    { family: prayer_potion }                     qualquer dose, noteada ou não (campo `family`
                                                         que o catálogo já tem)
 qty       número; "1k" → 1000; ausente = "não informado pelo guia" (já existe: qty_stated)
 noted     flag explícita quando o guia diz "noted"
```

No conteúdo, `family` é o slug da família do catálogo (`entities.FamilySlug`, que escreve "+" como
`plus`: `antidote_plus_plus`), e as três variantes são exclusivas (`item_id`, `any_of` ou `family`).
Com `noted`, o setup carregado exige o id noteado. Em `any_of`, cada id precisa atingir a quantidade
sozinho; quantidades de ids diferentes não somam.

`category` ("food", "teleport") fica fora desta revisão: o catálogo não tem categorias e não há fonte
limpa para elas na wiki. "Food" entra como `family` curada quando aparecer na extração.

A readiness (`internal/core/planning`) e a posse (`rules.Owns`) passam a avaliar cada variante: um
requisito `any_of` é `met` quando qualquer id da lista satisfaz a quantidade; `family` usa
`CompareFamily`. A comparação exata para o setup carregado continua valendo dentro de cada variante.

### 3.5 Posição na rota

`route_cursor (conta, guia)` guarda o passo atual de cada rota. Permite alternar de guia sem perder o
lugar, e o `/compare` mostra as duas posições. "Agora" (F2) passa a ser o cursor da rota ativa.

Regra do cursor: o dono pode fixar qualquer passo (`cause = pin`); sem fixação, o cursor é o primeiro
passo da rota principal que não está `done` (auto, manual ou `implied` confirmado) nem `skipped`,
recalculado a cada reavaliação e a cada marca (`cause = auto`). Um pin é descartado quando o passo fixado
é concluído.

## 4. Formato no conteúdo

### 4.1 O passo em memória e na API

Um passo do Oziris com objetivos explícitos, como o loader entrega ao motor e a API ao cliente:

```json
{
  "id": "1.1.5",
  "text": "Buy a spade, start X marks the spot quest",
  "objectives": [
    { "key": "quest:x_marks_the_spot@started", "role": "starts",
      "evidence": { "field": "/text", "quote": "start X marks the spot quest", "span": [13, 41] },
      "source": { "method": "explicit", "confidence": "high" } },
    { "key": "item:952x1", "role": "uses",
      "evidence": { "field": "/text", "quote": "Buy a spade", "span": [0, 11] },
      "source": { "method": "llm", "confidence": "high", "skill_version": "annotate-guide@1",
                  "confirmed_by": "owner" } }
  ]
}
```

- `evidence.quote` precisa corresponder a uma substring do campo citado. O verificador compara depois de
  dobrar caixa, espaços e pontuação tipográfica (como `entities.foldText` faz para as menções), porque os
  textos têm aspas curvas ("Monk’s Friend") que um agente tende a normalizar; o `span` gravado é sempre o
  trecho exato do original.
- `source.method` ∈ `explicit` (campo estruturado da fonte), `derived` (regra determinística), `llm`
  (proposta de agente verificada) e `manual` (curadoria humana). As constantes de `content.Provenance`
  ganham os dois últimos.
- `source.confirmed_by: "owner"` marca o que foi decidido pelo dono durante a extração (seção 6.4); a
  pergunta e a resposta ficam no registro de decisões da anotação. `source.decision` cita a decisão geral
  de `decisions.json` aplicada, quando houver, e implica `confirmed_by: "owner"`. Toda entrada de
  sidecar com `confidence` diferente de `high` precisa de `confirmed_by: "owner"`; o loader rejeita o
  contrário, e também uma decisão citada que não exista. `skill_version` identifica a versão
  da skill; o agente não sabe o próprio id de modelo com confiança, então ele não é gravado.

### 4.2 Onde cada coisa vive em disco

Os importadores continuam donos de `guide.json`; os objetivos extraídos ficam num sidecar lido pelo
loader. Nunca há dois escritores para o mesmo arquivo (decisão D3, seção 9.3).

| Arquivo | Escritor | Conteúdo |
|---|---|---|
| `content/guides/<id>/guide.json` | `content migrate` / `content import-bruhsailer` | texto, segmentos, `complete_when`, `prerequisites`, `items`, menções; regenerável a qualquer momento |
| `content/guides/<id>/annotations/<section>.json` | `ironman content mcp` (a cada `submit` aceito) | por passo: `step_id`, `text_hash`, `objectives[]`, `items[]`, `bank_actions[]`, `location`; `skill_version`; schema `content/schema/annotations.schema.json` |
| `content/guides/<id>/annotations/decisions.json` | `ironman content mcp` (`record_decision`) | decisões gerais do dono |
| `content/guides/<id>/annotations.report.json` | `ironman content mcp` | rejeitados, ambíguos, baixa confiança, pendências |

O loader compõe os dois: converte `complete_when`/`prerequisites` em objetivos com
`source.method: explicit` e `derived`, e sobrepõe a anotação do passo (objetivos, requisitos de item,
local e ações de banco; a anotação vence a derivação de `internal/core/content/derive` para o passo
anotado). `content validate` valida `guide.json` e as anotações juntas: schema, `text_hash` igual ao
hash do texto atual, citação verificável e refs conhecidos. Hash diferente é erro no `validate` (a CI
falha) e aviso no `serve`, que ignora a anotação desatualizada e segue; reprocessar só esse passo
resolve.

Detalhes da composição, como implementados (item 2 da sequência):

- `text_hash` é `sha256:` do JSON de `text`, `location`, `notes` e `items` do passo: tudo o que uma
  anotação pode citar (`content.StepTextHash`).
- Objetivos: os do `guide.json` (se houver), depois os convertidos das regras, depois os da anotação; a
  mesma chave com o mesmo papel é substituída no lugar. A conversão rejeita, sem aproximar, o que não
  tem alvo de objetivo (`any`/`not`, `op: eq`, varbit, `item_owned` com containers restritos); a regra
  `manual` não vira objetivo.
- Itens e ações de banco da anotação substituem o loadout e as ações derivadas do segmento quando o
  passo anotado é o primeiro do segmento (é dele que `derive` tira os dois). Num passo do meio do
  segmento, ficam no próprio passo (`requirements`, `bank_actions`). O ferramental de extração (item 4)
  não recorta segmentos: `next_batch` diz ao agente se o passo abre o segmento (`starts_here`) e mostra o
  loadout e as ações derivadas, para ele saber o que vai substituir. Se o piloto mostrar que falta um
  corte, isso volta como correção do `derive`, não da anotação.
- `content.LoadGuideFile`/`ParseGuide` validam tudo isso, mas não alteram o guia (ele recodifica para o
  mesmo `guide.json`); só `content.Load` compõe.

`complete_when` continua aceito no schema durante a migração. Sai quando os importadores passarem a
emitir objetivos diretamente.

## 5. Progresso e persistência

Tabelas novas e alteradas (uma migration goose; nunca editar a 0001–0003):

| Tabela | Chave | Conteúdo |
|---|---|---|
| `objective_progress` | `(account_id, objective_key)` | veredito, `evidence_snapshot_id`, `evidence` JSON, `first_met_at`, `met_in_guide_id` (rota ativa no momento em que ficou `met`; alimenta a cascata), `regressed_at` |
| `objective_progress_events` | id | `(account_id, objective_key, from, to, snapshot_id, at)`; fonte do F6 para objetivos |
| `step_marks` | `(account_id, guide_id, step_id)` | `kind ∈ {manual_done, skipped, pinned_open, implied}`, `implied_by_step_id`, `implied_at`, `note`, `marked_at` (evolui `step_progress`) |
| `step_status_events` | id | `(account_id, guide_id, step_id, from, to, cause ∈ {engine, user, implied, migration, import}, snapshot_id, at)`; gravada para **todos** os guias quando o status derivado muda (decisão D2) |
| `route_cursor` | `(account_id, guide_id)` | passo atual, `cause ∈ {pin, auto}`, `updated_at` |
| `goals` | — | `objective_key` no lugar de `ref TEXT` |
| `content_steps` | `(guide_id, step_id)` | índice mínimo (seção, posição, hash do texto) para joins de eventos e detecção de órfãos; gravado no boot junto com `content_versions` |
| `step_id_aliases` | `(guide_id, old_step_id)` | aplicado pelo loader no boot a `step_marks`, `step_status_events` e `route_cursor` |

O status automático do passo **não é armazenado**: é uma função pura em `internal/core/progress`
(objetivos do passo + vereditos + marcas do guia) e é calculado na leitura e na reavaliação. A ordem:

1. Marca da rota (`manual_done`, `skipped`, `pinned_open`, `implied`). O motor nunca a sobrescreve.
2. Todos os objetivos `completes`/`starts` estão `met`, com evidência ⇒ `auto_done` (evidência = a dos
   objetivos).
3. Senão, `pending`. Um passo sem objetivo concluível é checkbox, como hoje.

A reavaliação por snapshot faz, nesta ordem: avalia todos os objetivos referenciados por qualquer guia
(qualquer papel) → grava `objective_progress` e os eventos de objetivo → recalcula o status derivado de
cada passo de cada guia e grava `step_status_events` para o que mudou (`steps_changed` da resposta do
ingest é esse número) → aplica a cascata (seção 9.1) só na rota ativa, gravando marcas `implied` →
recalcula os cursores `auto`.

Com isso o progresso é compartilhado entre as rotas por construção: o mesmo `objective_key` aparece
cumprido nos dois guias, enquanto marcas, eventos de passo e cursor continuam por guia. A readiness não
muda de natureza: continua lendo `player_state`. A posse (`item_owned`) vira só mais um alvo de objetivo.

`manual_ok` sai do modelo: o clique do dono sempre vence (doc 03, princípio 4), então não existe passo
que não possa ser marcado à mão.

## 6. Extração dos guias por agente no Claude Code

Decisão do dono: a estruturação dos textos é uma **operação única feita por um agente no Claude Code**,
acompanhada ao vivo pelo dono. Não há script nem chave de API. O resultado é JSON commitado em
`content/` (os sidecars da seção 4.2), que passa a ser a fonte estruturada das rotas. O volume é pequeno:
cerca de 96 mil tokens de texto nos dois guias (Oziris ~16 mil, BRUHsailer ~80 mil; a maior seção,
BRUHsailer 1.4, tem 45 mil caracteres).

### 6.1 Por que Claude Code com um MCP local

- **Operação única, pequena e supervisionada.** Rodar na sessão do dono permite acompanhar, interromper
  e corrigir; um script batch não ofereceria isso.
- **A reprodutibilidade menor não importa aqui.** O resultado fica commitado e verificado, e só passos
  com texto alterado são reprocessados.
- **Ferramentas em vez de edição livre.** O agente fala com um **MCP stdio local** (`ironman content mcp`,
  declarado em `.mcp.json` na raiz do repositório) e segue uma **skill**. O MCP:
  - entrega lotes pequenos, sem o agente ler o `guide.json` inteiro;
  - expõe o resolvedor de entidades, então o agente não inventa ids;
  - verifica cada envio na hora e devolve os erros, para o agente corrigir no mesmo ciclo;
  - guarda o estado nos sidecars, então dá para parar e retomar em outra sessão.

  O agente nunca escreve no `guide.json` nem nos sidecars diretamente.
- **Não viola a regra 1 do CLAUDE.md.** `ironman content mcp` trabalha só sobre `content/`: sem banco e
  sem rede. É distinto do `ironman mcp` futuro, que é de progresso da conta e fala com a API.

### 6.2 Fluxo

```
 dono assiste no terminal do Claude Code
 ┌──────────────────────────────────────────────────────────────────────────────┐
 │ skill annotate-guide (taxonomia, exemplos +/-, procedimento)                 │
 │                                                                              │
 │  agente ──next_batch(guide, section)──▶ ironman content mcp (stdio, offline) │
 │    │      ◀── passos: id, hash, texto, notas, itens, contexto ──              │
 │    │                                                                         │
 │    ├──resolve_entity(kind, nome)──────▶ registry → candidatos / ambíguo       │
 │    ├──propose_entity(kind, nome, wiki) ▶ entrada nova em unlocks/locations,   │
 │    │                                     só com confirmação do dono           │
 │    ├──submit(guide, section, anotações)▶ verificador determinístico          │
 │    │      ◀── aceitos / rejeitados com motivo (citação, ref, schema, ordem) ─ │
 │    │      (corrige e reenvia os rejeitados)                                  │
 │    ├──record_decision / list_decisions ▶ decisions.json                      │
 │    └──status(guide) / report(guide) ──▶ cobertura, baixa confiança, pendências│
 └──────────────────────────────────────────────────────────────────────────────┘
          │ grava a cada submit aceito
          ▼
 content/guides/<id>/annotations/<section>.json   (saída verificada + hash por passo)
          │
          ▼ ironman content validate (guide.json + anotações) ──▶ commit
          │
          ▼ boot do serve: o loader compõe guide.json + anotações, indexa
            content_versions/content_steps e aplica aliases; objetivos avaliados
            contra player_state. A app já sobe "seedada" pelo conteúdo, sem seed
            de banco à parte (regra 3: o banco só indexa).
```

Não existe etapa de merge: o sidecar é a forma final (seção 4.2).

### 6.3 O que o agente devolve, por passo

- Objetivos com papel e alvo, cada um com a citação do texto.
- Requisitos de item no formato da seção 3.4, com quantidade interpretada ("1k" → 1000).
- Ações de banco com o tipo correto. Por exemplo, "bank everything" é `deposit_all`, não `open_bank`.
  A anotação vence a derivação determinística para o passo anotado.
- Local do passo, quando o texto disser.
- Confiança por item: `high` quando explícito no texto, `medium` quando inferido.

O agente devolve **nomes**. O ref final é sempre o que o resolvedor atribui no `submit`; o que não
resolver vai para o relatório, sem id.

### 6.4 Consulta interativa ao dono

O dono acompanha a extração o tempo todo. O agente **pergunta sempre que precisar**, em vez de deixar a
dúvida para um estado "sugerido" na aplicação. Pela skill, o agente pergunta quando:

- **o objetivo é inferido**: "do Waterfall" = concluir? "grab the gloves" = item possuído ou só usado?
- **o nome é ambíguo** ou não resolve: qual "Axe"? "prayer potions" = família ou dose específica?
- **a taxonomia não cobre o caso**: unlock novo, local novo, papel incerto entre `starts` e `completes`;
- **o texto contradiz o catálogo ou outro passo**: por exemplo, uma quest concluída antes de iniciada;
- **o verificador rejeita** e a correção não é óbvia.

Como perguntar:

- Agrupar as dúvidas por passo, ou por lote quando forem do mesmo tipo.
- Mostrar a citação exata e oferecer opções concretas, com a recomendação em primeiro lugar.
- Não perguntar o que o texto já diz explicitamente.

Cada resposta é gravada:

- **No item anotado**, como `confirmed_by: "owner"`.
- **Quando a resposta vale como regra geral** (por exemplo, "'Axe' sem qualificador = qualquer axe, o
  melhor disponível"), também em `content/guides/<id>/annotations/decisions.json`. O agente aplica a
  regra nos lotes seguintes sem perguntar de novo e cita a decisão quando a usar.

O MCP expõe `record_decision` e `list_decisions` para isso.

Consequência: o que entra no `content/` está sempre resolvido. Ou é explícito (`high`), ou foi confirmado
pelo dono. O que o dono deixar em aberto fica fora dos sidecars e aparece só no relatório.

### 6.5 Artefatos versionados

| Artefato | Papel |
|---|---|
| `.mcp.json` | declara `ironman content mcp` como servidor stdio para o Claude Code |
| `.claude/skills/annotate-guide/SKILL.md` | procedimento, taxonomia fechada, exemplos positivos e negativos, quando e como consultar o dono; versão em `skill_version` |
| `content/schema/annotations.schema.json` | schema dos sidecars, embutido no binário como o do guia |
| `content/reference/unlocks.json`, `locations.json`, `varbits.json` | catálogos novos (seções 3.1 e 7) |
| `content/guides/<id>/annotations/decisions.json` | decisões gerais do dono (pergunta, resposta, escopo), reaplicadas nos lotes seguintes |
| `internal/core/annotate` | lote, verificação e composição: puro, testado por tabela, sem LLM |
| `ironman content mcp` | servidor MCP stdio sobre `internal/core/annotate` e o registry |
| `ironman content validate` | valida `guide.json` e os sidecars juntos; roda na CI sobre o que está commitado |
| `content/guides/<id>/annotations/<section>.json` | saída aceita, com hash do texto de cada passo e versão da skill |
| `content/guides/<id>/annotations.report.json` | rejeitados, ambíguos, baixa confiança |

A CI nunca chama LLM: só verifica que o que está commitado continua consistente com o texto e o
catálogo. Reprocessar é necessário só para passos cujo hash mudou. Como os ids do BRUHsailer passam a ser
posicionais (decisão D6, seção 9.3), uma edição de texto upstream mantém o id e invalida só a anotação
daquele passo; uma inserção renumera os passos seguintes e o importador grava os aliases.

Alternativa considerada: a skill sozinha, com o agente gravando JSON e rodando
`content validate` pelo Bash. Funciona, mas o agente precisaria ler arquivos grandes e só
descobriria erros no fim do lote. Como o núcleo (`internal/core/annotate`) é o mesmo nos dois casos, o
MCP é uma camada fina por cima.

### 6.6 Piloto e critério de aceite

1. **Oziris 1.1** (286 passos): já tem 90 passos com regras determinísticas (94 regras: 76 `quest_state`,
   17 `skill_level`, 1 `diary_tier`) para medir concordância.
2. **BRUHsailer 1.1** (18 passos longos, 20 mil caracteres): o caso difícil.

Aceite, medido pelo `report`:

- ≥ 95% das propostas aceitas no primeiro `submit` (o verificador rejeita o resto, então "citações
  verificadas" é 100% por construção e não serve de métrica);
- ≥ 95% das 94 regras existentes reproduzidas como objetivo com o mesmo ref e alvo; cada divergência
  explicada no relatório;
- perguntas ao dono por passo medidas por seção, com a meta de cair nas seções seguintes conforme as
  decisões gerais se acumulam;
- relatório de revisão curto o bastante para leitura humana.

O dono acompanha cada lote, responde as consultas (seção 6.4) e pode aprovar ou corrigir antes do
commit. Só depois disso roda o resto, cerca de 13 seções.

### 6.7 Riscos

| Risco | Mitigação |
|---|---|
| Objetivo inventado | Citação literal obrigatória |
| Citação "quase igual" (aspas curvas, espaços) rejeitada em massa | Verificador compara dobrado e grava o span exato (seção 4.1) |
| Extrair demais ("talk to Hans" não é objetivo) | Taxonomia fechada com exemplos negativos |
| Inferência implícita ("do Waterfall") | O agente pergunta ao dono na hora; a resposta vira `confirmed_by: owner` ou uma decisão geral |
| Perguntas demais cansam o dono | Agrupar por passo ou lote, decisões gerais reaplicadas, métrica de perguntas por seção no piloto |
| Variação entre lotes | Prompt fixo, lotes por seção, métrica de concordância |
| Item fora do catálogo | Gerar o catálogo de itens **antes** do piloto (seção 7) |
| Unlock ou local fora do catálogo | `propose_entity` com confirmação do dono, no mesmo ciclo |
| Texto do guia sai da máquina | Aceito: uso pessoal; o texto vai só para a Claude, nunca para terceiros |

### 6.8 Contrato das tools (como implementado, item 4)

`ironman content mcp` (servidor `ironman-content` no `.mcp.json`, roda com `go run`) é uma camada fina
sobre `internal/core/annotate`; o protocolo é o MCP stdio (JSON-RPC por linha, só tools) de
`internal/mcp`. Argumentos com campo desconhecido são recusados, para um erro de digitação não sumir
calado.

| Tool | Faz |
|---|---|
| `status(guide?)` | cobertura por guia e seção: anotados, desatualizados (hash mudou), pendentes |
| `next_batch(guide, section, limit?)` | próximos passos sem anotação atual, na ordem do guia (8 por padrão; um lote fecha antes de passar de 12 mil caracteres, mas leva sempre um passo): campos citáveis, menções já resolvidas, objetivos vindos das regras, segmento (`starts_here`, loadout e ações derivados) e as decisões do guia |
| `resolve_entity(kind, name, tier?)` | resolve como o `submit` vai resolver: `resolved` com ref, `ambiguous` com candidatos, `unresolved` com motivo; para item, diz a família quando o nome cobre uma |
| `propose_entity(kind, name, ..., confirmed_by: owner)` | só `unlock` e `location`; grava em `content/reference` e recarrega o registry; recusa nome ou alias que já resolve |
| `submit(guide, section, skill_version, steps)` | verifica e grava; cada passo entra inteiro ou é rejeitado com todos os motivos |
| `record_decision` / `list_decisions` | `decisions.json`; mesmo id substitui (o dono mudou de ideia) |
| `report(guide)` | o relatório, também gravado em `annotations.report.json` |

O agente manda **nomes e citações**, nunca refs nem spans. O `submit`:

1. resolve cada nome pelo registry lido do disco (não o embutido, então uma entrada recém-proposta já
   resolve). `id` só escolhe entre os candidatos de um nome ambíguo, e nome sem resolução nunca ganha id;
2. localiza a citação no campo indicado (`/text` por padrão, ou `/location`, `/notes/<i>/text`,
   `/items/<i>`) com a mesma dobra do verificador e grava o span exato do original (primeira ocorrência);
3. exige `confidence` `high` ou `medium`, e `medium` só com `confirmed_by: owner` ou `decision`;
4. roda `Guide.CheckStepAnnotation`, a mesma checagem do loader, então o MCP aceita exatamente o que o
   `content validate` aceita;
5. confere a ordem da rota contra os objetivos das regras e as anotações atuais, só entre passos da
   mesma variante e nunca entre uma track e o resto: quest iniciada depois de concluída, quest concluída
   antes de iniciar, algo concluído duas vezes (inclui nível menor depois de um maior), requisito que a
   rota só cumpre depois;
6. grava os passos aceitos em `annotations/<seção>.json`, na ordem do guia, depois de validar o arquivo
   inteiro contra o schema (escrita atômica).

O relatório guarda o histórico por passo (`steps`: envios, aceito no primeiro envio, motivos da última
rejeição, comentário do agente) e recalcula a cada envio:

- a cobertura;
- a aceitação no primeiro envio;
- o que o dono confirmou e as decisões aplicadas;
- a concordância com as regras (objetivo de regra repetido com a mesma chave e papel; cada divergência
  com o comentário do passo);
- as rejeições abertas;
- tudo o que entrou com confiança `medium`.

Essas são as métricas do piloto (seção 6.6), exceto as perguntas por seção. As perguntas acontecem no
Claude Code, fora do MCP; o relatório conta só as respostas registradas.

## 7. Catálogos

- **Itens** (feito em 2026-11-04). `tools/extract_wiki_items.py` extrai os itens citados pelos dois
  guias mais as famílias que eles tocam (poções, runas, machados, picaretas, comida, joias com carga,
  tabs) para `data/wiki/items.json`: 856 ids de 567 páginas, com nome, forma noteada, empilhável, slot de
  equipamento, família e variante. O bucket `infobox_item` não tem forma noteada nem empilhável, então
  o script lê o wikitext do `{{Infobox Item}}`/`{{Infobox Bonuses}}` pela API da wiki e tira o id noteado
  da classe `Cert` do `gameval/ItemID.java` do RuneLite, num commit fixado (gravado em `source`). Só
  esse script usa a rede; `catalogbuild` gera `content/reference/items.json` a partir do extrato mais
  os aliases e nomes ambíguos curados de `data/wiki/entity_aliases.json`. O catálogo curado antigo
  (133 ids) virou fixture de teste: o gerado concorda com ele, salvo duas correções (o id 371 é
  "Raw swordfish", não "Swordfish"; "Rune sword" virou "Rune shortsword" e ficou como alias). O nome
  do guia só decide o que buscar; a resolução continua exata (nome, alias curado, plural). O cache
  inteiro (~30 mil ids) fica fora: não há sinal de que vá ser preciso.

  Efeito nos relatórios de entidades: no Oziris, itens resolvidos 87 → 119 e sem resolução 96 → 62;
  ambíguos 11 → 13, porque nomes antes desconhecidos agora caem em duas ou mais ids com o mesmo nome
  (Scrying orb cheio/vazio). No BRUHsailer: resolvidos 1023 → 1391, sem resolução 798 → 369, ambíguos
  85 → 146 pelo mesmo motivo (famílias sem variante, como "necklace of passage", e nomes repetidos). Ambíguo
  é o estado certo para esses casos: a extração escolhe com `any_of` ou `family`.
- **Unlocks e locais.** Catálogos novos, começando pelo que os guias citam (lista sai da extração via
  `propose_entity`), não pela wiki inteira. `unlocks.json` e `locations.json` existem e começam vazios: o
  `location` em texto livre do Oziris (119 valores, como "Draynor" e "Draynor Village", "Various") é
  evidência para as propostas, não um catálogo.
- **Varbits de referência** (`content/reference/varbits.json`, gerado): os 48
  `DIARY_<ÁREA>_<TIER>_COMPLETED` por diary (doc 02, seção 5) e o varp 101 (quest points). Os
  varbits/varps dos unlocks ficam no próprio `unlocks.json` (`proof`). É o que o manifest do ingest passa
  a pedir além das regras do conteúdo (seção 3.2; item 7 da sequência).

## 8. Migração do que existe

| Hoje | Depois |
|---|---|
| `complete_when` com nome de quest | `objectives` com chave canônica; conversão automática no loader |
| `prerequisites` | objetivos com papel `requires` |
| `$defs/ref` do schema (`quest:<nome wiki>`, `skill:<skill>:<nível>`) em `requires`/`recommended_after`/`unlocks` | `entityRef` e chave de objetivo; o grafo do doc 04 e as fixtures da web adotam a mesma gramática |
| `items` + `Loadout.inventory/equipment` (`LoadoutItem`) | `ItemRequirement` (seção 3.4); readiness e posse avaliam as variantes |
| `step_progress` (por guia) | `objective_progress` (por conta) + `step_marks` (por guia); a migration copia `manual_*` e `skipped` e **derruba** `step_progress`, `step_progress_events` e `v_step_board` |
| `step_progress_events` | `objective_progress_events` + `step_status_events` por guia |
| nove tabelas `snapshot_*` tipadas (quatro sem escritor) | uma `snapshot_facts(snapshot_id, facet, key, value)` com o shape de `player_state`; `snapshot_item_containers` fica; feito no item 7, junto com a faceta `quest` |
| `accounts.guide_id` (rota única) | continua como rota ativa; `route_cursor` por guia |
| `goals.ref TEXT` | `goals.objective_key` |
| `manual_ok` | sai; o clique do dono sempre vence |
| ids do BRUHsailer por hash do texto | ids posicionais da numeração upstream (`bruh-1.1-05` de `CH1.5`), hash do texto em `source.fingerprint`; aliases gerados pelo importador quando a numeração muda |
| `content_versions`/`content_steps`/`step_id_aliases` só como queries sqlc | gravados e aplicados pelo loader no boot; `content_steps` perde `rule_hash`, `is_manual`, `layers` e `modes` e ganha `text_hash`; `content_versions` troca `guide_version`/`step_count` por um resumo por guia |
| `wiki_cache` e `graph_cache` sem uso | ficam no schema até o adapter do wiki e o grafo existirem (decisão do dono, 2026-11-04) |
| marcos do `/compare` (heurística no cliente) | objetivos compartilhados servidos pela API |
| contrato de ingest v1 sem quests | mesmo `contract_version: 1`, com a faceta opcional `data.quest` e os varbits de referência no manifest (doc 04, seção 6.4); o servidor sobe antes do plugin porque rejeita campos desconhecidos |

## 9. Decisões

### 9.1 Tomadas

- **Progresso compartilhado entre rotas**: fatos da conta valem em todos os guias; marca manual e posição
  ficam por guia.
- **Extração dos textos por agente no Claude Code**, acompanhada pelo dono, como operação única com
  resultado commitado e verificado (seção 6). Não é recorrente nem automática.
- **MCP stdio local (`ironman content mcp`) + skill** como interface do agente; saída em JSON
  commitado, e a app já sobe com esse conteúdo (sem seed de banco à parte).
- **Objetivo** como unidade rastreável e **Ref canônico** como identidade única.
- **Consulta interativa**: o dono acompanha a extração e o agente pergunta sempre que precisar (seção
  6.4). Não existe estado "sugerido" na aplicação: o que entra no conteúdo é explícito ou confirmado pelo
  dono e pode concluir passos com evidência; o que ficar em aberto fica fora dos sidecars.
- **`unlock` e `location` entram já nesta revisão**: no schema e no registry desde o passo 2 da
  sequência, com catálogos povoados pelo que a extração encontrar (cada entrada nova confirmada pelo
  dono), não pela wiki inteira.
- **Cascata `implied` (F1)**: quando um passo conclui por evidência, os passos **manuais** anteriores
  ainda `pending` viram `implied`. Limites:
  - só **dentro da mesma seção**;
  - só na **rota ativa**: o passo que dispara precisa ter concluído enquanto aquela rota é a seguida
    (`met_in_guide_id` do objetivo = rota ativa). Um objetivo cumprido seguindo o Oziris não dispara
    cascata no BRUHsailer;
  - nunca em **tracks** paralelas;
  - a marca `implied` aponta para o passo que disparou (`implied_by_step_id`), tem visual próprio e só
    vira feito com a confirmação do dono ("confirmar tudo"). Nunca é `auto_done`.

### 9.2 Em aberto (dono)

Nenhuma. O planejamento do modelo está fechado; a execução segue o doc 04, seção 9.

### 9.3 Decisões da revisão crítica (2026-11-04)

| # | Decisão | Escolha do dono |
|---|---|---|
| D1 | Como quest e diary chegam ao `player_state` | faceta `data.quest` no contrato v1 (aditiva) e varbits de diary de referência no manifest; não mapear varbit → quest no servidor |
| D2 | Eventos de status de passo | para todos os guias, não só a rota ativa |
| D3 | Onde vivem as anotações | sidecar canônico lido pelo loader; sem comando `merge` |
| D4 | Requisitos de item não exatos | sem chave de objetivo; `category` fora desta revisão |
| D5 | `content_steps` | fica como índice mínimo, gravado pelo loader |
| D6 | Ids do BRUHsailer | posicionais da numeração upstream, com fingerprint do texto |
| D7 | Ordem da sequência | mantida: extração completa antes do progresso persistido; o contrato (D1) entra como item próprio antes do progresso |
| D8 | Catálogo de itens | itens citados pelos guias mais famílias, extraídos da wiki para `data/wiki/items.json` (wikitext do infobox, porque o bucket não traz forma noteada nem empilhável; seção 7) |
| D9 | Documentação | doc 04 reescrito como arquitetura atual; seções superadas e artefatos SQLite/Zod removidos (o git guarda o histórico); doc 01 vira registro de decisões |
| D10 | `manual_ok` | sai do modelo |

## 10. Sequência

A sequência do projeto, com os critérios de "pronto quando" de cada item, está só no doc 04, seção 9.
Os itens deste modelo são o 2 (schema), o 3 (dados e catálogos), o 4 (ferramental de extração), o 5
(piloto), o 6 (extração completa), o 7 (contrato com `data.quest`) e o 8 (progresso sobre objetivos).
