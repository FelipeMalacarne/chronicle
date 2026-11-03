# Fontes de dados — levantamento detalhado

Data: 2026-11-03. Complementa a seção 2 de `00-levantamento-inicial.md`. Tudo abaixo foi verificado com chamadas reais nesta sessão (User-Agent `ironman-planner-dev (research)`); as respostas brutas estão em `data/samples/<fonte>/` e os artefatos derivados em `data/wiki/`.

Conta usada como cobaia: **Iron Ohzone** (ironman maxed, aparece no WOM, no WikiSync e no TempleOSRS) e **ironman260** (ironman f2p de nível baixo, útil para ver skills sem rank e diaries vazias).

---

## 1. Hiscores oficial (Jagex)

### Endpoints

| Modo | URL base |
|---|---|
| Normal | `https://secure.runescape.com/m=hiscore_oldschool/` |
| Ironman | `https://secure.runescape.com/m=hiscore_oldschool_ironman/` |
| Hardcore | `https://secure.runescape.com/m=hiscore_oldschool_hardcore_ironman/` |
| Ultimate | `https://secure.runescape.com/m=hiscore_oldschool_ultimate/` |

Sufixos: `index_lite.json?player=NOME` (JSON) e `index_lite.ws?player=NOME` (CSV). Espaço no nome vai como `%20` (ou `_`).

### Forma do JSON (`data/samples/hiscores/iron_ohzone.json`)

```json
{
  "name": "iron ohzone",
  "skills": [
    {"id": 0,  "name": "Overall", "rank": 43,  "level": 2376, "xp": 2657416377},
    {"id": 1,  "name": "Attack",  "rank": 339, "level": 99,   "xp": 122462831},
    {"id": 24, "name": "Sailing", "rank": 193, "level": 99,   "xp": 61242946}
  ],
  "activities": [
    {"id": 0,  "name": "Grid Points",        "rank": -1,    "score": 0},
    {"id": 7,  "name": "Clue Scrolls (all)", "rank": 73,    "score": 12678},
    {"id": 25, "name": "Barrows Chests",     "rank": 16809, "score": 928}
  ]
}
```

- **25 skills**, nesta ordem (o `id` é o índice): Overall, Attack, Defence, Strength, Hitpoints, Ranged, Prayer, Magic, Cooking, Woodcutting, Fletching, Fishing, Firemaking, Crafting, Smithing, Mining, Herblore, Agility, Thieving, Slayer, Farming, Runecraft, Hunter, Construction, **Sailing** (id 24, já incluído).
- **92 activities** (id 0..91): Grid Points, League Points, Deadman Points, Bounty Hunter (Hunter/Rogue + Legacy), Clue Scrolls (all/beginner/easy/medium/hard/elite/master), LMS - Rank, PvP Arena - Rank, Soul Wars Zeal, Rifts closed, Colosseum Glory, **Collections Logged** (id 19), e depois os bosses em ordem alfabética: Abyssal Sire, Alchemical Hydra, Amoxliatl, Araxxor, Artio, Barrows Chests, Brutus, Bryophyta, Callisto, Calvar'ion, Cerberus, Chambers of Xeric, CoX: Challenge Mode, Chaos Elemental, Chaos Fanatic, Commander Zilyana, Corporeal Beast, Crazy Archaeologist, Dagannoth Prime/Rex/Supreme, Deranged Archaeologist, Doom of Mokhaiotl, Duke Sucellus, General Graardor, Giant Mole, Grotesque Guardians, Hespori, Kalphite Queen, King Black Dragon, Kraken, Kree'Arra, K'ril Tsutsaroth, Lunar Chests, Mad Angel, Maggot King, Mimic, Nex, Nightmare, Phosani's Nightmare, Obor, Phantom Muspah, Sarachnis, Scorpia, Scurrius, Shellbane Gryphon, Skotizo, Sol Heredit, Spindel, Tempoross, The Gauntlet, The Corrupted Gauntlet, The Hueycoatl, The Leviathan, The Royal Titans, The Whisperer, Theatre of Blood, ToB: Hard Mode, The Fractured Archive, Thermonuclear Smoke Devil, Tombs of Amascut, ToA: Expert Mode, TzKal-Zuk, TzTok-Jad, Vardorvis, Venenatis, Vet'ion, Vorkath, Wintertodt, Yama, Zalcano, Zulrah. A lista completa está no sample; **não assuma a ordem fixa**: a Jagex insere bosses novos no meio (ordem alfabética), então mapeie sempre por `name`.
- **Skill sem rank** (`ironman260.json`): `{"name": "Fletching", "rank": -1, "level": 1, "xp": 0}`. Ou seja, `rank: -1` e o `level`/`xp` vêm como 1/0 mesmo que o jogador tenha algum XP abaixo do corte de ranking (o hiscore só lista a partir de certo XP). Activities sem rank: `rank: -1, score: 0` (em CSV aparece `-1,-1` ou `-1,0`).
- A activity `Collections Logged` (score 2500 para Iron Ohzone) é um atalho barato para "quantos slots de collection log", sem plugin.

