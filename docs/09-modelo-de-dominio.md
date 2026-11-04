# 09 — Modelo de domínio: objetivos, entidades e progresso compartilhado

Data: 2026-11-04. Estado: **direção aprovada pelo dono**; detalhes em aberto na seção 9. Este documento
vem antes da API de progresso, porque ela é a primeira peça que grava estado da conta sobre o modelo de
conteúdo.

## 1. Por que revisar agora

Com dois guias carregados, o passo (`step`) como única unidade rastreável deixa de funcionar:

| | Oziris | BRUHsailer |
|---|---|---|
| Passos | 576 | 226 |
| Tamanho médio do texto | 53 caracteres | 1097 caracteres (maior: 7492) |
| Passos com `complete_when` | 209 (36%) | **0** |
| Metadados estruturados da fonte | quest, skillGoal, items | "GP after step", "Total time", "Items needed", "Skills / quests: yes" |

Consequências do modelo atual:

- **Sem auto-tracking no BRUHsailer.** Um passo dele é um parágrafo com dezenas de ações, e nenhuma vira
  regra.
- **Trocar de rota perde o progresso.** `step_progress` é por `(conta, guia, passo)`. Cook's Assistant
  concluído seguindo o Oziris não aparece no BRUHsailer.
- **Três identidades para a mesma coisa.**
  - As regras usam o nome (`"quest": "X Marks the Spot"`).
  - As menções usam o ref canônico (`quest:x_marks_the_spot`).
  - O exemplo do schema de `goals.ref` usa um terceiro formato (`'quest:Dragon Slayer II'`).
- **Metas (F9), grafo (F10) e marcos do `/compare` resolvem o mesmo conceito de três jeitos.** O conceito
  é "quest concluída", "skill ≥ 70" ou "item possuído".
- **Requisitos de item genéricos não cabem no loadout.** Exemplos: "Axe", "Pickaxe", "prayer potions",
  "food", "1k arrow shafts". O loadout só aceita um `item_id` exato. No Oziris, 11 itens ficaram ambíguos e
  96 sem resolução; o catálogo de itens tem cerca de 130 ids curados à mão.
- **Ids do BRUHsailer são hash do texto.** Uma edição upstream muda o id e deixa o progresso órfão. A
  tabela `step_id_aliases` existe, mas o importador não gera aliases.

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
 │ objective_progress (1× por conta)   step_marks (manual/skip/pin por guia)   │
 │ route_cursor (conta × guia)         goals → Objective                       │
 └────────────────────────────────────────────────────────────────────────────┘
   status do passo = derivado(objetivos `completes`) ⊕ marca manual do guia
                                                         * tipos novos
```

### 3.1 Entity

Uma coisa do mundo, com id estável no catálogo (`content/reference/`):

- **Tipos atuais**: `quest`, `item`, `skill`, `diary`, `ca`, `boss`.
- **Tipos novos** (decididos; entram já nesta revisão):
  - `unlock`: fairy rings, spellbooks, teleportes, acesso a áreas. São requisitos frequentes no grafo e
    raramente uma quest.
  - `location`: Draynor, Port Sarim. Serve para agrupar passos, "itens dos próximos passos no mesmo lugar"
    (F2) e "próximo banco".

### 3.2 Objective

O estado verificável de uma entidade. A identidade do objetivo é derivada do ref e do alvo, então o mesmo
objetivo em dois guias é literalmente a mesma chave:

```
 quest:x_marks_the_spot@finished        quest:rune_mysteries@started
 skill:agility>=70                      diary:ardougne_easy@complete
 item:960x120                           unlock:fairy_rings@done
```

Cada tipo de alvo mapeia para uma regra que o motor já sabe avaliar, ou que precisa ganhar:

| Alvo | Regra |
|---|---|
| `@started` / `@finished` | `quest_state` |
| `>=N` | `skill_level` |
| `xN` | `item_owned` |
| `@complete` | `diary_tier` |
| `@done` (unlock) | varbit/varp ou manual |

O objetivo é avaliado **uma vez por conta** e não por guia.

### 3.3 Papel do objetivo no passo

| Papel | Significado | Efeito no status do passo |
|---|---|---|
| `completes` | o passo conclui o objetivo | todos `met` ⇒ passo `auto_done` |
| `starts` | o passo inicia (quest começada, por exemplo) | conta como `completes` do estado `started` |
| `requires` | precisa estar cumprido antes | não conclui; alimenta o grafo e o "bloqueado" do F2 |
| `uses` | consome ou usa (item, runa) | não conclui; alimenta "não venda/não drope" (F13) |

Hoje o `complete_when` do Oziris corresponde a `completes`/`starts` e o `prerequisites` a `requires`. A
migração é quase direta (seção 8).

### 3.4 ItemRequirement (loadout e `uses`)

Substitui o `LoadoutItem` de id único:

```
 exact     { item: item:2347 }                         Hammer
 any_of    { any_of: [item:1351, item:1353, …],        "Axe": qualquer um, preferindo o melhor
             prefer: best }
 family    { family: prayer_potion }                   qualquer dose, noteada ou não
 category  { category: food, min: 10 }                 categoria do catálogo
 qty       número; "1k" → 1000; ausente = "não informado pelo guia" (já existe: qty_stated)
 noted     flag explícita quando o guia diz "noted"
