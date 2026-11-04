# Ironman Planner — produto, features e UX

Data: 2026-11-03. Autor: design de produto. Escopo: um planner self-hosted para a conta ironman do Felipe (reinício no early/mid game), com possibilidade de abrir para outras pessoas depois.

> Premissas desta rodada: a comparação com o site atual usa a estrutura extraída do próprio ironman.guide (`data/guide/*.json`, 576 steps) e o levantamento em `docs/00-levantamento-inicial.md`; as páginas companheiras do site estão em `data/raw/ironman-guide/pages/`. Os números de cobertura abaixo são estimativas feitas aqui; os números medidos pelo normalizador estão em `docs/05-conteudo-e-normalizacao.md` e prevalecem.

> Adendo de 2026-11-04: o produto passa a seguir duas rotas independentes (Oziris e BRUHsailer), com
> seletor de guia em `/plan` e comparação lado a lado em `/compare`. O doc 09 revisa o modelo de
> domínio em torno de objetivos com progresso compartilhado entre rotas. Isso muda como F1 (status
> derivado dos objetivos), F2 (cursor por rota), F9 (meta = objetivo) e F10 (nós = objetivos) são
> implementados, sem mudar o comportamento descrito aqui.

## 0. Fatos sobre o conteúdo que moldam o produto

| Fato (dados reais) | Consequência de produto |
|---|---|
| 576 steps, 7 seções; a seção 1.1 sozinha tem 286 steps (50%) | Checklist plano por seção não escala; precisa de agrupamento por `location` (573/576 têm) e de um "passo atual" |
| 145 steps com `quest` (111 complete, 33 start, 1 "todas as subquests de RFD"), 47 com `skillGoal`, 37 com `items` | A maior parte do auto-tracking vem de **quests**, não de skills: sem dado de quest (WikiSync ou plugin) o produto fica 80% manual |
| 362 steps (63%) são instrução pura (banco, compra, andar, falar com NPC) | Precisa de uma regra de **conclusão implícita em cascata**: se o step 1.1.31 (quest) foi detectado, os 1.1.22-1.1.30 são presumidos feitos |
| 38 steps `enhanced` (22 na seção 2.0), 17 `newQuest`, 14 `updateNote`, 6 `hcim`, 13 `sailingRelated`, 22 com `phase` | O guia já tem **camadas**; o modelo de conteúdo precisa de camadas de primeira classe, não de flags soltas |
| 119 quests referenciadas, nomes informais ("Dragon Slayer I", "Recipe for Disaster (Evil Dave)", "Alfred Grimhand's Barcrawl" é miniquest) | Tabela de normalização quest → nome canônico do wiki/varbit é trabalho obrigatório e chato do MVP |
| `links` só em 15 steps (imagens do imgur) | Imagens externas vão morrer; cachear localmente |

## 1. Jobs-to-be-done e momentos de uso

**JTBD principal:** "Quando sento para jogar, quero saber em segundos o que fazer e o que levar, sem reler o guia nem conferir manualmente o que já fiz, para gastar o tempo jogando e não planejando."

**JTBDs secundários:** entender o que uma meta grande exige e quanto falta; perceber o que destravou depois de uma sessão; manter o guia atualizado quando o jogo muda; não perder progresso ao trocar de browser/dispositivo.

| # | Momento | Pergunta do usuário | Resposta do produto | Tela |
|---|---|---|---|---|
| M1 | Sentou para jogar 40 min (desktop + celular ao lado) | "O que eu faço agora? O que levo?" | Passo atual + próximos 5, itens, local, link do wiki, aviso de requisito faltando | **Agora** (`/now`) |
| M2 | Durante a sessão, de olho no celular | "Esse já era? Qual o próximo?" | Checklist da seção com auto-check vivo, 1 toque para marcar/pular | **Guia** (mobile) |
| M3 | Depois da sessão | "O que mudou? O que destravou?" | Diff do último sync: levels, quests, steps auto-concluídos, novos "disponíveis" | **Sessões** (`/log`) |
| M4 | Planejamento semanal / escolha de meta longa | "Quero Barrows gloves / Quest cape / Bowfa: o que falta e em que ordem?" | Planner de metas: fechamento de requisitos − feito, ordenado, com estimativa | **Metas** (`/goals`) + **Grafo** |
| M5 | Manutenção do guia (mensal ou quando sai update) | "Esse step ainda vale? Onde encaixo Sailing / quest nova?" | Conteúdo em git com schema validado, camadas, preview de diff | **Editor** / repo |

