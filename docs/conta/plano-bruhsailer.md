# Plano da conta: troca de Oziris para BRUHsailer

Registro de decisão da conta `acc_eventsourced` (EventSourced). Retrato tirado em 2026-11-08 a partir
do planner: níveis por hiscores e plugin, quests e diaries pelo plugin, inventário, equipamento e banco
`live` e completos. Créditos das rotas: Oziris Ironman Efficiency Guide (OzirisLoL / ironman.guide) e
BRUHsailer (So Iron BRUH e ParasailerOSRS).

## 1. Decisão

- **Trocar a rota ativa de `oziris-main` para `bruhsailer-main`** assim que o barbarian fishing chegar a
  50 Agility. As duas rotas fazem exatamente esse grind nesse ponto (Oziris `1.2.16`, BRUHsailer CH1.31),
  então nada se perde.
- **Thieving segue o BRUHsailer, não o 99 do Oziris (seção 1.3).** O BRUHsailer também faz
  blackjacking: bearded bandits até 49 (`bruh-1.2-b63ba9348b08`), depois até juntar 1,25m GP, "level 77
  thieving, lower if you did more wintertodt" (`bruh-1.2-2028455e49a1`; bearded até 55, unbearded até
  65, menaphite thugs de 65 em diante). Daí em diante sobe por arteglass (84 em 2.2, 91 em 3.2, que
  também dão Crafting), Dorgesh-Kaan rich chest em 78 e o segundo sceptre do Pyramid Plunder a partir de
  91. Como o blackjacking já está automatizado (plugin blackjacker), passar de ~77 por ele é escolha do
  dono: custa horas de conta, não de atenção, e troca o Crafting do arteglass por GP.
- **Por que o BRUHsailer:** Sailing está tecido na rota inteira (port courier, charting, salvaging),
  enquanto o Oziris tem só uma trilha de Sailing à parte (12 passos); os grinds dele treinam várias
  skills ao mesmo tempo; e a conta está no fim do early game, o ponto mais barato para trocar.
- **O que não muda no planner:** as rotas continuam independentes (nunca se misturam). As marcações do
  Oziris ficam onde estão; o progresso por evidência (quests, níveis, itens) vale nas duas rotas.

### Durante o barbarian fishing atual

- Cut-eat os peixes em vez de dropar: Cooking está em 47 e o BRUHsailer precisa de 70 para os
  subquests de Recipe for Disaster. Sem isso, é treinar no mess hall de Hosidius depois.
- Guarde a heavy fishing rod: o BRUHsailer volta a usá-la (barbarian fishing na CoX e Barblore).
- A ida do meio (BRUHsailer pausa em 32 Agility para Waterfall Quest) não se aplica: Waterfall já está
  feita. Pesque direto até 50.

## 2. Os grinds do BRUHsailer

Os métodos de treino que motivaram a troca, com o passo equivalente do Oziris quando existe.

| Skill | Grind do BRUHsailer | Passo | Oziris faz |
|---|---|---|---|
| Fishing + Agility (+ Strength, Cooking) | Barbarian fishing na CoX (Mount Quidamortem) até **65 Agility / ~88 Fishing**, opcionalmente até **99 Fishing**; cut-eat para Cooking; Barblore (AFK, bancando sturgeons) | `bruh-2.2-da08d05c2a29`, `bruh-2.2-06f9f2ccf951` | Mesmo método, mas para em 50 Agility / 74 Fishing (`1.2.16`) |
| Agility | **Hallowed Sepulchre** floor 4 até **84**, depois opcionalmente "Fletchpalchre" até **98** (fletch headless arrows + High Alchemy correndo) | `bruh-2.2-be73f7328c3b` | Rooftops: Canifis 50–60, Seers 60+ até graceful (`1.4.43`) |
| Thieving + Crafting | **Arteglass** (artefatos em Port Piscarilius + glassblowing): unpowered orbs cedo, depois **84 Thieving** e **70 Crafting** | `bruh-1.4-5fcc565b7ed8`, `bruh-3.1-d54a651e72e9` | Blackjack até 50 (`1.2.6`), depois 99 Thieving (seção 1.3) |
| Mining | **3-tick sandstone** (alimenta o arteglass), **Motherlode** (coal bag e gem bag), **3t gem mining até 97** depois do Karamja Hard | `bruh-2.2-77a91e47362f`, `bruh-3.1-9f47e4966d68` | Sem grind equivalente encontrado |
| Firemaking + Fletching | **"Hoptodt"**: entrar em Wintertodt a 60–70%, fletch até 500 pontos, trocar de mundo; até ~99 FM deixa Fletching na casa dos 60 | `bruh-1.1-2a46d8b587d2` | Wintertodt até 200k e 22 Fletching (`1.1.82`) |
| Runecraft + skilling | **Guardians of the Rift multiskill** até acabarem os uncut gems | `bruh-3.1-66432b00a2de` | — |