```

A readiness (`internal/core/planning`) passa a avaliar cada variante; a regra de comparação exata para
o setup carregado continua valendo dentro de cada variante.

### 3.5 Posição na rota

`route_cursor (conta, guia)` guarda o passo atual de cada rota. Permite alternar de guia sem perder o
lugar, e o `/compare` mostra as duas posições. "Agora" (F2) passa a ser o cursor da rota ativa.

## 4. Formato no conteúdo (rascunho do schema)

Um passo do Oziris com objetivos explícitos:

```json
{
  "id": "1.1.5",
  "text": "Buy a spade, start X marks the spot quest",
  "objectives": [
    { "ref": "quest:x_marks_the_spot", "target": "started", "role": "starts",
      "evidence": { "quote": "start X marks the spot quest", "field": "/text" },
      "source": { "method": "explicit", "confidence": "high" } },
    { "ref": "item:952", "target": "owned", "qty": 1, "role": "uses",
      "evidence": { "quote": "Buy a spade", "field": "/text" },
      "source": { "method": "llm", "confidence": "high", "model": "<id>", "prompt": "annotate-v1" } }
  ]
}
```

- `evidence.quote` precisa ser uma substring exata do campo citado. O loader verifica isso, como já faz
  com o `span` das menções.
- `source.method` ∈ `explicit` (campo estruturado da fonte), `derived` (regra determinística), `llm`
  (proposta de agente verificada) e `manual` (curadoria humana).
- `source.confirmed_by: "owner"` marca o que foi decidido pelo dono durante a extração (seção 6.4); a
  pergunta e a resposta ficam no registro de decisões da anotação.
- `complete_when` continua aceito no schema durante a migração e é convertido para `objectives` pelo
  loader. Depois sai.

## 5. Progresso e persistência

Tabelas novas (uma migration goose; nunca editar a 0001–0003):

| Tabela | Chave | Conteúdo |
|---|---|---|
| `objective_progress` | `(account_id, objective_key)` | veredito, `evidence_snapshot_id`, `evidence` JSON, `first_met_at`, `regressed_at` |
| `step_marks` | `(account_id, guide_id, step_id)` | `manual_done`, `skipped`, `pinned_open`, nota (evolui `step_progress`) |
| `route_cursor` | `(account_id, guide_id)` | passo atual, `updated_at`, causa |
| `goals` | — | `objective_key` no lugar de `ref TEXT` |

Status do passo, avaliado em ordem:

1. Marca manual da rota (`manual_done`, `skipped`, `pinned_open`). O motor nunca a sobrescreve.
2. Todos os objetivos `completes`/`starts` estão `met`, com evidência ⇒ `auto_done` (evidência = a dos
   objetivos).
3. Cascata `implied` (F1): só dentro da seção e só na rota ativa, visualmente distinta e confirmável
   (seção 9.1).
4. Senão, `pending`. Um passo sem objetivo concluível é checkbox, como hoje.

Com isso o progresso é compartilhado entre as rotas por construção: o mesmo `objective_key` aparece cumprido
nos dois guias, enquanto marcas e cursor continuam por guia. A readiness não muda de natureza: continua
lendo `player_state`. A posse (`item_owned`) vira só mais um alvo de objetivo.

## 6. Extração dos guias por agente no Claude Code

Decisão do dono: a estruturação dos textos é uma **operação única feita por um agente no Claude Code**,
acompanhada ao vivo pelo dono. Não há script nem chave de API. O resultado é um JSON commitado em
`content/`, que passa a ser a fonte estruturada das rotas. O volume é pequeno: cerca de 96 mil tokens de
texto nos dois guias (Oziris ~16 mil, BRUHsailer ~80 mil).

### 6.1 Por que Claude Code com um MCP local

- **Operação única, pequena e supervisionada.** Rodar na sessão do dono permite acompanhar, interromper
  e corrigir; um script batch não ofereceria isso.
- **A reprodutibilidade menor não importa aqui.** O resultado fica commitado e verificado, e só passos
  com texto alterado são reprocessados.
- **Ferramentas em vez de edição livre.** O agente fala com um **MCP stdio local** (`ironman content mcp`)
  e segue uma **skill**. O MCP:
  - entrega lotes pequenos, sem o agente ler o `guide.json` inteiro;
  - expõe o resolvedor de entidades, então o agente não inventa ids;
  - verifica cada envio na hora e devolve os erros, para o agente corrigir no mesmo ciclo;
  - guarda o estado, então dá para parar e retomar em outra sessão.

  O agente nunca escreve no `guide.json`.
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
 │    │                                                                         │
 │    ├──submit(guide, section, anotações)▶ verificador determinístico          │
 │    │      ◀── aceitos / rejeitados com motivo (citação, ref, schema, ordem) ─ │
 │    │      (corrige e reenvia os rejeitados)                                  │
 │    │                                                                         │
 │    └──status(guide) / report(guide) ──▶ cobertura, baixa confiança, pendências│
 └──────────────────────────────────────────────────────────────────────────────┘
          │ grava a cada submit aceito
          ▼
 content/guides/<id>/annotations/<section>.json      (saída verificada + hash por passo)
          │ ironman content annotate merge   (determinístico, sem LLM)
          ▼
 content/guides/<id>/guide.json  ──▶ ironman content validate ──▶ commit
          │
          ▼ boot do serve: carrega e indexa (content_versions/content_steps por hash);
            objetivos avaliados contra player_state. A app já sobe "seedada" pelo conteúdo,
            sem seed de banco à parte (regra 3: o banco só indexa).
```