Ordem de importância para o MVP: M1 > M2 > M3 > M4 > M5. M4 é o motivo do projeto existir além do ironman.guide, mas sem M1-M3 ele não é usado.

## 2. Catálogo de features (MoSCoW por fase)

Fases: **MVP** (jogável por uma pessoa, 1-2 semanas), **v1** (tracking completo + planner), **v2** (extras). M = Must, S = Should, C = Could, W = Won't (por ora).

| Feature | MVP | v1 | v2 | Auto/manual | Fontes |
|---|---|---|---|---|---|
| F1 Checklist persistente com auto-complete e override | M | | | ambos | DB, hiscores, WikiSync/plugin |
| F2 Modo foco "Agora" | M | | | auto | guia + F1 |
| F3 Sync de hiscores + regras `skill`/`boss_kc` | M | | | auto | hiscores |
| F4 Import/export JSON (inclui localStorage do ironman.guide) | M | | | manual | arquivo |
| F5 Dashboard da conta | S | M | | auto | hiscores, WikiSync, Bucket |
| F6 Log de sessões / diff entre syncs | S | M | | auto | snapshots |
| F7 Import WikiSync (quests, diaries, CA, clog) | S | M | | auto | WikiSync |
| F8 Plugin RuneLite próprio (varbits, banco, posição) | | S | M | auto | plugin |
| F9 Planner de metas ("quero X") | | M | | auto+manual | Bucket + guia + estado |
| F10 Explorador do grafo de requisitos | | S | M | auto | Bucket + guia |
| F11 Camadas do guia (base/enhanced/modo/pessoal) | S | M | | manual | git |
| F12 Editor/manutenção do guia | C | S | M | manual | git |
| F13 Itens a guardar / consciência de banco | | S | M | auto c/ plugin | guia + plugin |
| F14 Estimativas de tempo | | C | S | auto | tabela de rates + Bucket |
| F15 Notificações (Discord webhook) | | | C | auto | F6 |
| F16 Multi-conta / multiusuário | | | C | manual | DB |
| F17 Histórico de XP, EHP, gráficos | W | W | W | | WOM já faz |

### F1 Checklist persistente com auto-complete e override manual (MVP)
- **Problema:** hoje o progresso vive em localStorage e cada check é manual; 576 checks à mão é o motivo de o guia ser abandonado.
- **Comportamento:** cada step tem `status ∈ {pending, done_auto, done_manual, done_implied, skipped}` + `evidence` (fonte, timestamp, valor). Regras `complete_when` avaliadas a cada snapshot novo. Clique do usuário sempre vence: um step marcado manualmente nunca é desmarcado pelo sync; um step auto-concluído pode ser "reaberto" pelo usuário e vira `pending` com flag `pinned_open` (o sync não insiste). Desmarcar não apaga evidência.
- **Cascata:** quando um step com evidência forte (quest, skill, diary) conclui, os steps anteriores da mesma seção ainda `pending` viram `done_implied`, com visual próprio e ação "confirmar tudo" em um clique. Sem isso, 63% do guia continua manual.
- **Dados:** guia (regras), `player_snapshot`, `step_progress`.
- **Resolvido (doc 09, 9.1):** a cascata não atravessa seções e só dispara na rota ativa; um objetivo cumprido em outra rota não gera `implied` nesta. Boosts de skill não aparecem no hiscores: regra de skill considera só level base, e isso está certo para o guia.

### F2 Modo foco "Agora" (MVP)
- **Problema:** achar "onde eu estava" numa lista de 286 itens.
- **Comportamento:** passo atual = primeiro `pending` da seção ativa (ou um step fixado pelo usuário). Mostra texto, `note`, `location`, itens (do step e dos próximos N na mesma location), requisito faltando (level/quest) com distância ("faltam 3 levels de Agility"), links do wiki gerados por quest/skill/item, próximos 5 steps colapsados, botões: Feito, Pular, Fixar outro, Abrir no guia. Atualiza sozinho quando o sync conclui o step (toast "1.1.43 detectado: 25 Thieving").
- **Dados:** guia + F1 + último snapshot.
- **Em aberto:** quando o guia manda fazer algo "em paralelo" (birdhouses a cada 50 min, kingdom), mostrar como "tarefas recorrentes" fixas no topo? Proposta: sim, como pílulas, em v1.