Outros grinds que valem:

- **Tempoross em mass world até 56 Fishing**: além do XP, dá planks, nails, peixe, gold bars e gems que a
  rota usa depois.
- **Tithe Farm até 62 Farming** (`bruh-1.4-f29fb6065a73`), depois **200–250 farming contracts** para
  bancar ervas para 70 Herblore via Barblore.
- **Teak Homes** (Mahogany Homes com teak, só Adept) por 352 carpenter points (`bruh-2.1-6456852fe14d`);
  depois **mahogany benches até 83 Construction** com planks da Miscellania (`bruh-3.2-15c96591367f`).
- **Blue crabs na Great Conch até ~61 Hunter** com fish offcuts (`bruh-2.2-68fb6e6f6386`).
- **Chinchompas até 92 Ranged** junto com Western Provinces Hard (`bruh-3.1`).
- **Green dragons com melee defensivo e thralls até 65 Defence** (`bruh-2.2-9c59a66bedd7`).
- **Gold (método UIM) até 86 Smithing** com dinheiro do Corrupted Gauntlet (`bruh-3.2`).
- **Sailing**: port courier até 14 e 50, charting até 22, salvaging até 55, método de Marlin rank até 67
  (`bruh-2.1` a `bruh-3.1`).

## 3. Onde a conta está no BRUHsailer 1.1–1.4

| Seção | Quests a terminar | Metas de skill | Observação |
|---|---|---|---|
| 1.1 Tutorial até Wintertodt | 4 | Firemaking 99 e Fletching 69 já passaram | Wintertodt feito |
| 1.2 Fishing e thieving | 4 | falta Agility 50 (CH1.31) | é o grind em andamento |
| 1.3 Bird runs | 19 | falta Mining 43 (CH1.51) | seeds de bird run já bancadas |
| 1.4 Farming, DT1, battlestaves | 30 | quase todas | o maior bloco de recuperação |

No total, 43 de 97 conclusões de quest de 1.1–1.4 já feitas e 16 de 28 metas de skill atingidas. O
board do BRUHsailer mostra 7/226 porque as marcações manuais do Oziris não passam para outra rota.

### Skills que faltam

| Skill | Hoje → meta | Passo |
|---|---|---|
| Agility | 34 → 50 | CH1.31 (barbarian fishing) |
| Mining | 39 → 43 | CH1.51 (sandstone) |
| Woodcutting | 64 → 70 | CH1.75 |
| Smithing | 35 → 40 | CH1.83 |
| Ranged | 1 → 43 | CH1.88 |
| Prayer | 21 → 43 | CH1.89 (green dragons) |
| Herblore | 20 → 40 | CH1.92 |
| Farming | 1 → 62 | CH1.93 (Tithe Farm) |
| Hunter | 9 → 11, 15, 42, 46 | CH1.70 a CH1.95 |

### Quests que faltam

- **1.1**: Misthalin Mystery, Clock Tower, The Ribbiting Tale of a Lily Pad Labour Dispute, Tower of Life.
- **1.2**: Client of Kourend, Shield of Arrav, Ethically Acquired Antiquities, Death on the Isle.
- **1.3**: Priest in Peril, Nature Spirit, Lost City, The Fremennik Trials, The Golem, The Giant Dwarf,
  Elemental Workshop II, Shades of Mort'ton, RFD (Goblin Generals, Mountain Dwarf, Pirate Pete),
  Vampyre Slayer, Rag and Bone Man I, Bone Voyage, A Ruff Situation, The Ides of Milk.