### CSV (`index_lite.ws`)

117 linhas (`25 skills + 92 activities`), sem cabeçalho. Skills: `rank,level,xp`; activities: `rank,score`. A ordem é a mesma do JSON. Prefira o JSON (tem `name`, então sobrevive a inserções de bosses).

### Comportamentos observados

- Jogador inexistente ou que não é do modo pedido: **HTTP 404** com HTML (`<title>404 - Page not found</title>` + meta refresh para `runescape.com/error404`). Testado: `Lynx Titan` no endpoint ironman → 404; `ironman260` (iron normal) no endpoint hardcore → 404. Isso serve de **detector de modo**: se responde 200 em `_hardcore_ironman` é HCIM; se só em `_ironman`, iron normal; etc. (um HCIM que morreu continua aparecendo no `_ironman`).
- Headers: `cache-control: no-cache`, Cloudflare (`cf-cache-status: DYNAMIC`, cookie `__cf_bm`). Não há header de rate limit.
- Rate limit: 10 requisições seguidas sem pausa responderam 200 em ~1 s cada; o bloqueio da Jagex não é documentado e aparece como 429/HTML da Cloudflare em volumes maiores. Para um planner single-user, 1 chamada a cada 10–30 min por conta é mais que seguro. Faça 1 chamada por conta, não uma por modo.

---

## 2. Wise Old Man (WOM)

Docs: https://docs.wiseoldman.net/ (SPA Docusaurus; páginas: `/api`, `/api/players/player-endpoints`, `/api/players/player-type-definitions`; texto extraído em `data/samples/wiseoldman/docs_*.txt`). Base: `https://api.wiseoldman.net/v2`.

### `GET /players/{username}` (`player_iron_ohzone.json`)

```json
{
  "id": 44304, "username": "iron ohzone", "displayName": "Iron Ohzone",
  "type": "ironman", "build": "main", "status": "active",
  "exp": 2657416377, "ehp": 9046.9, "ehb": 4750.9, "ttm": 0, "tt200m": 11498.7,
  "registeredAt": "2020-09-23T00:18:56.929Z", "updatedAt": "2026-11-03T22:34:23.217Z",
  "lastChangedAt": "...", "lastImportedAt": "...", "combatLevel": 126,
  "latestSnapshot": {
    "id": ..., "playerId": 44304, "createdAt": "...", "importedAt": null,
    "data": {
      "skills":   {"sailing": {"metric": "sailing", "experience": 61242946, "rank": 193, "level": 99, "ehp": 269.9}, "...": {}},
      "bosses":   {"abyssal_sire": {"metric": "abyssal_sire", "kills": 13028, "rank": 1, "ehb": 296.1}, "...": {}},
      "activities": {"clue_scrolls_all": {"metric": "clue_scrolls_all", "score": 12678, "rank": 73}, "...": {}},
      "computed": {"ehp": {"metric": "ehp", "value": 9046.9, "rank": 62}, "ehb": {...}}
    }
  }
}
```

- **`type`** é o campo de detecção de modo: `'unknown' | 'regular' | 'ironman' | 'hardcore' | 'ultimate'` (type-definitions). `build`: `main`, `f2p`, `lvl3`, `zerk`, `def1`, `hp10`, `f2p_lvl3`. Um iron que de-ironou fica `regular` depois do próximo update (ou via `POST /players/{username}/assert-type`).
- Métricas em snake_case (`runecrafting`, não `runecraft`; `sailing` já existe). Skill sem rank: `rank: -1`, `experience: -1` nos snapshots antigos ou 0.
- Erro: `GET /players/zzzzzzqqqqqq1` → **404** `{"code":"PLAYER_NOT_FOUND","message":"Player not found"}`.

### Histórico e ganhos

- `GET /players/{username}/snapshots?period=week|month|year|5min|day` ou `startDate`+`endDate`, `limit`/`offset`. Retorna array de `Snapshot` (mesma forma de `latestSnapshot`). Sample: 20 snapshots na semana.
- `GET /players/{username}/snapshots/timeline?metric=slayer&period=month` → `[{"value": 112529313, "rank": 82, "date": "..."}]` (leve, ideal para gráfico).
- `GET /players/{username}/gained?period=week` → `{"startsAt","endsAt","data":{"skills":{"slayer":{"metric":"slayer","experience":{"gained":0,"start":...,"end":...},"ehp":{...},"rank":{...},"level":{...}}},"bosses":{...},"activities":{...},"computed":{...}}}`.
- `GET /players/{username}/achievements` → marcos (`"name":"50m Agility","metric":"agility","threshold":50000000,"createdAt":...`).
- `GET /players/search?username=iron&limit=20` → array de players (sem snapshot).

### Update