### 6.3 O que o agente devolve, por passo

- Objetivos com papel e alvo, cada um com a citação do texto.
- Requisitos de item no formato da seção 3.4, com quantidade interpretada ("1k" → 1000).
- Ações de banco com o tipo correto. Por exemplo, "bank everything" é `deposit_all`, não `open_bank`.
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
pelo dono. O que o dono deixar em aberto fica fora do `guide.json` e aparece só no relatório.

### 6.5 Artefatos versionados

| Artefato | Papel |
|---|---|
| `.claude/skills/annotate-guide/SKILL.md` | procedimento, taxonomia fechada, exemplos positivos e negativos, quando e como consultar o dono |
| `content/guides/<id>/annotations/decisions.json` | decisões gerais do dono (pergunta, resposta, escopo), reaplicadas nos lotes seguintes |
| `internal/core/annotate` | lote, verificação e merge: puro, testado por tabela, sem LLM |
| `ironman content mcp` | servidor MCP stdio sobre `internal/core/annotate` e o registry |
| `ironman content annotate verify` / `merge` | os mesmos verificadores por CLI; o `verify` roda na CI sobre o que está commitado |
| `content/guides/<id>/annotations/<section>.json` | saída aceita, com hash do texto de cada passo, modelo e versão da skill |
| `content/guides/<id>/annotations.report.json` | rejeitados, ambíguos, baixa confiança |

A CI nunca chama LLM: só verifica que o que está commitado continua consistente com o texto e o
catálogo. Reprocessar é necessário só para passos cujo hash mudou. Isso também resolve a estabilidade
de ids do BRUHsailer: o importador gera `step_id_aliases` casando o passo antigo e o novo pelos
objetivos, quando o texto muda pouco.

Alternativa considerada: a skill sozinha, com o agente gravando JSON e rodando
`content annotate verify` pelo Bash. Funciona, mas o agente precisaria ler arquivos grandes e só
descobriria erros no fim do lote. Como o núcleo (`internal/core/annotate`) é o mesmo nos dois casos, o
MCP é uma camada fina por cima.

### 6.6 Piloto e critério de aceite

1. **Oziris 1.1** (286 passos): já tem cerca de 120 regras determinísticas para medir concordância.
2. **BRUHsailer 1.1** (18 passos longos): o caso difícil.

Aceite:

- ≥ 95% das citações verificadas;
- concordância alta com as regras existentes (cada divergência explicada no relatório);
- relatório de revisão curto o bastante para leitura humana.

O dono acompanha cada lote, responde as consultas (seção 6.4) e pode aprovar ou corrigir antes do
`merge`. Uma meta do piloto é medir quantas perguntas por seção a skill gera e ajustar a taxonomia e as
decisões gerais para que o volume caia nas seções seguintes. Só depois disso roda o
resto, cerca de 13 seções.

### 6.7 Riscos

