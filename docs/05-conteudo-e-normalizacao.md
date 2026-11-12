# 05 — Conteúdo companheiro e normalização do guia

Data: 2026-11-03. Fonte: https://ironman.guide (guia de eficiência de **OzirisLoL**, republicado e mantido por ironman.guide).

## 1. Resumo

- As 12 páginas companheiras e os 18 artigos do ironman.guide foram baixados e convertidos para Markdown limpo em `data/raw/ironman-guide/pages/` (30 arquivos, ~850 KB), com front matter de atribuição. Conversor reutilizável: `tools/extract_page.py`.
- O guia principal (7 seções, 576 passos, já extraído antes em `data/guide/*.json`) foi normalizado para `data/guide/normalized/guide.json` pelo `tools/normalize_guide.py`, com regras de conclusão avaliáveis por máquina. Resultado: **209 passos (36%) rastreáveis automaticamente** (50 só com hiscores, 159 com WikiSync/plugin) e **367 manuais**; 43 regras vieram de inferência no texto e estão marcadas como tal.
- `tools/validate_guide.py` roda limpo (0 erros, 2 avisos esperados).
- Bônus: a ordem de quests do `/quests` (215 quests em 6 fases, com QP, requisitos e recompensas) virou `data/guide/normalized/quest-order.json` via `tools/extract_quest_order.py`.
- Achado técnico: o site responde ao header `RSC: 1` com o payload React Flight (`text/x-component`), mas as páginas companheiras **não** carregam props estruturadas; só o `/guide` tem. Não há rotas `/api/*`.

## 2. O que foi extraído e onde

### 2.1 Páginas companheiras (Markdown)