### F3 Sync de hiscores (MVP)
- Cron a cada 15 min enquanto houve atividade nas últimas 2 h, senão a cada 2 h; botão "sincronizar agora" com rate limit de 1/min. Endpoint ironman; fallback para o endpoint normal se a conta foi des-ironada (detectar e avisar). Guarda snapshot bruto.
- **Em aberto:** conta precisa estar ranqueada em cada skill para aparecer (level < 15 em algumas skills não aparece no hiscores). Tratar "ausente" como desconhecido, não como 1.

### F4 Import/export (MVP)
- Importar o JSON do localStorage do ironman.guide (mapeando ids `1.1.31` → mesmos ids, já que o extrator preservou), marcar como `done_manual` com fonte `import`. Exportar tudo (estado + snapshots) em JSON. É o seguro contra perder dados e o caminho para migrar hoje mesmo.

### F5 Dashboard da conta (S no MVP, M v1)
- **Problema:** ver de relance "onde a conta está" em relação ao guia, não em absoluto.
- **Comportamento:** grade das 24 skills (inclui Sailing) com level atual e **próximo marco do guia** para aquela skill (próximo `skillGoal` pendente: "Thieving 53 → próximo: 99 em 1.3.x", "Agility 47 → 50 (1.2.17)"), QP atual vs. QP exigido por metas (Quest cape, RFD), matriz de diaries 12 áreas × 4 tiers, CA tier, KC dos bosses citados no guia, barra por seção do guia, idade do último sync por fonte.
- **Dados:** hiscores, WikiSync/plugin, Bucket (QP por quest), guia.

### F6 Log de sessões / diff (S no MVP, M v1)
- **Comportamento:** cada snapshot novo gera um diff: skills que subiram (com XP), KC, quests que mudaram de estado, steps concluídos por auto/cascata, steps que ficaram **disponíveis** (requisitos agora atendidos), metas que avançaram (%). Agrupa em "sessões" (gap > 90 min fecha uma sessão). Lista cronológica + detalhe.
- **Em aberto:** com polling de 15 min o diff é grosseiro; o plugin (F8) pode mandar evento ao fechar o cliente. Suficiente.

### F7 Import WikiSync (S MVP, M v1)
- Importador opcional com aviso claro do pedido do wiki; polling baixo (a cada sync manual + 1×/h). Alimenta `quest`, `quest_start`, `diary`, `diary_task`, `combat_achievement`, `collection_log_item`. Projetado para ser trocado pelo F8 sem mudar regras: ambos produzem o mesmo snapshot normalizado `{quests, diaries, ca, clog, varbits}`.

### F8 Plugin RuneLite próprio (S v1, M v2)
- Copia o modelo do WikiSync: baixa manifest de varbits do seu servidor, envia deltas com token. Acrescenta: itens no banco/inventário (só os ids que o guia referencia, não o banco inteiro), região atual, evento de logout. É o que leva o auto-tracking de ~42% para ~70% (seção 5).
- **Em aberto:** sideload vs. Plugin Hub (Hub exige revisão e licença; sideload basta para uso pessoal).

### F9 Planner de metas (M v1)
- **Problema:** "quero Barrows gloves" exige 10 subquests, ~20 quests pré-requisito, levels, itens; o ironman.guide só oferece a ordem fixa.
- **Comportamento:** o usuário escolhe uma meta de um catálogo (quest, item/unlock, diary tier, level, "seção do guia") ou cria uma meta custom com requisitos manuais. O sistema calcula o fechamento transitivo no grafo, subtrai o que está feito, ordena topologicamente respeitando a ordem do guia quando há empate (o guia é a heurística de eficiência), e exibe: lista ordenada com status, % de progresso, "gargalo" (o requisito mais distante, ex.: 70 Agility), e os steps do guia ligados a cada requisito. Metas ficam no dashboard com barra.
- **Dados:** Bucket (requisitos de quest e diary), guia (steps ↔ quest/skill), estado.
- **Em aberto:** requisitos de item (ex.: "precisa de 1 anti-dragon shield") vêm do Bucket só parcialmente; começar só com skill/quest/QP e anotar itens no guia.