`POST /players/{username}` (sem body) força o WOM a ler o hiscore e devolve o player atualizado (200). Repetir imediatamente devolveu 200 de novo (eles têm cooldown interno curto, não retornou 429 no teste). Nome inválido → 400 `{"code":"INVALID_USERNAME","subError":{"code":"USERNAME_TOO_LONG"}}`.

### Rate limits (docs `/api`, confirmados pelos headers)

| Janela | Sem API key | Com API key |
|---|---|---|
| por minuto | 20 | 100 |
| por hora | 500 | sem limite |
| por dia | 3.000 | 20.000 |

Headers em toda resposta: `ratelimit-limit: 20`, `ratelimit-remaining: 19`, `ratelimit-reset: 60`. Texto dos docs: *"If you're planning on using the API to auto-update players on any given short interval, please think twice... the best approach is to use a longer interval, such as 1-6 hours... If you are found to be updating players too often, this can result in an IP ban."* Peça API key no Discord deles (`#support`) e **sempre mande User-Agent identificável** (sem UA eles banem IP).

### Quests/diaries no WOM

**Não existem.** Nenhuma menção a quest/diary nos endpoints de player nem nos type-definitions; o snapshot só tem `skills`, `bosses`, `activities`, `computed`.

---

## 3. WikiSync (sync.runescape.wiki)

### Política do wiki (verbatim, https://oldschool.runescape.wiki/w/RuneScape:WikiSync, seção "Third-party use")

> The WikiSync plugin and the associated API is intended for use by the wiki, and not by third-party developers. While we love seeing what cool projects people can make with data from the wiki, people who enable the plugin are choosing to share their game data to improve their experience on the wiki, rather than arbitrary third-parties. **Please do not use the WikiSync API in your own projects**. We will be actively taking steps to limit the usage of our API outside of our own websites.

Conclusão prática: usar no máximo como **importador manual/opcional** ("importar meu estado do WikiSync agora"), nunca como polling. O caminho correto de longo prazo é o plugin próprio (seção 6).

### `GET https://sync.runescape.wiki/runelite/player/{nome}/STANDARD`

Quem tem dados: Iron Ohzone, iron neuron, ironman260, Settled, B0aty (samples em `data/samples/wikisync/player_*.json`). Sem dados: **400** `{"code":"NO_USER_DATA","error":"No user data found."}`. Outros profiles do path: `STANDARD` é o mundo normal (há variantes para leagues/beta no código do wiki).

Chaves do JSON: `username, timestamp, quests, achievement_diaries, levels, music_tracks, combat_achievements, league_tasks, bingo_tasks, collection_log, collectionLogItemCount, sea_charting`.

```json
{
  "username": "iron ohzone",
  "timestamp": "2026-11-03T22:44:33.396Z",
  "quests": {"Animal Magnetism": 2, "The Fractured Archive": 1, ".": 0, "...": 2},
  "levels": {"Attack": 99, "Runecraft": 99, "Sailing": 99, "...": 99},
  "achievement_diaries": {
    "Ardougne": {
      "Easy":   {"complete": true, "tasks": [true, true, true, true, true, true, true, true, true, true]},
      "Medium": {"complete": true, "tasks": [12 booleans]},
      "Hard":   {"complete": true, "tasks": [12 booleans]},
      "Elite":  {"complete": true, "tasks": [8 booleans]}
    },
    "Desert": {}, "Falador": {}, "Fremennik": {}, "Kandarin": {}, "Karamja": {},
    "Kourend & Kebos": {}, "Lumbridge & Draynor": {}, "Morytania": {}, "Varrock": {},
    "Western Provinces": {}, "Wilderness": {}
  },
  "music_tracks": {"7th Realm": true, "Adventure": true, "...": false},
  "combat_achievements": [0, 1, 2, 3, "...ids numéricos das CAs concluídas"],
  "league_tasks": null,
  "bingo_tasks": [],
  "collection_log": [1249, 2366, 2577, "...item IDs obtidos"],
  "collectionLogItemCount": 1613,
  "sea_charting": [0, 1, 2, "...ids"]
}
```

- **`quests`**: 215 entradas (quests + miniquests + as 10 subquests de RFD). Valores: **0 = não iniciada, 1 = em progresso, 2 = concluída** (ironman260: 193×0, 22×2; Iron Ohzone: 213×2 e `The Fractured Archive` = 1). Há uma chave lixo `"."`. Nomes seguem o RuneLite/wiki: `Desert Treasure I`, `Desert Treasure II - The Fallen Empire`, `Vampyre Slayer`, `Garden of Tranquillity`, `Recipe for Disaster - Evil Dave`, `Rag and Bone Man I/II`, `The Hand in the Sand`.
- **`levels`**: nível real (não boostado; o plugin usa `client.getRealSkillLevel`), 24 skills, chave `Runecraft`.
- **`achievement_diaries`**: 12 áreas × 4 tiers; `tasks` é array de booleanos **na ordem da tabela do jogo/wiki**. Cruzei a contagem com as tabelas de tarefas do wiki e bateu para todas as 48 combinações área/tier (ex.: Ardougne 10/12/12/8, Karamja 10/19/10/5, Varrock 14/13/10/5). Portanto `tasks[i]` corresponde à tarefa `i+1` da página `X Diary` do wiki (`data/wiki/diary_tasks.json`).
- **`combat_achievements`**: lista de IDs numéricos; o ID é o mesmo campo `id` do bucket `combat_achievement` do wiki (0 = "Noxious Foe", 1 = "Abyssal Adept"...). Iron Ohzone tem 595 de 655.
- **`collection_log`**: lista de **item IDs** obtidos (resolvida a partir do bitset que o plugin envia); `collectionLogItemCount` é o total de slots únicos. `music_tracks`, `sea_charting` (Sailing) e `league_tasks` existem mas não interessam ao planner por ora.