| Risco | Mitigação |
|---|---|
| Objetivo inventado | Citação literal obrigatória |
| Extrair demais ("talk to Hans" não é objetivo) | Taxonomia fechada com exemplos negativos |
| Inferência implícita ("do Waterfall") | O agente pergunta ao dono na hora; a resposta vira `confirmed_by: owner` ou uma decisão geral |
| Perguntas demais cansam o dono | Agrupar por passo ou lote, decisões gerais reaplicadas, métrica de perguntas por seção no piloto |
| Variação entre lotes | Prompt fixo, lotes por seção, métrica de concordância |
| Item fora do catálogo | Gerar o catálogo de itens **antes** do piloto (seção 7) |
| Texto do guia sai da máquina | Aceito: uso pessoal; o texto vai só para a Claude, nunca para terceiros |

## 7. Catálogos

- **Itens.** Gerar `content/reference/items.json` a partir dos dados da wiki (ids, nome, forma noteada,
  empilhável, slot de equipamento, família/variante, categorias como food e teleport). O gerador fica em
  `internal/core/entities/catalogbuild`, como os de quests e diaries. O catálogo curado atual (~130 ids)
  fica só como teste.
- **Unlocks e locais.** Catálogos novos, começando pelo que os guias citam (lista sai da extração), não
  pela wiki inteira.

## 8. Migração do que existe

| Hoje | Depois |
|---|---|
| `complete_when` com nome de quest | `objectives` com `Ref`; conversão automática no loader e no `content migrate` |
| `prerequisites` | objetivos com papel `requires` |
| `items` + `Loadout.inventory/equipment` (`LoadoutItem`) | `ItemRequirement` (seção 3.4); a readiness avalia as variantes |
| `step_progress` (por guia) | `objective_progress` (por conta) + `step_marks` (por guia); migration copia `manual_*` e `skipped` |
| `accounts.guide_id` (rota única) | continua como rota ativa; `route_cursor` por guia |
| `goals.ref TEXT` | `goals.objective_key` |
| ids do BRUHsailer por hash | iguais, mais `step_id_aliases` gerado no reimport |
| marcos do `/compare` (heurística no cliente) | objetivos compartilhados servidos pela API |

Nenhuma dessas mudanças altera o contrato de ingest (doc 04, seção 6.4) nem o plugin.

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
  dono e pode concluir passos com evidência; o que ficar em aberto fica fora do `guide.json`.
- **`unlock` e `location` entram já nesta revisão**: no schema e no registry desde o passo 2 da
  sequência, com catálogos povoados pelo que a extração encontrar (cada entrada nova confirmada pelo
  dono), não pela wiki inteira.
- **Cascata `implied` (F1)**: quando um passo conclui por evidência, os passos **manuais** anteriores
  ainda `pending` viram `done_implied`. Limites:
  - só **dentro da mesma seção**;
  - só na **rota ativa**: o passo que dispara precisa ter concluído enquanto aquela rota é a seguida
    (cursor ativo). Um objetivo cumprido seguindo o Oziris não dispara cascata no BRUHsailer;
  - nunca em **tracks** paralelas;
  - `done_implied` aponta para o passo que disparou como evidência, tem visual próprio e só vira feito
    com a confirmação do dono ("confirmar tudo"). Nunca é `auto_done`.

### 9.2 Em aberto (dono)

Nenhuma. O planejamento do modelo está fechado; a execução segue a seção 10.

## 10. Sequência

| # | Entrega | Pronto quando |
|---|---|---|
| 1 | Este doc revisado e decisões fechadas | **feito** (2026-11-04) |
| 2 | Schema: `objectives`, `ItemRequirement`, tipos `unlock`/`location`; loader converte `complete_when` | `content validate` passa nos dois guias, readiness verde |
| 3 | Catálogo de itens gerado | ambíguos e não resolvidos do Oziris caem de forma mensurável |
| 4 | `internal/core/annotate` (lote, verificação, merge), `ironman content mcp`, `content annotate verify/merge` e a skill `annotate-guide` | testes de tabela das checagens; o MCP responde a um lote de exemplo |
| 5 | Piloto (Oziris 1.1, BRUHsailer 1.1) com o agente no Claude Code | critérios da seção 6.6 |
| 6 | Extração completa dos dois guias | relatório revisado pelo dono |
| 7 | Migration de progresso + API (`objective_progress`, `step_marks`, `route_cursor`) | `PATCH` de marca e reavaliação por snapshot testados no Postgres |
| 8 | Web e `/compare` sobre objetivos; metas (F9) | marcos vindos da API, progresso igual nos dois guias |