### F10 Explorador do grafo (S v1, M v2)
- **O que mostrar:** nós = quests, diaries (tier), skill-levels (um nó por skill com o level máximo exigido no recorte, não um nó por level), unlocks/itens-chave, metas. Arestas = `requires`. Cores por estado (feito, disponível, bloqueado), espessura por "quantas metas passam por aqui".
- **Como evitar o hairball:** nunca renderizar o grafo inteiro. Entrada sempre por um foco (meta, quest ou skill) com profundidade 1-2 (ego-graph); nós feitos colapsados em um contador por padrão ("12 requisitos concluídos"); layout em camadas (dagre/ELK, esquerda → direita, raiz à direita); filtros: tipo de nó, esconder feitos, só caminho crítico, só o que é alcançável nesta semana; busca; clique expande, duplo clique refoca; painel lateral com detalhes e botão "virar meta". Sempre com **visão lista equivalente** (árvore indentada) para celular e para quem não gosta de grafo.
- **Dados:** mesmos do F9. Grafo calculado no servidor e servido como JSON do recorte.

### F11 Camadas do guia (S MVP, M v1)
- Conteúdo = `base` (Oziris) + overlays: `enhanced` (conteúdo 2025/2026, hoje `enhanced: true`), `mode` (`hcim`, `uim`, `gim`: steps extras, steps removidos, notas), `tracks` paralelos (Sailing), `personal` (notas, reorder, skip, steps próprios, guardados no DB e nunca no conteúdo). Toggle por camada na UI, como o Enhanced Mode, mas composto.
- **Em aberto:** UIM precisa de steps de banco reescritos, não só flags; começar só com `hcim` (6 steps já marcados).

### F12 Editor / manutenção (C MVP, S v1)
- MVP: YAML no repo, validação por schema (ids estáveis, quests normalizadas, skills válidas), `make check`, hot-reload. v1: tela de edição de step que abre PR/commit via API do git (ou salva em branch de rascunho), com preview do diff e relatório "steps cujas regras nunca disparam" e "quests sem mapeamento". v2: import semiautomático de novos steps a partir de `updateNote`.

### F13 Itens a guardar (S v1)
- 37 steps com `items` + dezenas de "bank 7 logs for later": gerar uma lista "não vender/não dropar" derivada do guia, com o step que vai consumir cada item. Com plugin: alerta quando o item não está no banco. Sem plugin: lista estática com check manual.

### F14 Estimativas de tempo (C v1, S v2)
- Por step: skill → XP faltante × taxa do método do step (tabela editável `rates.yaml`, ~40 entradas cobrem o guia); quest → duração oficial do wiki (Bucket tem `length`); boss → KC × tempo médio. Mostrar só faixas ("~2-3 h"), nunca precisão falsa. Soma por meta e por seção.

### F15 Notificações (C v2)
- Webhook Discord no fim de cada sessão com o diff do F6 e "próximo passo". Zero UI além de uma URL nas configurações.

### O que NÃO construir
| Não fazer | Quem já faz melhor | O que fazer em vez disso |
|---|---|---|
| Walkthrough passo a passo dentro da quest | Quest Helper (RuneLite) | Linkar a quest; nosso step é "faça a quest", não "fale com o NPC X" |
| Página de quest com requisitos, itens, recompensas | OSRS Wiki | Linkar; puxar só requisitos estruturados via Bucket |
| Gráficos de XP ao longo do tempo, EHP/EHB, ranks | Wise Old Man | Linkar o perfil WOM; no máximo embed do link |
| Navegador de collection log | Collection Log plugin / TempleOSRS | Usar só os itens que o guia pede como regra |
| Gear ladder e boss ladder genéricos | ironman.guide/gear, wiki, Oziris doc | Linkar; o guia 2.0 já diz o que farmar e quando |
| Preços de GE, calculadoras de drop | wiki, plugins | Nada |
| Guia dentro do cliente (plugin que mostra os steps) | IronGuide (B0aty) existe; mas Quest Helper ocupa esse espaço | Celular ao lado do monitor resolve M2 melhor e mais barato |

## 3. Arquitetura da informação e UX por tela

### Sitemap
```
/login
/setup            (primeiro acesso: RSN, modo, camadas, import, 1º sync)
/now              Agora (foco)                         [mobile: tela inicial]
/guide            Seções (progresso por seção, tracks)
/guide/:section   Checklist da seção (agrupado por location)
/dashboard        Conta (skills vs marcos, QP, diaries, CA, bosses)
/log              Sessões e diffs
/goals            Metas  /goals/:id  Plano da meta
/graph            Explorador do grafo (?focus=quest:Lunar_Diplomacy&depth=2)
/items            Itens a guardar
/settings         Fontes (hiscores, WikiSync, plugin token), camadas, export, Discord
/editor           (v1) edição de steps / relatório de conteúdo
```
Navegação principal (desktop): barra lateral fixa com 7 itens + status do sync no rodapé. Mobile: barra inferior com **Agora, Guia, Conta, Mais**.