### Manifest `GET https://sync.runescape.wiki/runelite/manifest` (`manifest.json`)

```json
{"varbits": [18274, 12296, "... 294 ids"], "varps": [1196, 1197, "... 164 ids"],
 "version": 1, "collections": [1249, 2366, "... 1738 item ids"], "timestamp": "2026-11-03T22:27:51.068Z"}
```

O plugin (`andmcadams/WikiSync`, `WikiSyncPlugin.java`, sample em `data/samples/runelite/WikiSync_WikiSyncPlugin.java`) baixa esse manifest, lê `client.getVarbitValue(id)` para cada varbit, `client.getVarpValue(id)` para cada varp, `getRealSkillLevel` para cada skill, e envia **deltas** em `POST https://sync.runescape.wiki/runelite/submit` junto com o collection log como bitset base64 cuja posição `i` é o item `collections[i]`. Ou seja: **é o servidor do wiki que traduz varbit → estado de quest/diary**; a lista de varbits do manifest é um ótimo ponto de partida para o plugin próprio (inclui p.ex. 6104 = `DRAGON_SLAYER_II_PROGRESS`, mas **não** inclui os varbits de "diary completed" 4458+; o wiki deriva a diary dos varbits de cada tarefa).

Cache: `cf-cache-status: HIT` no manifest; o endpoint de player é dinâmico.

---

## 4. OSRS Wiki Bucket API

Doc: https://oldschool.runescape.wiki/w/RuneScape:Bucket e https://meta.weirdgloop.org/w/Extension:Bucket (+ `/Api`). Chamada: `GET https://oldschool.runescape.wiki/api.php?action=bucket&format=json&query=<lua>`. Sintaxe: `bucket('nome').select('a','b').where('campo','valor').limit(500).offset(0).run()`. Operadores aceitos em `where`: igualdade (e `'!='`, `'>'`, `'<'`, `'>='`, `'<='`); **`LIKE` não existe** (`"error":"Invalid operand for bucket.where(): LIKE."`). Colunas reservadas em todo bucket: `page_name`, `page_name_sub`, `_page_id`, `_index`. Limite máximo observado: 500 por chamada (`limit(500)` + `offset`). Sem rate limit explícito; use UA e pausas de 1 s.

Lista de buckets (namespace 9592): `allpages&apnamespace=9592` → Bountytaskline, Clue equipment, Clue info, Collection log source, **Combat achievement**, Couriertaskline, Dependency list, Drop table sources, Dropsline, Exchange, Infobox activity/bonuses/construction/item/location/monster/npc/pure/scenery/ship part/shop/spell, Interface, Item id, Locline, Logs, Map, Mine, Money making guide, Music*, Npc id, Object id, **Quest**, Recipe, Recommended equipment, Seachart, Sound effect, Soundtrack, Storeline, Transcript, Update, **Varbit**. **Não existe bucket de achievement diary.**

Schemas (raw em `data/samples/wiki-bucket/schema_Bucket_*.txt`, obtidos com `/w/Bucket:<Nome>?action=raw`):

| Bucket | Campos |
|---|---|
| `quest` | `description, enemies_to_defeat, ironman_concerns, items_required, official_difficulty, official_length, requirements, start_point, json` (todos TEXT; `json` = a chamada inteira de `{{Quest details}}` serializada: `requirements, desc, difficulty, kills, name, start, length, items`). **Não tem** quest points, série, release, members. |
| `varbit` | `content (PAGE), name (TEXT), index (INTEGER)`; `page_name` = `RuneScape:Varbit/6104` ou `RuneScape:Varplayer/101`. |
| `combat_achievement` | `id, name, monster (PAGE), task, tier, type, league_region` — 655 linhas (Easy 41, Medium 64, Hard 89, Elite 166, Master 173, Grandmaster 122). `id` = id do WikiSync. Dump: `ca_all.json`. |
| `infobox_item` | `item_name, image[], is_members_only, item_id[], examine, high_alchemy_value, league_region, release_date, removal_date, value, weight, version_anchor, buy_limit, default_version, quest, tradeable`. |
| `infobox_monster` | `name, id[], combat_level, hitpoints, max_hit[], slayer_level, slayer_experience, slayer_category[], attack/strength/defence/ranged/magic_level, bônus ofensivos/defensivos, attribute[], is_members_only, release_date, ...` (schema completo no sample). |