| Página | Arquivo | Tamanho | Conteúdo |
|---|---|---|---|
| /quests | `pages/quests.md` | 62 KB | 215 quests em 6 fases, cards com QP, Skill Reqs, Quest Prereqs, Key Rewards; cadeias de quests; top XP |
| /gear | `pages/gear.md` | 34 KB | progressão melee/ranged/magic em tabelas GFM, 196 links para o wiki |
| /bosses | `pages/bosses.md` | 20 KB | ladder de 37 bosses (early/mid/late), estilo, requisitos, drops prioritários |
| /hcim, /uim, /gim | `pages/hcim.md`, `uim.md`, `gim.md` | 6 / 29 / 4 KB | dicas por modo; HCIM tem regras de morte 2026 |
| /herblore, /crafting | `pages/herblore.md`, `crafting.md` | 30 / 25 KB | guias de skill (métodos por faixa de nível) |
| /money-making, /afk, /best-ironman-guide | `pages/money-making.md`, `afk.md`, `best-ironman-guide.md` | 7 / 39 / 9 KB | listas e tabelas |
| /articles (índice) | `pages/articles.md` | 6 KB | lista dos 18 artigos |
| /articles/* (18) | `pages/articles-<slug>.md` | 6–49 KB cada | slayer, magic, prayer, sailing, fractured-archive, construction, fishing, fletching, smithing, barrows, barrows-gloves, fire-cape, zulrah, avas-assembler, while-guthix-sleeps, maggot-king, shipwreck-salvaging, wyrmscraig |

Cada `.md` começa com front matter: `title`, `source` (URL), `description`, `author` (OzirisLoL, ironman.guide), `fetched_at`, `license_note`.

O que o conversor preserva: h1–h6, parágrafos, listas (inclusive numeração embutida em `<span>`), tabelas (GFM), `details/summary`, negrito/itálico/código, links (relativos viram absolutos; links do wiki ficam intactos), imagens com alt. O que descarta: header fixo, navs (breadcrumb, barras de fase, "On this page", prev/next), footer, scripts, SVG, blocos de anúncio/afiliado (Creator Crafted) e comentários `<!-- -->` que o React usa para separar nós de texto (o texto é rejuntado).

HTML bruto baixado nesta rodada: `/tmp/claude-0/-home-user/b64077c9-cf41-57b8-85ac-60dc4bd14b8c/scratchpad/dl/` (efêmero; os `.md` são a cópia durável).

### 2.2 Dados normalizados

| Arquivo | Conteúdo |
|---|---|
| `data/guide/normalized/guide.json` | guia normalizado (schema abaixo) |
| `data/guide/normalized/stats.json` | estatísticas, lista de revisão, locais, quests |
| `data/guide/normalized/quest-order.json` | ordem de quests do /quests (6 fases, 215 quests, 60 milestones) |

### 2.3 Scripts (`tools/`)

| Script | Uso |
|---|---|
| `extract_page.py` | `python3 -I tools/extract_page.py --batch 'dir/*.html' --out-dir data/raw/ironman-guide/pages` ou arquivo único com `-o`. Depende de `beautifulsoup4` (já instalado; `markdownify` não é usado: um walker próprio controla melhor a sopa de `div` do Tailwind). |
| `normalize_guide.py` | `python3 -I tools/normalize_guide.py [--print]` lê `data/guide/*.json` (+ `quests.md` como catálogo de nomes) e escreve `guide.json` + `stats.json`. |
| `validate_guide.py` | `python3 -I tools/validate_guide.py` — ids únicos, skills canônicas, quests start↔complete, referências de `requires`, áreas de diário e bosses conhecidos. Exit 1 em erro. |
| `extract_quest_order.py` | `python3 -I tools/extract_quest_order.py` — `quests.md` → `quest-order.json`. |

## 3. Achados sobre a API do site

| Teste | Resultado |
|---|---|
| `GET /quests` com header `RSC: 1` | 200, `text/x-component`, 465 KB: payload React Flight (linhas `id:JSON`). Útil para obter só a árvore de componentes sem o HTML, mas o conteúdo continua sendo elementos React (`["$","h3",null,{"children":"Cook's Assistant"}]`), não um objeto de dados. |
| `GET /quests?_rsc=1` sem header | devolve o HTML normal (o parâmetro sozinho não muda nada). |
| `GET /api/quests` | 404 (página 404 do Next). Nenhuma rota `/api` descoberta. |
| `/sitemap.xml` | lista todas as rotas: as 12 páginas, 7 capítulos do guia, 18 artigos e também `/drop-oracle`, `/drop-calculator`, `/leagues/*` (não baixados, fora do escopo). |
| `/robots.txt` | `Allow: /` para todos. |
| Props estruturadas | só `/guide` e `/guide/<capítulo>` têm `{"sections":[...]}` no payload (já extraído). `/quests`, `/gear`, `/bosses` renderizam cards diretamente; por isso o caminho HTML→Markdown. |
| Dependências do cliente | o HTML faz `preconnect` para `api.wiseoldman.net` e `oldschool.runescape.wiki`: o tracker do site consome Wise Old Man no browser, consistente com o levantamento do doc 00. |

## 4. Schema normalizado (v0.1)

```json
{
  "schema_version": "0.1",
  "guide": {"id": "oziris-efficiency", "title": "OSRS Ironman Efficiency Guide (OzirisLoL)",
            "source": {"url": "https://ironman.guide/guide", "author": "OzirisLoL",
                       "maintainer": "ironman.guide", "extracted_at": "2026-11-03", "license_note": "..."}},
  "sections": [{
    "id": "1.1", "slug": "early-game", "title": "...", "description": "...", "track": false,
    "related": [{"href": "/articles/osrs-ironman-magic-guide", "label": "Magic guide (1-99)"}],
    "steps": [{
      "id": "1.1.11",
      "text": "Light the 4 logs until 15 Firemaking",
      "location": "Lumbridge Castle",
      "note": null, "update_note": null,
      "items": [], "links": [],
      "layer": "classic",
      "modes": ["ironman", "hcim", "uim", "gim"],
      "tags": ["skill"],
      "phase": null,
      "quest": null,
      "complete_when": [{"type": "skill_level", "skill": "firemaking", "level": 15,
                         "source": "skillGoal", "confidence": "high"}],
      "prerequisites": [],
      "requires": [],
      "completes_in": null,
      "track_via": "hiscores"
    }]
  }]
}
```

Semântica:

- `complete_when`: lista de regras em **AND**. Passo sem regra recebe `[{"type":"manual"}]`.
- Tipos de regra: `skill_level {skill, level}`, `quest_state {quest, state: FINISHED | IN_PROGRESS, or_finished}`, `diary_tier {area, tier}` (área `all` = todas), `boss_kc {boss, kc}` (nomes do hiscores), `manual`. `combat_achievement` está previsto no validador, mas nenhum passo gerou.
- Cada regra tem `source` (`skillGoal`, `questStatus`, `quest`, `text`); as vindas do texto têm `inferred: true`, `confidence: high|medium` e `match` (a frase que disparou).
- `prerequisites`: níveis que o texto diz que você precisa **antes** do passo ("Req 42 slayer", "(needs 49 Construction)", "At 58 slayer:"). Informativo, não bloqueia conclusão. 20 passos têm.
- `layer`: `classic` (texto original do Oziris, 538) ou `enhanced` (passo adicionado pelo ironman.guide, 38).
- `modes`: sempre os quatro. `hcim: true` da fonte virou a tag `hcim` (relevância, não exclusividade), como pedido.
- `tags`: `quest`, `skill`, `diary`, `diary-task` (tarefa individual de diário mencionada, sem tier), `boss:<nome>` (boss citado sem contagem), `sailing`, `new-quest`, `hcim`, `uim`, `phase:<mid|late|endgame>`.
- `quest`: `{name, status, wiki}` com nome canonizado contra o catálogo do `/quests` (nomes do wiki; subquests do RFD como `Recipe for Disaster/Freeing Evil Dave`).
- Dependências: no passo de *start* de uma quest, `completes_in` aponta para o passo que a conclui; no passo de conclusão, `requires` contém o id do start. 31 pares ligados. Ordem sequencial é implícita e não é repetida.
- `track_via`: fonte mais barata capaz de avaliar **todas** as regras do passo: `hiscores` (só skill_level/boss_kc), `wikisync` (há quest_state/diary_tier), `manual`.

Skills canônicas (minúsculas): attack, strength, defence, ranged, prayer, magic, runecraft, construction, hitpoints, agility, herblore, thieving, crafting, fletching, slayer, hunter, mining, smithing, fishing, cooking, firemaking, woodcutting, farming, sailing. Abreviações do Oziris (herb, rc, fm, wc, con, str, range...) são mapeadas; quando a regra vem do texto com abreviação, a confiança cai para `medium`.

## 5. Heurísticas de inferência no texto

| Padrão | Exemplo | Regra |
|---|---|---|
| `<verbo> [level] N <skill>` com verbo em until/till/get/reach/train/finish (high) ou have/for/to (medium) | "until 15 Firemaking", "get 43 prayer", "for 13 magic" | `skill_level` |
| encadeamento "and N skill" / "and skill" | "until 37 Range and 18 slayer", "for 9 Hunter and Slayer" | uma regra por skill |
| `do <skill> ... until level N` | "Keep doing slayer until level 75" | `skill_level` |
| `<skill> until/to N` não seguido de outra skill | "Train Sailing to 12" (mas **não** "Do Slayer until 60 attack") | `skill_level` (medium) |
| `<área> <tier> diary/tasks`, `<tier> <área> diary`, `ardy cloak N` | "Varrock easy diary", "Fremmy hard diary", "Ardy easy tasks" | `diary_tier` |
| "all easy and medium diaries" | 1.5.10 | `diary_tier` área `all` (medium) |
| lista "quests...: A, B, C" e "complete/finish/do/start/continue <quest>" | 1.5.5, 1.4.29, "finish vampire slayer" | `quest_state` (medium); "continue"/"start"/"subquests"/"until the part" → IN_PROGRESS |
| "kill/do/camp N <boss>", "N kc <boss>", "<boss> until N kc" | — | `boss_kc` (nenhum passo do guia dá contagem explícita; ficou só a tag `boss:<nome>`) |
| contexto de requisito: dentro de parênteses, após req/needs/when you/if you/after/at | "(15 Smithing required)", "At 58 slayer:" | vai para `prerequisites`, não para `complete_when` |
| aproximações | "(around 83 thieving)" | ignorado |

Quando o passo já tem `skillGoal` para a skill, o valor do campo prevalece e o texto não duplica.

## 6. Estatísticas

### 6.1 Regras por tipo (576 passos)

| Tipo | Passos com a regra | Observação |
|---|---|---|
| manual | 367 | sem regra inferível |
| quest_state | 170 | 144 de `questStatus` (high), 25 do texto (medium), 1 do campo `quest` sem status |
| skill_level | 59 | 47 de `skillGoal` (high), 3 texto high, 9 texto medium |
| diary_tier | 5 | 3 high (Ardy easy, Varrock easy, Fremennik hard), 2 medium (todas easy/medium) |
| boss_kc | 0 | nenhuma contagem explícita no guia |
| combat_achievement | 0 | não há menção |

Regras inferidas do texto: 43. Passos com mais de uma regra: 11 (um deles, 1.5.5, tem 10 quests).

### 6.2 Rastreabilidade

| Via | Passos | % |
|---|---|---|
| hiscores apenas (skill_level/boss_kc) | 50 | 8,7% |
| WikiSync/plugin (quest_state, diary_tier) | 159 | 27,6% |
| manual | 367 | 63,7% |

Por seção:

| Seção | hiscores | wikisync | manual | total |
|---|---|---|---|---|
| 1.1 Early game | 13 | 77 | 196 | 286 |
| 1.2 Thieving/fishing/mining | 5 | 7 | 21 | 33 |
| 1.3 Fairy rings/prayer/kingdom | 8 | 36 | 67 | 111 |
| 1.4 Skilling/graceful | 6 | 10 | 34 | 50 |
| 1.5 Diaries/RFD | 2 | 15 | 4 | 21 |
| 2.0 After Barrows gloves | 13 | 9 | 41 | 63 |
| S Sailing | 3 | 5 | 4 | 12 |

Leitura: a maior parte dos passos manuais do 1.1 são micro-passos de deslocamento, compra e coleta de itens ("buy a spade", "get a drink for barcrawl") que nenhuma fonte externa consegue ver sem um plugin próprio lendo inventário/varbits. O que é rastreável com WikiSync cobre praticamente todos os marcos (quests e diários); o hiscores cobre as metas de nível.

Camadas e tags: classic 538 / enhanced 38; tags `quest` 155, `skill` 55, `new-quest` 17, `sailing` 13, `diary-task` 11, `hcim` 7, `diary` 4, `phase:*` 22.

### 6.3 Top 30 passos manuais que parecem rastreáveis (para revisão humana)

Gerado por `normalize_guide.py --print` (80 candidatos no total, em `stats.json → manual_review_top30`).

| id | texto | regra sugerida |
|---|---|---|
| 1.1.82 | Do Wintertodt until 200k cash, get at least 22 fletching at wintertodt as well | `boss_kc Wintertodt` com kc alvo a definir; `skill_level fletching 22` ("at least" não foi reconhecido como verbo de meta: candidato a acrescentar); gp só via plugin |
| 1.1.167 | Get a drink for the barcrawl quest from brimhaven bar | sub-passo do Barcrawl (varbit por bar via plugin) |
| 2.0.40a | Moons of Peril: Farm Blue Moon boss first..., then Blood Moon | `boss_kc Lunar Chests` (kc alvo) |
| 2.0.40b | Farm Royal Titans for Twinflame Staff, Deadeye and Mystic Vigour... | `boss_kc The Royal Titans` (kc alvo) |
| 2.0.57 | Fortis Colosseum for Dizana's Quiver (BIS ranged cape) | `boss_kc Sol Heredit` ≥ 1 ou item (quiver) via collection log |
| 2.0.61a | Farm Doom of Mokhaiotl delves for Eye of Ayak and Avernic Treads | `boss_kc Doom of Mokhaiotl` (kc alvo) + prereq quest The Final Dawn |
| 1.1.140 | Get a security book from the Security guard... for a diary task | `diary_task` (Falador easy, tarefa individual; WikiSync expõe por área) |
| 1.2.16a | Now that you're 74 Fishing, do a few Tempoross kills for the Fish Barrel... | `boss_kc Tempoross` (ex.: ≥ 5) ou item Fish barrel |
| S.1 | Unlock Sailing via the Pandemonium quest | `quest_state Pandemonium FINISHED` (nome não reconhecido por causa do "via"; candidato a alias) |
| 1.1.104 | Get a drink for barcrawl at the bar south of Varrock fountain | sub-passo do Barcrawl |
| 1.1.111 | Get a drink for the barcrawl | sub-passo do Barcrawl |
| 1.1.128 | Get a drink from the Falador bar for Barcrawl | sub-passo do Barcrawl |
| 1.1.163 | Get a drink from Ardy bar for the barcrawl | sub-passo do Barcrawl |
| 1.1.205 | Get a drink for barcrawl from Yanille pub | sub-passo do Barcrawl |
| 1.1.210 | Get a drink from the port sarim bar for barcrawl | sub-passo do Barcrawl |
| 1.1.214 | Get a drink for barcrawl from the bar south of the Karamja banana trees | sub-passo do Barcrawl |
| 1.1.237 | Get a drink from Seers' bar for the barcrawl | último bar: `quest_state Alfred Grimhand's Barcrawl FINISHED` |
| 1.1.60 | Buy a drink from Blurberry for the Barcrawl miniquest | sub-passo do Barcrawl |
| 1.3.13 | Kill the nearby ram, bear and unicorn for their bones (Rag and bone man quest) | só plugin (kill count/itens); ou tratar como parte de `Rag and Bone Man I` |
| 1.4.41 | If you have 60 magic, get mage arena cape while you're here anyway | item (god cape) via plugin; prereq magic 60 já capturado |
| 2.0.41 | camp Corrupted Gauntlet until 2 enhanced weapon seeds... | `boss_kc The Corrupted Gauntlet` (kc alvo) ou collection log |
| 2.0.53 | Farm DT2 bosses: Vardorvis first for Ultor Ring + Soulreaper Axe | `boss_kc Vardorvis` (kc alvo) |
| 2.0.58 | Nex masses for Torva armour | `boss_kc Nex` (kc alvo) |
| 2.0.59 | Chambers of Xeric for Twisted Bow, Dragon Hunter Crossbow... | `boss_kc Chambers of Xeric` (kc alvo) |
| 2.0.60 | Theatre of Blood for Scythe of Vitur, Rapier, and Sanguinesti Staff | `boss_kc Theatre of Blood` (kc alvo) |
| S.9 | Build a teleport focus at Sailing 55 (needs 49 Construction) | varbit/unlock via plugin; prereqs capturados |
| 1.1.103 | Buy a rotten tomato from the crate near the achievement diary guy | item via plugin |
| 1.1.112 | Cut a dying tree at the sawmill and make 1 regular plank for diary tasks | `diary_task` (Varrock easy) |
| 1.1.130 | Buy 8 woad leaves, offer 20gp to get 2 leaves | item via plugin |
| 1.1.142 | Go down the dungeon and mine 2 blurite ore, 1 is for falador diary | `diary_task` (Falador easy) |

Outros candidatos fora do top 30 que valem olhar: 1.5.10 "Do all easy and medium diaries" já virou regra `all` (medium); 1.1.265 "Finish off Ardy easy tasks, and get the cloak" virou `diary_tier ardougne easy` (high); passos "Get X" de itens de quest (1.1.185 Prince Ali, 1.1.143 portrait) só com plugin.

### 6.4 Locais (`location`): 120 valores distintos

Mais frequentes: Various (59), Varrock (58), Lumbridge (40), Ardougne (24), Falador (17), Port Sarim (17), Draynor Village (15), Gnome Stronghold (14), Port Khazard (13), Al Kharid (10), Draynor (10), Fossil Island (10), Varlamore (9), Edgeville (9), Keldagrim (9), Lumbridge Castle (8), Rimmington (8), Seers Village (8), Castle Wars (7), Fred's Farm (7), Karamja (7), Lumbridge Swamp (6), Wizard Tower (6), Camelot (6), Catherby (6), Pollnivneach (6), Burthorpe (6), Miscellania (6), Yanille (5), Barbarian Village (5), Taverley (5), Shilo Village (5), Entrana (5), Port Phasmatys (5), Barbarian Assault (4), Baxtorian Falls (4), Draynor Manor (4), Dorgesh-Kaan (4), *(sem local)* (3), North of Ardougne (3), Great Kourend (3), Wintertodt (3), Ice Mountain (3), Mudskipper Point (3), Khazard Battlefield (3), Hosidius (3), Champions Guild (3), Uzer (3), Mage Bank (3), Ape Atoll (3), Rellekka (3), Pest Control (3), Prifddinas (3), Tombs of Amascut (3).

Com 2: West of Lumbridge, Clan Wars, Ferox Enclave, Varrock Museum, Shantay Pass, Brimhaven, Goblin Village, Rogues' Den, Morytania, Canifis, Zanaris, Crandor, Underground Pass, Mos Le'Harmless, Trollheim.

Com 1: Witchaven, Barbarian Outpost, South-west of Castle Wars, Varrock Sawmill, Lumber Yard, South of Mind Altar, Falador Park, Giants' Foundry, Desert, South of Khazard, Tower of Life, Fight Arena, ZMI Bank, Fishing Guild, HAM Hideout, Wilderness, Hazelmere's Island, Keep Le Faye, Observatory, Digsite, White Wolf Mountain, Barbarian Fishing, Tempoross Cove, Ardougne Monastery, Fisher King Realm, Lighthouse, Enchanted Valley, Nardah, Taverley Dungeon, Mort'ton, McGrubor's Wood, Canifis/Seers, Isafdar, Sophanem, Braindeath Island, Farming Guild, Desert Quarry, Warriors Guild, Weiss, Lunar Isle, Mage Arena, Royal Titans, God Wars Dungeon, Tormented Demons, Fortis Colosseum, Nex, Chambers of Xeric, Theatre of Blood, Yama, Doom of Mokhaiotl, Wyrmscraig.

Observações para uma futura tabela de locais: há pares a unificar (Draynor / Draynor Village; Lumbridge / Lumbridge Castle / Lumbridge Swamp; Seers Village / Camelot; Canifis/Seers); "Various" é um valor-curinga; os passos de endgame usam o nome do boss como local (Nex, Yama, Royal Titans...). O valor bruto foi mantido em `location`; a lista completa com contagens está em `stats.json → locations`.

### 6.5 Quests

- Catálogo canônico: 224 nomes (215 do `/quests`, sem o sufixo "(miniquest)", + aliases).
- Quests no campo `quest` dos passos: 115. Com as inferidas do texto: **129 nomes distintos** (lista completa em `stats.json → quests_in_guide`).
- Comparação com `data/guide/_quests-referenced.txt` (115 nomes, grafia do Oziris):
  - 7 renomeados para o nome do wiki: Vampire Slayer → **Vampyre Slayer**; Desert Treasure → **Desert Treasure I**; Desert Treasure II → **Desert Treasure II - The Fallen Empire**; Forgettable Tale... → **Forgettable Tale of a Drunken Dwarf**; Garden of Tranquility → **Garden of Tranquillity**; Rag and Bone Man → **Rag and Bone Man I**; Recipe for Disaster (Evil Dave) → **Recipe for Disaster/Freeing Evil Dave**.
  - 20 só aparecem no texto (sem campo `quest`), agora com regra inferida: A Soul's Bane, Another Slice of H.A.M., Between a Rock..., Big Chompy Bird Hunting, Contact!, Death Plateau, Eagles' Peak, Enakhra's Lament, Family Crest, Icthlarin's Little Helper, In Aid of the Myreque, In Search of the Myreque, Legends' Quest (start), Tai Bwo Wannai Trio, e os renomeados acima.
  - Nenhum nome ficou fora do catálogo (`quests_not_in_catalog: []`).
- 33 passos de *start*; 31 ligados a um passo de conclusão. Os 2 sem par são reais no guia: **Alfred Grimhand's Barcrawl** (o guia só manda pegar as bebidas; o último bar, 1.1.237, é a conclusão de fato) e **Fairytale II** (só se inicia, para os fairy rings).

## 7. Validação

```
$ python3 -I tools/validate_guide.py
guide: oziris-efficiency  sections=7  steps=576
quest start steps: 33  (unpaired: 2)   quests referenced: 115  catalog size: 224
errors: 0  warnings: 2
WARN  quest started but never completed in the guide: 1.1.57 (Alfred Grimhand's Barcrawl)
WARN  quest started but never completed in the guide: 1.3.31 (Fairytale II - Cure a Queen)
```

Checagens: ids únicos e com prefixo da seção; skills canônicas e níveis 1–99; estados de quest válidos e nomes no catálogo; áreas/tiers de diário e nomes de boss conhecidos; `requires`/`completes_in` apontando para ids existentes; `complete_when` nunca vazio e `manual` nunca misturado com outra regra; `layer`/`track_via` válidos.

## 8. Ressalvas

**Licença e atribuição.** O guia é texto de OzirisLoL (originalmente um Google Doc público), republicado, reorganizado e estendido pelo ironman.guide (passos `enhanced`, `updateNote`, páginas companheiras, cujos dados por sua vez citam o OSRS Wiki, CC BY-NC-SA). Não há licença explícita em nenhuma das camadas. Tratar tudo como **conteúdo para uso pessoal self-hosted**: manter `guide.source` (autor, mantenedor, URL) e o front matter dos `.md`, exibir a atribuição na UI, não redistribuir publicamente nem usar comercialmente. Se um dia o planner for publicado, o caminho é pedir autorização ou trocar o conteúdo por um guia do wiki (CC BY-NC-SA) ou próprio.

**Qualidade da inferência.** As 43 regras `inferred` foram revisadas uma a uma nesta rodada (falsos positivos conhecidos foram corrigidos: "Do Slayer until 60 attack", "when you reach 62 Agility", "Dragon slayer 2" → DS I, "RFD subquests" como FINISHED). Ainda assim são heurísticas: na UI, trate `confidence: medium` como sugestão que o usuário confirma, e permita marcar qualquer passo manualmente mesmo quando há regra (o clique do dono sempre vence; doc 09, decisão D10).

**Cobertura.** Nenhum passo gera `boss_kc` porque o guia nunca dá contagem alvo; os bosses de endgame ficaram como tags `boss:<nome>` esperando um kc definido por quem revisar. Também não há `combat_achievement`. A regra `diary_tier area=all` (1.5.10) precisa de um avaliador que itere todas as áreas.

**Nomes de boss do hiscores.** A tabela `BOSSES` em `normalize_guide.py` reflete a lista do hiscores até onde conheço (inclui Doom of Mokhaiotl, Yama, Royal Titans, Hueycoatl, Amoxliatl, Araxxor, Sol Heredit). Bosses de 2026 (Maggot King, Wyrmscraig) ainda não estão lá; confirmar contra o `index_lite.json` real quando o sync de hiscores existir.

**WikiSync.** Lembrete do doc 00: o wiki pede para não usar a API do WikiSync em projetos de terceiros. Os 159 passos "wikisync" são igualmente avaliáveis por um plugin RuneLite próprio lendo varbits de quests/diários.

**Páginas companheiras.** A conversão é fiel ao texto, mas cards de `div` viraram blocos "rótulo  \nvalor" (ex.: "Skill Reqs  \nNone"); para uso estruturado, prefira `quest-order.json` (já parseado) e, se necessário, um parser análogo para `/bosses` e `/gear` (tabelas GFM já são fáceis de ler). Os artigos têm "Updated <data>" no corpo; `/gear` e `/quests` declaram que os dados foram conferidos contra o wiki em setembro/outubro de 2026.

## 9. Próximos passos sugeridos

Superados em 2026-11-04. O top 30 e os bosses sem kc entram na extração por agente; locais canônicos
viram o catálogo `location`; o grafo usa `data/wiki/quests.json` e chaves de objetivo. Tudo no doc 09 e no
doc 04, seção 9.

## 10. Mapeamento para o schema canônico (Go)

`chronicle content migrate` (pacote `internal/core/content/migrate`) lê `data/guide/normalized/guide.json` (schema v0.1 acima) e escreve `content/guides/oziris-main/guide.json` no schema canônico `content/schema/guide.schema.json`, validando a saída antes de gravar. A saída é determinística (mesmo input ⇒ mesmos bytes) e deve ser regenerada, não editada à mão. Mapeamento campo a campo:

| v0.1 (normalizador) | Canônico | Regra |
|---|---|---|
| `guide.id` = `oziris-efficiency` | `id`/`slug` = `oziris-main`; `source.original_id` guarda o id antigo | id do diretório é o canônico |
| `guide.source {url, author, maintainer, extracted_at, license_note}` | `source {origin: ironman.guide, url, author, license: license_note, imported_at: extracted_at}`; `maintainer` vai para `description` | `source` não tem campo `maintainer`; não valeu a pena estender |
| — | `schema_version: 1`, `version: "2026.11.0"`, `default_modes` (4), `default_layers` (`classic`,`enhanced`) | fixos no migrador (`--content-version`) |
| `section.track: true/false` | `section.kind: track/main` | explícito em todas as seções |
| `section.seo_title` | descartado | — |
| `section.related` | igual | hrefs relativos mantidos |
| `step.note` | `notes[{kind: hcim \| tip}]` | `hcim` quando a tag `hcim` está presente, senão `tip` |
| `step.update_note` | `notes[{kind: update}]` | — |
| `step.items[]` (strings) | `items[]` (forma string) | — |
| `step.links[]` | `links[]` | URL relativa (`/articles/...`) vira absoluta em `https://ironman.guide` |
| `step.quest {name, status, wiki}` | `links[{type: wiki, url, text: name}]` | o estado em si já está em `complete_when`; `status` não é repetido |
| `step.layer: classic` | `layers: [classic, enhanced]` | passo original aparece nos dois modos |
| `step.layer: enhanced` | `layers: [enhanced]` | — |
| `step.modes` | `modes` | sempre os quatro |
| `step.phase` | `phase` | — |
| `step.tags` | `tags` | normalizadas para `^[a-z0-9][a-z0-9:_-]*$`: `boss:The Corrupted Gauntlet` → `boss:the-corrupted-gauntlet`; `new-quest` → `new-content` (vocabulário do doc 04) |
| `complete_when: [{type: manual}]` | `complete_when` **ausente** | o schema define "ausente/vazio ⇒ manual"; 367 passos |
| regra `skill_level {skill, level}` | igual | — |
| regra `quest_state {state: FINISHED}` | igual, sem `op` (default `gte`) | — |
| regra `quest_state {state: IN_PROGRESS, or_finished: true}` | `{state: IN_PROGRESS, op: gte}` | — |
| regra `diary_tier {area, tier}` | igual; `area: all` mantido | o engine expande `all` para as 12 áreas |
| regra `boss_kc {boss, kc}` | igual | nenhuma ocorrência hoje |
| regra `.source/.inferred/.confidence/.match` | `meta {origin, inferred, confidence, match}` | **extensão do schema** (abaixo) |
| `step.prerequisites[]` | `prerequisites[]` (lista de regras com `meta`) | **extensão do schema** (abaixo); informativo, não entra em `complete_when` |
| `step.requires: ["1.1.5"]` | `requires: ["step:1.1.5"]` | formato `ref` do schema; o loader verifica que o id existe |
| `step.completes_in` | descartado | redundante: é o inverso de `requires` do passo de conclusão |
| `step.track_via` | descartado | derivável dos tipos de regra |
| todos os passos | `source {origin: ironman.guide, original_id: id, imported_at: 2026-11-03}` | — |

### Extensões feitas no schema canônico

Ambas aditivas e opcionais; `additionalProperties: false` continua em todos os objetos.

1. **`$defs/ruleMeta`** e a propriedade **`meta`** em todas as 13 regras atômicas (`skill_level` … `manual`): `{origin: string, inferred: boolean, confidence: high|medium|low, match: string}`. Guarda a proveniência da regra (campo estruturado do site vs. inferida do texto) sem influenciar a avaliação. A UI deve tratar `confidence: medium` como sugestão a confirmar (ressalva da seção 8).
2. **`step.prerequisites`**: array de regras (mesmo `$defs/rule`), informativo: o que o texto diz que você precisa antes do passo. Não bloqueia conclusão; o grafo pode derivar arestas suaves daí. 20 passos.

Resultado da migração: 7 seções, 576 passos, 209 com regra automática (170 `quest_state`, 59 `skill_level`, 5 `diary_tier`, com sobreposição por passo), 367 manuais; `chronicle content validate` passa com 0 erros.

## 11. Adendo de 2026-11-04: segundo guia, segmentos e entidades

- **BRUHsailer** (So Iron BRUH e ParasailerOSRS) entrou como rota independente em
  `content/guides/bruhsailer-main/`: 226 passos em 9 seções, importados dos três DOCX do mirror
  `Osrsper/BRUHsailer` no commit `b697c0a` (checksums em `data/sources/bruhsailer/`) por
  `chronicle content import-bruhsailer`. Os passos são parágrafos longos (média de 1097 caracteres) e
  **não têm regras `complete_when`**; os metadados da fonte (GP, tempo, itens, skills/quests) ficam como
  notas. Proveniência e mapeamento em `content/guides/bruhsailer-main/README.md`.
- **Cores do autor** (2026-11-08): o DOCX usa cor de texto como ênfase ("cheapest in red" no CH1.30,
  laranja para o caminho da Phoenix e vermelho para o da Black Arm no CH1.13, avisos de HCIM). O
  importador grava cada trecho colorido em `step.marks[]` (`span` em bytes de `text`, `color`
  `red`/`orange`/`green` pela família do matiz); preto, cinza, o azul dos links e o roxo das linhas de
  metadados não viram mark. O loader confere que cada span cai dentro do texto, em ordem e sem cortar
  caractere. Os links da fonte (`links[]`, 158 no BRUHsailer) aparecem na Rota: no texto, quando o
  autor cita o nome do arquivo ("route in red in Sunset Ocean.png"), ou numa linha "Links".
- **Segmentos**: os dois guias são divididos em `bank_leg` (começa numa ação de banco explícita) e
  `sequence`, com `bank_actions` e `loadout` `known`/`partial`/`unknown`, sempre com proveniência.
  Oziris: 32 segmentos (25 bank legs, 5 loadouts parciais). BRUHsailer: 102 (99 bank legs, 84 parciais).
- **Entidades**: `internal/core/entities` resolve menções a quest, item, skill, diary e CA por match
  exato contra os catálogos em `content/reference/` e grava `Step.entities` com citação e posição. O que
  é ambíguo ou não resolve vai para `content/guides/<id>/entities.report.json`. No Oziris:
  672 resolvidas, 20 ambíguas, 109 sem resolução. No BRUHsailer: 1846, 176 e 643. O catálogo de
  itens é gerado da wiki (cerca de 850 ids: os itens citados pelos dois guias mais as famílias que eles
  tocam; doc 09, seção 7).
- **Limites conhecidos**:
  - itens que a fonte nomeia de outro jeito que o jogo ("Juliet's letter", "Rune scim") continuam sem
    resolução; a extração por agente (doc 09, seção 6) os resolve com confirmação do dono;
  - quantidades como "1k" não são interpretadas;
  - requisitos genéricos ("Axe", "food") ficam ambíguos;
  - algumas ações de banco são mal classificadas ("bank everything" vira `open_bank`).

  A revisão do modelo e a extração estruturada dos textos estão no doc 09.

## 12. Correções de rota do dono (2026-11-05)

O objetivo é uma rota boa, não uma cópia literal do Oziris. Quando uma nota de update do site diz que
o jogo mudou e o passo ficou impossível ou fora de ordem, a correção entra no import, nunca à mão no
`guide.json` (que continua gerado):

- `data/guide/route-patches.json` lista os patches, cada um com `id`, `reason` e operações em ordem:
  `drop` (remove o passo; recusa se outro passo o tem em `requires`), `move` (põe o passo logo depois de
  outro, na mesma seção), `insert` (passo novo, id novo no estilo `1.1.254a`, `source.origin: own`),
  `note` (troca a nota de update que pediu o patch por uma que diz o que foi ajustado) e `items` (troca
  os itens do passo).
- `chronicle content migrate` aplica os patches depois de converter os passos e antes de derivar os
  segmentos (`internal/core/content/migrate/patches.go`). Um patch que não casa mais com o guia
  importado é erro, nunca é pulado.
- Passos movidos mantêm o id: marcas, cursor e anotações continuam valendo, e `content_steps.position`
  segue a nova ordem. Um passo removido leva a anotação junto: o `submit` do `content mcp` reescreve o
  sidecar da seção sem ele (e o relatório sem o log dele). Passos com texto, nota ou itens alterados
  ficam `stale` e são reprocessados pela skill `annotate-guide`.

Patches atuais (578 passos: 576 importados, 1 removido, 3 adicionados):

| Patch | O que faz |
|---|---|
| `lost-tribe-after-goblin-diplomacy` | Goblin Diplomacy virou requisito da The Lost Tribe (nota de 1.1.254). A quest inteira vai para depois de 1.1.278: 280 (home tele), 254, 254a (tele Varrock), 256, 256a (Falador tele até a Goblin Village, com o brooch e o livro), 279, 279a (tele Lumbridge), 281. 1.1.277 leva só as dyes. Substitui a decisão `update-note-overrides-text` nesse trecho |
| `no-hosidius-favour` | Remove 1.4.2 (100% de favour em Hosidius): o sistema de favour de Kourend saiu em janeiro de 2024. Com ele sai o bank leg que o passo abria (31 segmentos, 24 bank legs) |

As outras 12 notas de update só sugerem alternativas e continuam como notas.