### Tela 1 — Agora (`/now`)
```
┌─────────────────────────────────────────────────────────────┬──────────────────┐
│ Seção 1.3 · Fairy rings, 43 prayer...   ▮▮▮▮▮▮░░░░ 61/111   │ SYNC  há 4 min ● │
├─────────────────────────────────────────────────────────────┼──────────────────┤
│ PASSO ATUAL  1.3.62                       [Fixar outro ▾]   │ LEVAR            │
│ ┌───────────────────────────────────────────────────────┐   │ ☐ Rope           │
│ │ 📍 Catherby                                           │   │ ☐ Dwellberries   │
│ │ Complete Fairytale II - Cure a Queen                  │   │ ☐ Hangover cure  │
│ │ 🔒 falta: Herblore 57 (você: 54, ~45 min)             │   │ (itens dos próx. │
│ │ 📝 note: ...                                          │   │  3 steps aqui)   │
│ │ [wiki: Fairytale II]  [Quest Helper abre no cliente]  │   ├──────────────────┤
│ │ [✓ Feito]  [↷ Pular]  [≡ Abrir no guia]               │   │ RECORRENTES      │
│ └───────────────────────────────────────────────────────┘   │ ⟳ Birdhouses 50m │
│ PRÓXIMOS                                                    │ ⟳ Kingdom        │
│  ○ 1.3.63  Catherby   Pesque até 60 Fishing   auto·hiscores │ ⟳ Herb run       │
│  ○ 1.3.64  Various    ...                                   ├──────────────────┤
│  ◌ 1.3.64c Varlamore  Calcified rocks (enhanced)            │ METAS            │
│  ▸ mostrar mais 5                                           │ B-gloves 48% ▮▮▮ │
└─────────────────────────────────────────────────────────────┴──────────────────┘
```
- **Estados:** *vazio* (sem RSN: card "configure sua conta" + checklist 100% manual funciona mesmo assim); *carregando* (skeleton do card); *sincronizado* (ponto verde, "há 4 min"); *desatualizado* (>24 h ou fonte falhou: faixa amarela "dados de 3 dias; requisitos podem estar errados", botão sincronizar); *bloqueado* (passo atual tem requisito não atendido: card mostra o gargalo e sugere "próximo passo disponível": 1.3.63).
- **Mobile:** só a coluna esquerda; "Levar" vira acordeão abaixo do card; botões Feito/Pular com 48 px; passo atual fixado no topo ao rolar.

### Tela 2 — Checklist da seção (`/guide/:section`)
```
┌ 1.1 Early quests, wintertodt and ardy cloak 1 ───────── 212/286 ▮▮▮▮▮▮▮░░ ┐
│ Filtros: [Pendentes] [Todos] [Auto] [Manual] [Enhanced ✓] [HCIM ☐]  🔍      │
│ ▸ Lumbridge (12)  ✓ 12/12  ──────────────────────────── colapsado          │
│ ▾ Draynor Village (6)  4/6                                                 │
│   ✓ 1.1.27 Dig the next clue for X marks the spot             ⋯ presumido │
│   ✓ 1.1.31 Finish X marks the spot ... [X Marks the Spot]     ● auto·WS   │
│   ☐ 1.1.29 Buy 5 wine from Fortunato                          manual      │
│   ◌ 1.1.29a (enhanced) ...                                                │
│ ▾ Falador (9)  0/9                                                         │
│   ☐ 1.1.34 Start The Knight's Sword   🔒 precisa: Mining 10 (você 7)       │
│ ...                                                                        │
│ [Confirmar 8 presumidos]                                                   │
└────────────────────────────────────────────────────────────────────────────┘
```
- Linha de 36 px no desktop, 52 px no celular. Clique na linha expande nota, itens, links, evidência ("detectado via hiscores em 03/11 14:20: Thieving 25"). Checkbox com menu longo-toque: Feito / Pular / Reabrir / Fixar como atual / Nota pessoal.
- Grupos por `location` consecutiva (não por location global, para preservar a ordem do guia). Grupo 100% concluído colapsa sozinho.
- **Estados:** *stale* mostra os checks auto com ícone de relógio; *conflito* (usuário marcou feito mas a regra diz que não: ex. marcou "25 Thieving" com 23) mostra badge âmbar, nunca desmarca.

### Tela 3 — Seções (`/guide`)
Cards por seção com barra, "passo atual", contagem de pendentes disponíveis vs. bloqueados, e tracks paralelos (Sailing) separados visualmente. Toggle de camadas aqui e em Configurações.