### Requisitos de quest: onde estão e como extrair

Estão no campo `requirements` do bucket `quest` (igual ao `json.requirements`), já **renderizado como HTML + wikitext**. Cada requisito de skill vem como:

```
*<span class="scp" data-skill="Magic" data-level="75" ...>[[File:Magic icon.png|...]] 75 [[Magic]]</span> <sup ...>[not boostable]</sup> <sup ...>[required to start]</sup>
```

e os pré-requisitos de quest como árvore de bullets depois de `*Completion of the following quests:`, com `**[[Quest]]` = pré-requisito direto e `***`/`****` = transitivos (o wiki expande a árvore inteira). Linhas de `[[Quest points]]` vêm com `data-skill="Quest points"`. Requisitos parciais aparecem como texto livre ("Started the pyre ship portion of the Barbarian Training miniquest"). No wikitext original a mesma coisa é `{{SCP|Magic|75|link=yes}} {{Boostable|no}} {{Questreqstart|yes}}`.

Query usada (uma chamada devolve as 236 linhas = 225 páginas únicas; `Recipe for Disaster/Full guide` duplica a linha de RFD):

```
bucket('quest').select('page_name','official_difficulty','official_length','requirements','items_required','ironman_concerns','start_point','enemies_to_defeat','description').limit(500).run()
```

Resposta bruta: `data/samples/wiki-bucket/quest_all_p1.json` (590 KB). `offset(500)` devolve `[]`, então não há paginação necessária.

**Quest points, número, série, release e members não estão no Bucket.** A lista `Quests/Members` do wiki é gerada por DPL a partir de `{Infobox Quest}:number:series:release` e `{Quest rewards}:qp` das páginas, então busquei o wikitext das 225 páginas com a API MediaWiki normal, 50 títulos por chamada (5 chamadas):

```
api.php?action=query&prop=revisions&rvprop=content&rvslots=main&format=json&formatversion=2&titles=A|B|...
```

(`data/samples/wiki-wikitext/quest_pages/batch0..4.json`, 3,7 MB). De lá saem `{{Infobox Quest|number=136|release=[[4 January]] [[2018]]|members=Yes|series=[[Quests/Series#Dragonkin|Dragonkin]], #3}}`, `{{Quest rewards|qp=5|...}}` e `{{Infobox Miniquest}}` (marca miniquest). Esse mesmo wikitext é o fallback se o Bucket sumir: `{{Quest details|requirements=...}}` tem exatamente os mesmos dados em forma de template.

### Resultado: `data/wiki/quests.json`

Gerado por `data/wiki/scripts/build_quests.py` (Bucket + wikitext). 225 entradas (205 quests com `number` + 20 miniquests), 138 com `skill_reqs`, 132 com `quest_reqs`, 198 com `quest_points` inteiro. Formato:

```json
{
  "name": "Dragon Slayer II", "number": "136", "difficulty": "Grandmaster", "length": "Very Long",
  "quest_points": 5, "quest_points_required": 200, "series": "Dragonkin, #3",
  "members": true, "release_date": "4 January 2018", "is_miniquest": false,
  "aliases": {"wikisync": "Dragon Slayer II", "runelite": "Dragon Slayer II"},
  "skill_reqs": [{"skill": "Magic", "level": 75, "boostable": false, "required_to_start": true}, "..."],
  "quest_reqs": ["Legends' Quest", "Dream Mentor", "A Tail of Two Cats", "Animal Magnetism", "Ghosts Ahoy", "Bone Voyage", "Client of Kourend"],
  "quest_reqs_all": ["...fechamento transitivo que o wiki lista"],
  "other_reqs": ["Started the pyre ship portion of the Barbarian Training miniquest to gain access to the Ancient Cavern"],
  "start_point": "Talk to Alec Kincade outside the Myths' Guild.",
  "items_required_raw": "...", "enemies_raw": "...", "ironman_concerns_raw": "...", "requirements_raw": "...",
  "wiki_url": "https://oldschool.runescape.wiki/w/Dragon_Slayer_II"
}
```

Observações do parser: `boostable: null` quando o wiki não marca; `skill: "Combat level"` para requisito de combat; `aliases` traduz o nome da página do wiki para o nome que WikiSync/RuneLite usam (só difere nas 10 subquests de RFD, `Recipe for Disaster/Freeing Evil Dave` → `Recipe for Disaster - Evil Dave`, e em `The Fractured Archive (miniquest)`/`Vale Totems (miniquest)`). Quests sem nenhum requisito ficam com as três listas vazias (ex.: Rune Mysteries, Pandemonium).

### Cross-check com o guia (`data/guide/_quests-referenced.txt`, 115 nomes)

108 batem exatamente com `page_name`. Os 7 que **não** batem e o nome correto no wiki/WikiSync/RuneLite:

| Nome no guia | Página do wiki (= WikiSync = RuneLite) |
|---|---|
| Desert Treasure | **Desert Treasure I** |
| Desert Treasure II | **Desert Treasure II - The Fallen Empire** |
| Garden of Tranquility | **Garden of Tranquillity** (dois "l") |
| Hand in the Sand | **The Hand in the Sand** |
| Rag and Bone Man | **Rag and Bone Man I** (o guia provavelmente quer o I; o II é o da lista de ossos) |
| Recipe for Disaster (Evil Dave) | wiki: **Recipe for Disaster/Freeing Evil Dave**; WikiSync/RuneLite: **Recipe for Disaster - Evil Dave** |
| Vampire Slayer | **Vampyre Slayer** |

Recomendação: manter no conteúdo do guia um mapa `alias → nome canônico do wiki` e validar no boot (schema) que toda `quest:` do YAML existe em `quests.json`.

---

## 5. Achievement diaries (tarefas e requisitos)

Não há bucket. A fonte estruturada é o **wikitext das 12 páginas `<Área> Diary`** (Ardougne, Desert, Falador, Fremennik, Kandarin, Karamja, Kourend & Kebos, Lumbridge & Draynor, Morytania, Varrock, Western Provinces, Wilderness), via `api.php?action=parse&page=Ardougne_Diary&prop=wikitext&format=json&formatversion=2` (samples em `data/samples/wiki-wikitext/diaries/*.json`).

Estrutura por tier:

```
{| class="wikitable lighttable qc-active diary-table" style=width:750px; data-diary-name="Ardougne" data-diary-tier="Easy"
! Task !! Requirements
|-
|1. Have [[Wizard Cromperty]] teleport you to the [[Rune Essence mine]].
|
*{{SCP|Quest}} Completion of [[Rune Mysteries]]
|-
|2. Steal a [[cake]] from the [[Ardougne Baker's Stall.|Ardougne market stalls]].
|
*{{SCP|Thieving|5|link=y}}
```

Além disso cada tier tem um bloco `{{DiarySkillStats|boostable=true|Thieving = 5|Total = 36}}` (resumo de níveis do tier) e uma tabelinha "Quest requirements" (`|- data-rowid="Biohazard"`). Em Karamja as células têm espaço depois do `|` (`| 1. Pick 5...`), por isso o parser tolera `^\|\s*\d+\.`.

Resultado: `data/wiki/diary_tasks.json` (script `data/wiki/scripts/build_diaries.py`): 48 blocos área/tier, **492 tarefas**, cada uma `{n, task, skill_reqs:[{skill,level}], quest_reqs:[...], requirements_raw}`. A contagem por tier bate 48/48 com os arrays `tasks` do WikiSync, então o índice `n-1` é a posição no array do WikiSync e, no plugin próprio, a ordem das linhas da interface de diary.

Varbits de diary (bucket `varbit`, `where('content','Achievement Diary')`, 147 linhas em `varbit_achievement_diary_all.json`): por área/tier há `DIARY_<AREA>_<TIER>_COMPLETED` (ex.: Ardougne 4458/4459/4460/4461), `_COLLECTED` (recompensa pega, 4499..4502) e `_COUNT` (tarefas feitas, 6291..6294). O RuneLite expõe os mesmos em `Varbits.DIARY_ARDOUGNE_EASY = 4458` (deprecated) e `VarbitID.ARDOUGNE_DIARY_EASY_COMPLETE = 4458`. Os varbits de **cada tarefa** individual (ex.: `ATJUN_EASY_BANANA = 3566` para Karamja easy 1) existem em `VarbitID.java` mas o nome é por codinome interno, não por área; para mapear tarefa → varbit o caminho é o manifest do WikiSync (lista exatamente os varbits que o wiki usa) cruzado com chisel.

---

## 6. Varbits, chisel e plugin RuneLite próprio

### Onde achar IDs