- **1.4**: Death Plateau, Troll Stronghold, Eadgar's Ruse, Mountain Daughter, Fairytale I, A Porcine of
  Interest, Holy Grail, Prince Ali Rescue, Temple of Ikov, Desert Treasure I, Watchtower, Shadow of the
  Storm, The Hand in the Sand, Twilight's Promise, Forgettable Tale, Garden of Tranquillity, The Eyes of
  Glouphrie, The Corsair Curse, Below Ice Mountain, Enlightened Journey, A Soul's Bane, My Arm's Big
  Adventure, Getting Ahead, Creature of Fenkenstrain e os de Kourend (The Queen of Thieves, The Depths of
  Despair, The Forsaken Tower, Tale of the Righteous, The Ascent of Arceuus, The Garden of Death).

## 4. Itens que o BRUHsailer espera

Rastreado a partir da lista de itens de cada passo das anotações, contra inventário, equipamento e banco
`live`. Ficaram de fora os itens de quest pegos no próprio passo. "Origem" é o passo do BRUHsailer onde
a rota obtém o item; "—" quando a rota não registra (compra ou crafting por conta própria).

### Método das earth battlestaves (fim da seção 1.4)

| Item | Tem | Precisa | Origem |
|---|---|---|---|
| Unpowered orbs | 0 | 1590 | arteglass, CH1.91 |
| Cosmic runes | 295 | ~4770 | Mage Arena CH1.27, Tal Teklan CH1.97 |
| Paddewwa teleports | 0 | 33 | exige Desert Treasure I |
| Earth orbs | 0 | 100 até CH2.1 | carregadas em CH1.97 |
| Buckets of sand / soda ash | 595 / 595 | 1600 / 1600 | a partir de CH1.9 |
| Gold bars | 5 | ~1100 | — (loot de Tempoross, Blast Furnace) |
| Sapphires / emeralds / rubies | 0 / 1 / 0 | 16 / 41 / 51 | — (loot de Tempoross) |
| Nature runes | 448 | 1650 | CH1.34 |
| Law runes | 93 | 600 (67 passos) | CH1.34 |
| Steel nails | 0 | 263 | CH1.63 |
| Soft clay | 4 | 108 | CH1.24 |
| Arrow shafts | 20 | 1500 | CH1.14 (feathers: 16.890, já cobertos) |

### Teleportes

| Item | Tem | Precisa | Origem |
|---|---|---|---|
| Ring of dueling | 0 | 5 | — (crafting próprio) |
| Games necklace | 0 | 7 | — (crafting próprio) |
| Necklace of passage | 0 | 1 | — (crafting próprio) |
| Digsite pendant | 0 | 1 | — |
| Ring of charos(a) | 0 | 1 (9 passos) | — (a partir de CH1.80) |
| Kharedst's memoirs | 0 | 1 | — (a partir de CH1.76) |
| Staff of air / earth / water | 0 / 0 / 0 | 1 cada (20 / 18 / 11 passos) | CH1.22; hoje só há staff of fire equipado |

### Equipamento e ferramentas

| Item | Usado em | Origem |
|---|---|---|
| Rune sword | 29 passos | CH1.39 |
| Dramen staff | 25 passos | Lost City |
| Rune axe | 14 passos | a partir de CH1.75 |
| Lockpick | 6 passos | CH1.32 |
| Anti-dragon shield | 4 passos | — |
| Climbing boots | 3 passos | CH1.65 |
| Forestry kit | 3 passos | CH1.3 |
| Magic secateurs | 3 passos | Fairytale I (CH1.68) |

### Suprimentos recorrentes

Prayer potions (29 passos a partir de CH1.95), swordfish (25 passos), energy potions (16 passos na seção 2).

### Já cobertos

Seeds de bird run (2.000+ de marigold, rosemary, hammerstone e outras), compost, teak logs, feathers.

### Fora do alcance do planner

Tool leprechaun, costume room da casa, STASH units e seed vault não são containers do plugin. Rake, seed
dibber, watering cans, trowel, secateurs e plant pots aparecem como 0 mas devem estar no leprechaun; se
rune sword, staffs ou joias de teleporte estiverem guardados num desses lugares, também aparecem como 0.

## 5. Como trocar

1. Ao chegar em 50 Agility: `just account-guide acc_eventsourced bruhsailer-main`.
2. Percorrer 1.1 → 1.4 em ordem. Passos de quest e nível que já estão feitos fecham sozinhos pela
   evidência; os demais se marcam ao passar (marcar um passo adiante oferece marcar os anteriores).
3. Usar a seção 4 como lista de compras e crafting. Tempoross e arteglass, feitos quando a rota pede,
   cobrem a maior parte de gold, gems e orbs.