### Tela 4 — Conta (`/dashboard`)
```
┌ SKILLS (24)  Total 1.234 · Combat 78 ───────────────┐ ┌ GUIA ───────────────┐
│ [atk 60 →65 2.0.13] [str 60 →65] [def 45 →70 2.0.x]   │ │ 1.1 ▮▮▮▮▮▮▮▮▮░ 92% │
│ [rng 50 ✓] [pray 43 ✓] [mag 59 →60 1.4.41] ...        │ │ 1.2 ▮▮▮▮░░░░░░ 41% │
│ cada célula: ícone, level, seta p/ próximo marco, id  │ │ ...                 │
├──────────────────────────────────────────────────────┤ ├ METAS ──────────────┤
│ QUESTS  QP 187/  RFD 7/10 subq · Quest cape 69%       │ │ B-gloves  48%       │
│ DIARIES  matriz 12 áreas × E/M/H/El (✓ ◐ ☐ 🔒)        │ │ Fairy rings ✓       │
│ CA  Medium 61/..  ·  BOSSES do guia: WT 312, Temp 45  │ │ + nova meta         │
└──────────────────────────────────────────────────────┘ └─────────────────────┘
Fontes: hiscores há 4 min ● · WikiSync há 2 h ● · plugin — (não configurado)
```
- Célula de skill é o átomo visual do produto; reaparece em Metas e Grafo. Diaries e quests ficam cinza-hachurado quando a fonte (WikiSync/plugin) não existe: "sem dados, configure em Fontes".

### Tela 5 — Sessões (`/log`)
Lista: "Ter 03/11 · 20:10-21:35 · +3 levels · 2 quests · 14 steps". Detalhe em duas colunas: *o que mudou* (skills com Δ XP, KC, quests, diaries) e *o que isso causou* (steps concluídos auto/cascata, steps que ficaram disponíveis, metas que avançaram). Estado vazio: "faça um sync e jogue; o primeiro diff aparece no próximo". Snapshot com erro de fonte aparece como linha cinza "hiscores indisponível".

### Tela 6 — Meta (`/goals/:id`)
```
┌ Barrows gloves (RFD) ───── 48% · ~14 h restantes (estimativa, v2) ──────┐
│ GARGALO: Agility 56 → precisa 70 (Culinaromancer? não: Fairytale II)     │
│ ORDEM SUGERIDA (segue o guia quando possível)                            │
│  ✓ Cook's Assistant               ✓ Fishing Contest                      │
│  ● Lost City  (disponível)   steps: 1.3.40                               │
│  🔒 Legends' Quest  falta: Agility 50, Crafting 50, 4 quests  [expandir]  │
│  ...                                                                     │
│ [Ver no grafo]  [Fixar próximo step disponível em Agora]                 │
└──────────────────────────────────────────────────────────────────────────┘
```
Catálogo de metas (`/goals`): busca por quest/item/diary/skill + presets do guia (Ardy cloak 1, Fairy rings, 43 Prayer, Graceful, B-gloves, Fire cape, Quest cape, Bowfa).

### Tela 7 — Grafo (`/graph`)
Topo: foco (busca), profundidade (1/2/3), filtros (tipo, esconder feitos, só caminho crítico). Centro: DAG em camadas, nós compactos (ícone + nome + level), cor por estado, feitos colapsados em "+12 ✓". Direita: painel do nó selecionado (requisitos, dependentes, steps do guia, botão "virar meta"). Rodapé: alternar para **lista em árvore** (padrão no celular). Estado vazio: "escolha um foco" com 6 sugestões. Nunca abrir o grafo completo nem por "mostrar tudo".

### Tela 8 — Configurações
Conta (RSN, modo iron/HCIM/UIM/GIM, endpoint detectado), Fontes (hiscores on/off e intervalo; WikiSync on/off com aviso do wiki; token do plugin + último POST), Camadas (enhanced, hcim, sailing track, pessoal), Dados (import localStorage, export JSON, reprocessar regras), Integrações (Discord URL).

### Fluxos de navegação
1. **Primeiro acesso:** login → `/setup` (RSN → detectar modo pelo hiscores → camadas → "tem progresso no ironman.guide? cole o JSON" → 1º sync com progresso visível) → `/now`.
2. **Sessão de jogo:** `/now` → (joga) → toast "detectado" → card avança sozinho → fim: `/log` destacado com badge "nova sessão".
3. **Meta:** `/dashboard` → "+ meta" → busca → `/goals/:id` → "Ver no grafo" (foco já aplicado) → nó → "fixar step em Agora".
4. **Bloqueio:** `/now` mostra 🔒 → "ver o que falta" abre o mini-plano do requisito (mesmo componente de meta) → "próximo disponível" pula para o step viável.