- **Bucket `varbit` do wiki** (mais simples): `bucket('varbit').select('page_name','content','name','index').where('content','Dragon Slayer II').run()` → `[{"page_name":"RuneScape:Varbit/6104","index":6104,"content":"Dragon Slayer II","name":"DRAGON_SLAYER_II_PROGRESS"}, {"index":6140,"name":"DRAGON_SLAYER_II_MAKE_SUPER_ANTIFIRE_POTIONS"}]`. `content` é a página do jogo a que o var se refere (quest, diary, minigame); `page_name` diferencia `Varbit/` de `Varplayer/` (ex.: `RuneScape:Varplayer/101` = `QUEST_POINTS`). Cada página `RuneScape:Varbit/NNNN` descreve os valores.
- **chisel** (https://chisel.weirdgloop.org): `/varbs/index` (1,4 MB de HTML, tabela de todos os varbits/varps com nome, contagens de uso em cs2/cache; readme em `/varbs/readme`; `?orderBy=&dir=`), `/varbs/display?varbit=6104` (valores observados, scripts que o usam, link de edição do wiki), e `/structs/` (SPA que carrega `structs/structs/all.json`, `structs/enums/all.json`, `structs/params/all.json`: structs/enums do cache, úteis para tabelas de CA, music, clog).
- **RuneLite** (`runelite/runelite`, `runelite-api/src/main/java/net/runelite/api/`): `Quest.java` (enum `NOME(id, "Nome")` com o **id de quest do cs2**, ex. `DRAGON_SLAYER_II(32, "Dragon Slayer II")`, `THE_FRACTURED_ARCHIVE(17027, ...)`), `QuestState.java` (`IN_PROGRESS, NOT_STARTED, FINISHED`), `Varbits.java`/`VarPlayer.java` (constantes antigas, deprecadas), `gameval/VarbitID.java` (13.8k linhas, nomes oficiais do cache) e `gameval/VarPlayerID.java`, `ScriptID.java`, `Skill.java` (enum na ordem oficial, com `SAILING`). Cópias em `data/samples/runelite/`.

### Como um plugin lê o estado

Trecho real de `Quest.java`:

```java
public QuestState getState(Client client) {
    client.runScript(ScriptID.QUEST_STATUS_GET, id);   // QUEST_STATUS_GET = 4029
    switch (client.getIntStack()[0]) {
        case 2:  return QuestState.FINISHED;
        case 1:  return QuestState.NOT_STARTED;
        default: return QuestState.IN_PROGRESS;
    }
}
```

Ou seja, **não é preciso conhecer varbit por quest**: `Quest.DRAGON_SLAYER_II.getState(client)` roda o script do próprio jogo. Precisa ser chamado na thread do cliente (`ClientThread.invoke`) e com o jogador logado. Observação: o retorno do script é 1 = não iniciada / 2 = concluída / outro = em progresso, diferente da codificação 0/1/2 do WikiSync.

Skills: `client.getRealSkillLevel(Skill)`, `client.getBoostedSkillLevel(Skill)`, `client.getSkillExperience(Skill)` (ou os arrays `getRealSkillLevels()` etc.), e o evento `StatChanged`. Varbits: `client.getVarbitValue(VarbitID.ARDOUGNE_DIARY_EASY_COMPLETE)` e o evento `VarbitChanged` (tem `getVarbitId()`/`getVarpId()`/`getValue()`), ideal para enviar deltas. Diary por tarefa: ler os varbits do manifest do WikiSync (ou `DIARY_*_COUNT` para progresso agregado). Combat achievements: o estado fica em varps `CA_TASK_COMPLETED_*` (bitfields; o cs2 `CA_TASKS_...` os lê) — mais fácil copiar a abordagem do WikiSync (varps do manifest) do que decodificar à mão. Collection log: o WikiSync monta um bitset ao abrir o clog (scripts `COLLECTION_DRAW_LIST` e varps `COLLECTION_ITEMS_*`), e a activity "Collections Logged" do hiscore dá o total sem plugin. Banco/inventário: `client.getItemContainer(InventoryID.BANK)` ao abrir o banco (evento `ItemContainerChanged`).

Plano mínimo do plugin: na primeira sincronização enviar tudo (todas as `Quest` com `getState`, skills, varbits/varps de uma lista hospedada pelo seu servidor igual ao manifest do WikiSync, clog se aberto); depois só deltas em `VarbitChanged`/`StatChanged`, com `POST /api/ingest` + token. Licença do WikiSync é BSD-2, pode copiar a estrutura.

---

## 7. TempleOSRS

Docs: https://templeosrs.com/api_doc.php (linkado do FAQ; texto extraído em `data/samples/templeosrs/api_doc.txt`). Sem autenticação para leitura; eles pedem para consultar só o necessário e usar endpoints de grupo quando for mais de um jogador.

- `GET https://templeosrs.com/api/player_info.php?player=iron%20ohzone&cloginfo=1` → `{"data":{"Username":"Iron Ohzone","Game mode":1,"GIM":0,"Last checked":"2026-11-03 22:48:41","Last changed":"...","collection_log":{"log_synced":1,"last_checked":"2026-10-20 18:23:32","last_changed":"..."},"Datapoint Cooldown":"-"}}`. `Game mode`: 0 Main, 1 IM, 2 UIM, 3 HCIM; `GIM` 12–15/22–25 para group iron.
- `GET /api/player_stats.php?player=NOME&bosses=1` → chaves planas `Attack`, `Attack_rank`, `Attack_level`, `Attack_ehp`, `<Boss>`, ... (mesmo conteúdo do hiscore + EHP/EHB). `player_gains.php?player=&time=SEGUNDOS`, `player_datapoints.php` (histórico).
- `GET /api/collection-log/player_collection_log.php?player=iron%20ohzone&categories=barrows_chests&includenames=1` → `{"data":{"player":"Iron Ohzone","game_mode":1,"last_checked":"...","total_collections_finished":1611,"total_collections_available":1734,"ehc":6287.9,"collections_hiscores_rank":54,"items":{"barrows_chests":[{"id":4732,"count":1,"date":"2024-12-31 12:00:00","name":"Karil's coif"}, ...]}}}`. Parâmetros: `categories` (lista em "List: Categories"), `onlyitems=1`, `includemissingitems=1`, `includenames=1`, `dateformat=unix`. Depende do jogador usar o plugin TempleOSRS (ou o clog sync deles) — Iron Ohzone sincronizou em 2026-10-20.
- Também: `player_names.php`, competições/grupos (`group_member_info.php?id=&details=1` inclui flag de clog sincronizado), `recent_200ms.php`, `ehp_by_rank.php`.

Útil só se você não quiser plugin próprio e já usar o plugin do Temple; cobre KC e collection log, nada de quest/diary.

---

## 8. O que cada fonte entrega × o que o planner precisa

| Necessidade | Hiscores | WOM | WikiSync | TempleOSRS | Wiki (Bucket/wikitext) | Plugin próprio |
|---|---|---|---|---|---|---|
| Skills (nível) | sim (rank -1 abaixo do corte) | sim | sim (nível real) | sim | — | sim (real + boosted) |
| XP por skill | sim | sim + histórico/ganhos | não | sim + histórico | — | sim |
| KC de bosses / activities | sim (92) | sim | não | sim | — | sim (via varps/chat) |
| Estado de quests | não | não | sim (0/1/2, 215 nomes) | não | requisitos, QP, série (`quests.json`) | sim (`Quest.getState`) |
| Tarefas de diary | não | não | sim (bool por tarefa, 492) | não | texto + requisitos (`diary_tasks.json`) | sim (varbits) |
| Combat achievements | não | não | sim (ids) | não | catálogo 655 (bucket) | sim (varps) |
| Collection log | só total ("Collections Logged") | não | sim (item ids) | sim (por categoria, com datas) | catálogo (`collection_log_source`, `infobox_item`) | sim (ao abrir o clog) |
| Itens no banco | não | não | não | não | — | só aqui |
| Unlocks diversos (fairy rings, spellbooks, etc.) | não | não | parcial (varbits do manifest, não expostos) | não | ids em `varbit` | sim |
| Detecção de modo iron | sim (404 por endpoint) | sim (`type`) | não | sim (`Game mode`) | — | sim (`AccountType`) |

### Estratégia de sync recomendada

1. **Base (fase 1)**: hiscore oficial `index_lite.json` no endpoint `_ironman` a cada 15–30 min (1 chamada/conta); guardar o JSON bruto em `player_snapshot`. Mapear por `name`, nunca por índice. Isso já auto-completa steps de `skill`, `boss_kc` e "Collections Logged".
2. **Histórico**: WOM como leitura (`/players/{name}`, `/snapshots/timeline`, `/gained`) para gráficos; `POST /players/{name}` no máximo a cada 1–6 h (ou só quando o usuário clicar em "atualizar"), com User-Agent próprio e, idealmente, API key. Não duplicar o que o hiscore já dá.
3. **Quests/diaries/CA/clog agora**: botão "Importar do WikiSync" (1 chamada manual, com o aviso da política do wiki na tela), traduzindo `quests` 0/1/2, `achievement_diaries[area][tier].tasks[i]`, `combat_achievements` (ids → nomes via bucket `combat_achievement`) e `collection_log` (item ids). Nunca em cron.
4. **Quests/diaries/CA/clog de verdade (fase 2)**: plugin RuneLite próprio copiando o modelo do WikiSync (manifest hospedado por você + deltas), `Quest.getState` para quests, varbits `DIARY_*` para diaries, `POST /api/ingest`. Aí o WikiSync vira opcional e o TempleOSRS desnecessário.
5. **Dados estáticos (grafo de requisitos)**: importar `data/wiki/quests.json` e `data/wiki/diary_tasks.json` no boot; regenerar com os scripts em `data/wiki/scripts/` mensalmente (1 chamada Bucket + 5 de wikitext + 12 de diaries). Usar `aliases` para casar com WikiSync/RuneLite e um mapa de aliases para os nomes do guia (7 divergências listadas acima).

### Inventário dos samples

`data/samples/hiscores/` (JSON/CSV de iron, UIM, 404), `wiseoldman/` (player, snapshots, gained, timeline, achievements, POST update, 404, docs em txt), `wikisync/` (manifest, 5 jogadores com dados, 400 sem dados, política em wikitext), `wiki-bucket/` (schemas, dump de quests, CAs, varbits de diary, docs do Bucket), `wiki-wikitext/` (wikitext de 225 quests, 12 diaries, Quests/List), `templeosrs/` (api_doc, player_info, player_stats, collection log), `runelite/` (Quest, QuestState, Varbits, VarPlayer, VarbitID, VarPlayerID, Client, ScriptID, Skill, WikiSyncPlugin), `chisel/` (structs.js, varbs index/readme).