## 4. Princípios de design e direção visual

**Princípios**
1. **Densidade calma:** muita informação por tela, pouca cor. Uma hierarquia (passo atual) grita; o resto sussurra.
2. **Legível a 1 m de distância no celular:** texto do passo atual ≥ 18 px, contraste AA no escuro, ícones de skill com rótulo.
3. **Nunca mentir sobre certeza:** todo check mostra de onde veio e quando; dado velho parece velho.
4. **O usuário manda:** auto-detecção sugere, cascata presume, o clique decide. Nada é desfeito pelo sistema sem aviso.
5. **Mesmos átomos em todo lugar:** célula de skill, pílula de quest, badge de estado reaparecem idênticos em todas as telas.

**Visual**
- Tema escuro padrão (fundo `#0E1116`, superfícies `#161B22`/`#1C2230`, texto `#E6EDF3`), tema claro opcional. Acento único âmbar-dourado (`#D4A72C`) só para o passo atual e metas; verde (`#3FB950`) para feito, azul-acinzentado para auto-fonte, âmbar para stale/conflito, vermelho só para erro de fonte. Nada de cor como único sinal: sempre glifo + texto.
- Tipografia: Inter (UI) + tabular numerals para levels/XP; sem fonte "RuneScape" no corpo (ilegível em densidade), no máximo no logo.
- Ícones de skill, quest e itens do OSRS Wiki, cacheados localmente, com rodapé de atribuição: "Imagens © Jagex, via Old School RuneScape Wiki, CC BY-NC-SA 3.0" + link. Isso restringe uso comercial do produto; aceitável para self-hosted.
- Layout: 1 coluna até 720 px, 2 colunas (conteúdo + painel) acima de 1024 px, sidebar fixa acima de 1280 px.

**Estados de conclusão (visual)**
| Estado | Checkbox | Badge | Tooltip/evidência |
|---|---|---|---|
| `done_auto` | ✓ cheio verde | ● + fonte (HS / WS / RL) | "Thieving 25 via hiscores, 03/11 14:20" |
| `done_manual` | ✓ cheio verde com glifo de mão | manual | "marcado por você, 02/11" |
| `done_implied` | ✓ pontilhado, verde apagado | presumido | "presumido porque 1.1.31 foi detectado" + [confirmar] |
| `done_auto` + stale | ✓ verde dessaturado | ◷ | "última verificação há 3 dias" |
| conflito | ✓ verde + contorno âmbar | ! | "você marcou, mas o hiscores mostra 23/25" |
| `skipped` | — tachado cinza | pulado | motivo opcional |
| `pending` disponível | ○ | | |
| `pending` bloqueado | ○ com 🔒 | falta X | requisito inline com distância |

## 5. Regras de auto-conclusão: tipos e cobertura

Contagem sobre os 576 steps (campos explícitos; a terceira coluna vem de uma classificação por texto dos 388 steps sem `quest`/`skillGoal`, portanto é estimativa ±10%).

| Sinal no step | Steps | Fonte mínima | Tipo de regra |
|---|---|---|---|
| `skillGoal` explícito | 47 | hiscores | `skill_level {skill, level}` |
| level só no texto ("get 70 agility", "at 58 slayer") | ~17 | hiscores (após anotar) | `skill_level` |
| boss/minigame com KC (Wintertodt, Tempoross, Gauntlet, ToA, GWD, Nex, Yama...) | ~19 | hiscores (após anotar KC alvo) | `boss_kc {boss, min_kc}` / `activity_score` |
| `quest` + `complete` (+1 "todas as subquests RFD") | 112 | WikiSync ou plugin | `quest_state = FINISHED` (RFD = `all` das 10 subquests) |
| `quest` + `start` | 33 | WikiSync ou plugin | `quest_state >= IN_PROGRESS` |
| diary tier ("Ardy easy", "all easy and medium") | 3 (expande para ~26 tiers) | WikiSync ou plugin | `diary_tier {area, tier}` |
| task única de diary ("mine 2 blurite... for diary") | ~11 | WikiSync ou plugin | `diary_task {area, tier, task_id}` |
| item de collection log (2.0: Bowfa seeds, Fang, Torva...) | ~10 | WikiSync (clog) ou plugin | `collection_log_item {item, qty}` |
| unlock via varbit (POH, Kourend, barcrawl, kingdom, fairy rings, miniquests, minigame tele) | ~25 (dos 40 classificados) | **plugin** | `varbit {id, op, value}` |
| itens (`items` 37 + "bank/keep X" ~60) | ~70 | **plugin** (banco+inv) | `item_owned {item, qty, where}` |
| deslocamento ("walk to Falador") | ~59 | plugin (região) — baixo valor | `region_visited` ou só cascata |
| instrução pura (vender, dropar, falar, cavar clue, usar lamp) | ~118 | nenhuma | `manual` + cascata |
| combat achievement | 0 hoje (2.0 Jad/Inferno candidatos) | WikiSync ou plugin | `combat_achievement` |

Meta-regras necessárias além dessas: `all`/`any` (composição), `implied_by_successor` (cascata dentro da seção), `manual_ok` (padrão true), `never_auto` (steps tipo "use authenticator").

**Cobertura acumulada estimada**

| Cenário | Steps auto-detectáveis | % de 576 |
|---|---|---|
| Só hiscores (skills + KC, após anotar levels/KC no texto) | ~83 | **~14%** |
| + WikiSync (quests, diaries, clog) | ~250 | **~43%** |
| + plugin RuneLite próprio (varbits, itens, região) | ~400 | **~70%** |
| + cascata (`implied`) sobre qualquer um dos cenários acima | ~540 | **~94%** (presumidos, confirmáveis em 1 clique) |

Leitura: hiscores sozinho é pouco; **quests são o sinal dominante** e dependem do cliente. A cascata é a feature mais barata com maior impacto e deve estar no MVP. O plugin vale mais pelos itens e unlocks do que por quests (que o WikiSync já dá).

Trabalho de conteúdo implícito nesses números: anotar `skillGoal` em ~17 steps, `kc` em ~19, normalizar 119 nomes de quest, mapear ~11 tasks de diary e ~25 varbits. Estimativa: 1-2 dias de edição de YAML, parcialmente scriptável.

## 6. Riscos e decisões em aberto

**Riscos**
| Risco | Impacto | Mitigação |
|---|---|---|
| Wiki restringir ou cortar a API do WikiSync | perde quests/diaries (o sinal dominante) | abstrair fonte desde o MVP; plugin próprio em v1, não v2 |
| Rate limit / instabilidade do hiscores oficial | checks stale | cache, backoff, estado stale visível, nunca marcar errado |
| Licença do conteúdo do Oziris/ironman.guide | ok para uso pessoal; problema se publicar | manter crédito; se abrir, pedir autorização ou trocar pelo guia CC do wiki |
| Ícones do wiki (CC BY-NC-SA) | bloqueia uso comercial | atribuição no rodapé; aceitável |
| Cascata presume errado (usuário pulou a seção) | falsos "feitos" | estado `implied` visualmente distinto, sem contar como feito em metas até confirmar |
| Grafo vira hairball ou brinquedo | feature cara e não usada | entrar sempre por foco; lista em árvore como cidadã igual; medir uso antes de investir em v2 |
| Normalização de quests mal feita | regras nunca disparam silenciosamente | relatório de "regras órfãs" no editor + teste que valida todo nome contra o Bucket |
| Contas abaixo do rank mínimo não aparecem no hiscores | skills "desconhecidas" | tratar ausente como desconhecido e permitir manual |

**Decisões para o dono**
1. **Cascata no MVP** (recomendado) ou só a partir da v1?
2. **WikiSync no MVP** apesar do pedido do wiki, ou pular direto para plugin próprio (adia quests em semanas)?
3. Grafo: começar pela **lista em árvore** (v1) e deixar a visualização para v2, ou os dois juntos?
4. Qual o conjunto inicial de **metas pré-definidas** (sugestão: os títulos das 7 seções + B-gloves, Fire cape, Quest cape, Bowfa)?
5. Modos: só **ironman + HCIM** no início (6 steps já anotados) e UIM/GIM "won't" até alguém pedir?
6. Tempo estimado: entra em v1 como "could" com tabela de rates mantida à mão, ou fica fora até o tracking estar confiável?
7. Mobile: PWA instalável (recomendado, zero custo extra) ou só site responsivo?
8. Publicar o projeto como open source desde o início (implica resolver a licença do conteúdo do guia antes) ou manter privado enquanto for de uso pessoal?
