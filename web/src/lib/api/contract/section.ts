// A real answer of GET /api/guides/oziris-main/sections/1.1, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { ContentSection } from '../../progress/types';

export const fixture = {
	"description": "Starting your ironman journey. Complete early quests, get 50+ firemaking at Wintertodt, and unlock the Ardougne cloak 1 for its teleport.",
	"guide": "oziris-main",
	"id": "1.1",
	"kind": "main",
	"names": {
		"diary:ardougne:easy": "Ardougne Easy Diary",
		"item:10006": "Bird snare",
		"item:10010": "Butterfly net",
		"item:10012": "Butterfly jar",
		"item:1013": "Pink skirt",
		"item:1017": "Wizard hat",
		"item:1059": "Leather gloves",
		"item:1101": "Iron chainbody",
		"item:11171": "Newspaper",
		"item:1139": "Bronze med helm",
		"item:12297": "Black pickaxe",
		"item:1265": "Bronze pickaxe",
		"item:1267": "Iron pickaxe",
		"item:1269": "Steel pickaxe",
		"item:1271": "Adamant pickaxe",
		"item:1273": "Mithril pickaxe",
		"item:1275": "Rune pickaxe",
		"item:12859": "Eye of newt pack",
		"item:13121": "Ardougne cloak 1",
		"item:1333": "Rune scimitar",
		"item:13421": "Saltpetre",
		"item:1349": "Iron axe",
		"item:1351": "Bronze axe",
		"item:1353": "Steel axe",
		"item:1355": "Mithril axe",
		"item:1357": "Adamant axe",
		"item:1359": "Rune axe",
		"item:1361": "Black axe",
		"item:13658": "Teleport card",
		"item:13660": "Chronicle",
		"item:1387": "Staff of fire",
		"item:1438": "Air talisman",
		"item:1470": "Red bead",
		"item:1472": "Yellow bead",
		"item:1474": "Black bead",
		"item:1476": "White bead",
		"item:1504": "Hangover cure",
		"item:1506": "Gas mask",
		"item:1511": "Logs",
		"item:1549": "Stake",
		"item:1550": "Garlic",
		"item:1552": "Seasoned sardine",
		"item:1599": "Holy mould",
		"item:1733": "Needle",
		"item:1734": "Thread",
		"item:1759": "Ball of wool",
		"item:1761": "Soft clay",
		"item:1763": "Red dye",
		"item:1765": "Yellow dye",
		"item:1767": "Blue dye",
		"item:1769": "Orange dye",
		"item:1775": "Molten glass",
		"item:1779": "Flax",
		"item:1781": "Soda ash",
		"item:1783": "Bucket of sand",
		"item:1793": "Woad leaf",
		"item:1907": "Wizard's mind bomb",
		"item:1909": "Greenman's ale",
		"item:1917": "Beer",
		"item:1925": "Bucket",
		"item:1927": "Bucket of milk",
		"item:1929": "Bucket of water",
		"item:1931": "Pot",
		"item:1933": "Pot of flour",
		"item:1935": "Jug",
		"item:1937": "Jug of water",
		"item:1939": "Swamp tar",
		"item:1941": "Swamp paste",
		"item:1944": "Egg",
		"item:1951": "Redberries",
		"item:1957": "Onion",
		"item:1965": "Cabbage",
		"item:19691": "Clue hunter gloves",
		"item:19695": "Clue hunter boots",
		"item:19704": "Compost pack",
		"item:1975": "Chocolate dust",
		"item:1978": "Cup of tea",
		"item:1985": "Cheese",
		"item:1993": "Jug of wine",
		"item:2015": "Vodka",
		"item:2025": "Cocktail shaker",
		"item:2026": "Cocktail glass",
		"item:2028": "Premade blurb' sp.",
		"item:2102": "Lemon",
		"item:2108": "Orange",
		"item:2114": "Pineapple",
		"item:2116": "Pineapple chunks",
		"item:2126": "Dwellberries",
		"item:21314": "Rainbow scarf",
		"item:2132": "Raw beef",
		"item:2134": "Raw rat meat",
		"item:2136": "Raw bear meat",
		"item:2138": "Raw chicken",
		"item:2146": "Burnt meat",
		"item:221": "Eye of newt",
		"item:22660": "Empty bucket pack",
		"item:229": "Vial",
		"item:2309": "Bread",
		"item:231": "Snape grass",
		"item:2313": "Pie dish",
		"item:2325": "Redberry pie",
		"item:233": "Pestle and mortar",
		"item:2347": "Hammer",
		"item:2349": "Bronze bar",
		"item:2351": "Iron bar",
		"item:2419": "Wig",
		"item:2423": "Key print",
		"item:2424": "Paste",
		"item:2518": "Rotten tomato",
		"item:27": "Fishing pass",
		"item:290": "Research package",
		"item:291": "Research notes",
		"item:294": "Glarial's pebble",
		"item:295": "Glarial's amulet",
		"item:296": "Glarial's urn",
		"item:2976": "Sickle mould",
		"item:3": "Nulodion's notes",
		"item:301": "Lobster pot",
		"item:303": "Small fishing net",
		"item:307": "Fishing rod",
		"item:314": "Feather",
		"item:315": "Shrimps",
		"item:317": "Raw shrimps",
		"item:321": "Raw anchovies",
		"item:33": "Lit candle",
		"item:35": "Excalibur",
		"item:36": "Candle",
		"item:38": "Black candle",
		"item:39": "Bronze arrowtips",
		"item:4": "Ammo mould",
		"item:418": "Plague sample",
		"item:419": "Touch paper",
		"item:426": "Priest gown",
		"item:428": "Priest gown",
		"item:4286": "Bucket of slime",
		"item:433": "Pirate message",
		"item:434": "Clay",
		"item:436": "Copper ore",
		"item:440": "Iron ore",
		"item:442": "Silver ore",
		"item:453": "Coal",
		"item:455": "Barcrawl card",
		"item:4819": "Bronze nails",
		"item:5008": "Brooch",
		"item:5009": "Goblin symbol book",
		"item:52": "Arrow shaft",
		"item:526": "Bones",
		"item:530": "Bat bones",
		"item:5518": "Scrying orb",
		"item:5519": "Scrying orb",
		"item:552": "Ghostspeak amulet",
		"item:553": "Ghost's skull",
		"item:554": "Fire rune",
		"item:555": "Water rune",
		"item:556": "Air rune",
		"item:557": "Earth rune",
		"item:558": "Mind rune",
		"item:560": "Death rune",
		"item:563": "Law rune",
		"item:590": "Tinderbox",
		"item:592": "Ashes",
		"item:6032": "Compost",
		"item:666": "Portrait",
		"item:667": "Blurite sword",
		"item:668": "Blurite ore",
		"item:686": "Rusty sword",
		"item:753": "Cadava berries",
		"item:755": "Message",
		"item:756": "Cadava potion",
		"item:783": "Bark sample",
		"item:784": "Translation book",
		"item:8009": "Falador teleport",
		"item:8013": "Teleport to house",
		"item:8790": "Bolt of cloth",
		"item:8794": "Saw",
		"item:88": "Boots of lightness",
		"item:882": "Bronze arrow",
		"item:9003": "Security book",
		"item:946": "Knife",
		"item:950": "Silk",
		"item:952": "Spade",
		"item:954": "Rope",
		"item:960": "Plank",
		"item:970": "Papyrus",
		"item:995": "Coins",
		"location:al_kharid": "Al Kharid",
		"location:ardougne": "Ardougne",
		"location:barbarian_assault": "Barbarian Assault",
		"location:barbarian_outpost": "Barbarian Outpost",
		"location:barbarian_village": "Barbarian Village",
		"location:baxtorian_falls": "Baxtorian Falls",
		"location:brimhaven": "Brimhaven",
		"location:camelot": "Camelot",
		"location:castle_wars": "Castle Wars",
		"location:catherby": "Catherby",
		"location:champions_guild": "Champions' Guild",
		"location:clan_wars": "Clan Wars",
		"location:digsite": "Digsite",
		"location:draynor_village": "Draynor Village",
		"location:falador": "Falador",
		"location:falador_park": "Falador Park",
		"location:ferox_enclave": "Ferox Enclave",
		"location:fight_arena": "Fight Arena",
		"location:fishing_guild": "Fishing Guild",
		"location:freds_farm": "Fred's Farm",
		"location:giants_foundry": "Giants' Foundry",
		"location:goblin_village": "Goblin Village",
		"location:great_kourend": "Great Kourend",
		"location:ham_hideout": "H.A.M. Hideout",
		"location:hazelmeres_island": "Hazelmere's Island",
		"location:hosidius": "Hosidius",
		"location:ice_mountain": "Ice Mountain",
		"location:karamja": "Karamja",
		"location:keep_le_faye": "Keep Le Faye",
		"location:kharidian_desert": "Kharidian Desert",
		"location:khazard_battlefield": "Khazard Battlefield",
		"location:lumber_yard": "Lumber Yard",
		"location:lumbridge": "Lumbridge",
		"location:lumbridge_castle": "Lumbridge Castle",
		"location:lumbridge_swamp": "Lumbridge Swamp",
		"location:mudskipper_point": "Mudskipper Point",
		"location:observatory": "Observatory",
		"location:ourania_altar": "Ourania Altar",
		"location:port_khazard": "Port Khazard",
		"location:port_sarim": "Port Sarim",
		"location:rimmington": "Rimmington",
		"location:seers_village": "Seers' Village",
		"location:shantay_pass": "Shantay Pass",
		"location:shilo_village": "Shilo Village",
		"location:taverley": "Taverley",
		"location:tower_of_life": "Tower of Life",
		"location:tree_gnome_stronghold": "Tree Gnome Stronghold",
		"location:varlamore": "Varlamore",
		"location:varrock": "Varrock",
		"location:varrock_museum": "Varrock Museum",
		"location:varrock_sawmill": "Varrock Sawmill",
		"location:white_wolf_mountain": "White Wolf Mountain",
		"location:wilderness": "Wilderness",
		"location:wintertodt": "Wintertodt",
		"location:witchaven": "Witchaven",
		"location:wizards_tower": "Wizards' Tower",
		"location:yanille": "Yanille",
		"quest:alfred_grimhands_barcrawl": "Alfred Grimhand's Barcrawl",
		"quest:biohazard": "Biohazard",
		"quest:black_knights_fortress": "Black Knights' Fortress",
		"quest:children_of_the_sun": "Children of the Sun",
		"quest:cooks_assistant": "Cook's Assistant",
		"quest:daddys_home": "Daddy's Home",
		"quest:demon_slayer": "Demon Slayer",
		"quest:dorics_quest": "Doric's Quest",
		"quest:dragon_slayer_i": "Dragon Slayer I",
		"quest:druidic_ritual": "Druidic Ritual",
		"quest:dwarf_cannon": "Dwarf Cannon",
		"quest:elemental_workshop_i": "Elemental Workshop I",
		"quest:enter_the_abyss": "Enter the Abyss",
		"quest:fight_arena": "Fight Arena",
		"quest:fishing_contest": "Fishing Contest",
		"quest:gertrudes_cat": "Gertrude's Cat",
		"quest:goblin_diplomacy": "Goblin Diplomacy",
		"quest:hazeel_cult": "Hazeel Cult",
		"quest:holy_grail": "Holy Grail",
		"quest:imp_catcher": "Imp Catcher",
		"quest:jungle_potion": "Jungle Potion",
		"quest:merlins_crystal": "Merlin's Crystal",
		"quest:monks_friend": "Monk's Friend",
		"quest:murder_mystery": "Murder Mystery",
		"quest:observatory_quest": "Observatory Quest",
		"quest:pandemonium": "Pandemonium",
		"quest:pirates_treasure": "Pirate's Treasure",
		"quest:plague_city": "Plague City",
		"quest:prince_ali_rescue": "Prince Ali Rescue",
		"quest:recipe_for_disaster": "Recipe for Disaster",
		"quest:recruitment_drive": "Recruitment Drive",
		"quest:romeo_and_juliet": "Romeo \u0026 Juliet",
		"quest:rune_mysteries": "Rune Mysteries",
		"quest:sea_slug": "Sea Slug",
		"quest:sheep_herder": "Sheep Herder",
		"quest:sheep_shearer": "Sheep Shearer",
		"quest:shield_of_arrav": "Shield of Arrav",
		"quest:shilo_village": "Shilo Village",
		"quest:sleeping_giants": "Sleeping Giants",
		"quest:temple_of_the_eye": "Temple of the Eye",
		"quest:the_dig_site": "The Dig Site",
		"quest:the_grand_tree": "The Grand Tree",
		"quest:the_knights_sword": "The Knight's Sword",
		"quest:the_lost_tribe": "The Lost Tribe",
		"quest:the_restless_ghost": "The Restless Ghost",
		"quest:the_tourist_trap": "The Tourist Trap",
		"quest:tree_gnome_village": "Tree Gnome Village",
		"quest:tribal_totem": "Tribal Totem",
		"quest:vampyre_slayer": "Vampyre Slayer",
		"quest:waterfall_quest": "Waterfall Quest",
		"quest:witchs_house": "Witch's House",
		"quest:witchs_potion": "Witch's Potion",
		"quest:x_marks_the_spot": "X Marks the Spot",
		"skill:construction": "Construction",
		"skill:cooking": "Cooking",
		"skill:firemaking": "Firemaking",
		"skill:fletching": "Fletching",
		"skill:hunter": "Hunter",
		"skill:magic": "Magic",
		"skill:runecraft": "Runecraft",
		"skill:slayer": "Slayer",
		"skill:smithing": "Smithing",
		"skill:thieving": "Thieving",
		"skill:woodcutting": "Woodcutting",
		"unlock:player_owned_house": "Player-owned house"
	},
	"related": [
		{
			"href": "/articles/osrs-ironman-magic-guide",
			"label": "Magic guide (1-99)"
		}
	],
	"segments": [
		{
			"id": "seg-2e7358630635",
			"kind": "sequence",
			"loadout": {
				"status": "unknown"
			},
			"source": {
				"confidence": "high",
				"evidence": "section start; no bank boundary asserted",
				"method": "derived",
				"step_ids": [
					"1.1.1"
				]
			},
			"steps": [
				{
					"id": "1.1.1",
					"layers": [
						"classic"
					],
					"modes": [
						"ironman"
					],
					"notes": [
						{
							"kind": "tip",
							"text": "Female char is for Recruitment Drive quest"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.1"
					},
					"text": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island"
				},
				{
					"complete_when": [
						{
							"level": 77,
							"meta": {
								"confidence": "medium",
								"inferred": true,
								"match": "UNTIL 77 herb",
								"origin": "text"
							},
							"skill": "herblore",
							"type": "skill_level"
						}
					],
					"entities": [
						{
							"field": "/complete_when/0/skill",
							"match": "name",
							"method": "rule",
							"ref": "skill:herblore",
							"source_text": "herblore"
						}
					],
					"id": "1.1.2",
					"layers": [
						"classic"
					],
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.2"
					},
					"tags": [
						"hcim"
					],
					"text": "Use every book and lamp on Herblore UNTIL 77 herb, after that they should be used on prayer (HCIM only), agility or RC, whichever you dislike the most"
				},
				{
					"id": "1.1.3",
					"layers": [
						"classic"
					],
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.3"
					},
					"text": "Use Authenticator AND 2-step verification on registered e-mail"
				},
				{
					"id": "1.1.4",
					"layers": [
						"classic"
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.4"
					},
					"text": "As soon as you spawn, sell bronze dagger, sword, axe, wooden shield and shortbow to the general store"
				},
				{
					"complete_when": [
						{
							"meta": {
								"confidence": "high",
								"origin": "questStatus"
							},
							"op": "gte",
							"quest": "X Marks the Spot",
							"state": "IN_PROGRESS",
							"type": "quest_state"
						}
					],
					"entities": [
						{
							"field": "/complete_when/0/quest",
							"match": "name",
							"method": "rule",
							"ref": "quest:x_marks_the_spot",
							"source_text": "X Marks the Spot"
						},
						{
							"field": "/text",
							"match": "name",
							"method": "text",
							"ref": "quest:x_marks_the_spot",
							"source_text": "X marks the spot",
							"span": [
								19
							]
						}
					],
					"id": "1.1.5",
					"layers": [
						"classic"
					],
					"links": [
						{
							"text": "X Marks the Spot",
							"type": "wiki",
							"url": "https://oldschool.runescape.wiki/w/X_Marks_the_Spot"
						}
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "start X marks the spot quest",
								"span": [
									13
								]
							},
							"key": "quest:x_marks_the_spot@started",
							"role": "starts",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.5"
					},
					"tags": [
						"quest"
					],
					"text": "Buy a spade, start X marks the spot quest"
				},
				{
					"id": "1.1.7",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "don't forget the cabbage",
								"span": [
									70
								]
							},
							"key": "item:1965x1",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.7"
					},
					"text": "Pick up every item spawn in the castle, including the ones in cellar, don't forget the cabbage"
				}
			],
			"title": "Sequence from 1.1.1"
		},
		{
			"bank_actions": [
				{
					"position": "end",
					"source": {
						"confidence": "high",
						"evidence": "bank everything",
						"method": "llm",
						"step_ids": [
							"1.1.10"
						]
					},
					"text": "bank everything",
					"type": "deposit_all"
				}
			],
			"id": "seg-2db1e200a25c",
			"kind": "bank_leg",
			"loadout": {
				"status": "unknown"
			},
			"source": {
				"confidence": "high",
				"evidence": "segment begins at an explicit bank action",
				"method": "derived",
				"step_ids": [
					"1.1.10"
				]
			},
			"steps": [
				{
					"id": "1.1.10",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.10"
					},
					"text": "Go to the top of the castle, bank everything"
				},
				{
					"complete_when": [
						{
							"level": 15,
							"meta": {
								"confidence": "high",
								"origin": "skillGoal"
							},
							"skill": "firemaking",
							"type": "skill_level"
						}
					],
					"entities": [
						{
							"field": "/complete_when/0/skill",
							"match": "name",
							"method": "rule",
							"ref": "skill:firemaking",
							"source_text": "firemaking"
						}
					],
					"id": "1.1.11",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "until 15 Firemaking",
								"span": [
									17
								]
							},
							"key": "skill:firemaking\u003e=15",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"requirements": [
						{
							"item_id": 1511,
							"name": "Logs",
							"qty": 4,
							"source_text": "the 4 logs"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.11"
					},
					"tags": [
						"skill"
					],
					"text": "Light the 4 logs until 15 Firemaking"
				},
				{
					"id": "1.1.12",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "fletch the same logs into 1000 arrow shafts",
								"span": [
									11
								]
							},
							"key": "item:52x1000",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"requirements": [
						{
							"item_id": 1511,
							"name": "Logs",
							"source_text": "the same logs"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.12"
					},
					"text": "After that fletch the same logs into 1000 arrow shafts"
				}
			],
			"title": "Bank leg from 1.1.10"
		},
		{
			"bank_actions": [
				{
					"position": "start",
					"source": {
						"confidence": "high",
						"evidence": "Bank at least 4 ashes",
						"method": "llm",
						"step_ids": [
							"1.1.13"
						]
					},
					"text": "Bank at least 4 ashes",
					"type": "deposit"
				}
			],
			"id": "seg-223fff4bb5ee",
			"kind": "bank_leg",
			"loadout": {
				"inventory": [
					{
						"item_id": 592,
						"name": "Ashes",
						"qty": 4,
						"source_text": "4 ashes"
					}
				],
				"source": {
					"confidence": "high",
					"evidence": "annotation items",
					"method": "llm",
					"step_ids": [
						"1.1.13"
					]
				},
				"status": "partial"
			},
			"source": {
				"confidence": "high",
				"evidence": "segment begins at an explicit bank action",
				"method": "derived",
				"step_ids": [
					"1.1.13"
				]
			},
			"steps": [
				{
					"id": "1.1.13",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"requirements": [
						{
							"item_id": 592,
							"name": "Ashes",
							"qty": 4,
							"source_text": "4 ashes"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.13"
					},
					"text": "Bank at least 4 ashes"
				}
			],
			"title": "Bank leg from 1.1.13"
		},
		{
			"bank_actions": [
				{
					"position": "start",
					"source": {
						"confidence": "high",
						"evidence": "Bank 7 logs for later",
						"method": "llm",
						"step_ids": [
							"1.1.14"
						]
					},
					"text": "Bank 7 logs for later",
					"type": "deposit"
				}
			],
			"id": "seg-7d7aa1f730f2",
			"kind": "bank_leg",
			"loadout": {
				"inventory": [
					{
						"item_id": 1511,
						"name": "Logs",
						"qty": 7,
						"source_text": "7 logs"
					}
				],
				"source": {
					"confidence": "high",
					"evidence": "annotation items",
					"method": "llm",
					"step_ids": [
						"1.1.14"
					]
				},
				"status": "partial"
			},
			"source": {
				"confidence": "high",
				"evidence": "segment begins at an explicit bank action",
				"method": "derived",
				"step_ids": [
					"1.1.14"
				]
			},
			"steps": [
				{
					"id": "1.1.14",
					"layers": [
						"classic"
					],
					"location": "Lumbridge Castle",
					"location_ref": "location:lumbridge_castle",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Bank 7 logs for later",
								"span": [
									0
								]
							},
							"key": "item:1511x7",
							"role": "uses",
							"source": {
								"confidence": "medium",
								"confirmed_by": "owner",
								"decision": "consumed-items-as-requirements",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"requirements": [
						{
							"item_id": 1511,
							"name": "Logs",
							"qty": 7,
							"source_text": "7 logs"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.14"
					},
					"text": "Bank 7 logs for later"
				}
			],
			"title": "Bank leg from 1.1.14"
		},
		{
			"bank_actions": [
				{
					"position": "start",
					"source": {
						"confidence": "high",
						"evidence": "Take out GP, Clue (x marks the spot), air talisman, spade, mind/air runes, bread and shrimp from the bank",
						"method": "llm",
						"step_ids": [
							"1.1.15"
						]
					},
					"text": "Take out GP, Clue (x marks the spot), air talisman, spade, mind/air runes, bread and shrimp from the bank",
					"type": "withdraw"
				}
			],
			"id": "seg-3e7e37adc5bd",
			"kind": "bank_leg",
			"loadout": {
				"inventory": [
					{
						"item_id": 995,
						"name": "GP",
						"source_text": "GP"
					},
					{
						"family": "treasure_scroll",
						"name": "Treasure scroll",
						"source_text": "Clue (x marks the spot)"
					}
				],
				"source": {
					"confidence": "high",
					"evidence": "annotation items",
					"method": "llm",
					"step_ids": [
						"1.1.15"
					]
				},
				"status": "partial"
			},
			"source": {
				"confidence": "high",
				"evidence": "segment begins at an explicit bank action",
				"method": "derived",
				"step_ids": [
					"1.1.15"
				]
			},
			"steps": [
				{
					"id": "1.1.15",
					"layers": [
						"classic"
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"requirements": [
						{
							"item_id": 995,
							"name": "GP",
							"source_text": "GP"
						},
						{
							"family": "treasure_scroll",
							"name": "Treasure scroll",
							"source_text": "Clue (x marks the spot)"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.15"
					},
					"text": "Take out GP, Clue (x marks the spot), air talisman, spade, mind/air runes, bread and shrimp from the bank"
				},
				{
					"complete_when": [
						{
							"level": 5,
							"meta": {
								"confidence": "high",
								"origin": "skillGoal"
							},
							"skill": "thieving",
							"type": "skill_level"
						}
					],
					"entities": [
						{
							"field": "/complete_when/0/skill",
							"match": "name",
							"method": "rule",
							"ref": "skill:thieving",
							"source_text": "thieving"
						}
					],
					"id": "1.1.16",
					"layers": [
						"classic"
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"notes": [
						{
							"kind": "hcim",
							"text": "HCIMs eat bread/shrimp, quickhop cabbages from basement if needed, normals can just suicide for hp"
						}
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "until 5 thieving",
								"span": [
									11
								]
							},
							"key": "skill:thieving\u003e=5",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.16"
					},
					"tags": [
						"hcim"
					],
					"text": "Thieve men until 5 thieving"
				},
				{
					"id": "1.1.17",
					"layers": [
						"classic"
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.17"
					},
					"text": "Dig up the clue north of bob's axes"
				},
				{
					"id": "1.1.18",
					"layers": [
						"classic"
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"notes": [
						{
							"kind": "tip",
							"text": "Make sure the world is full stock, should cost 200gp"
						}
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Buy a steel axe",
								"span": [
									0
								]
							},
							"key": "item:1353x1",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.18"
					},
					"text": "Buy a steel axe"
				},
				{
					"complete_when": [
						{
							"meta": {
								"confidence": "high",
								"origin": "questStatus"
							},
							"op": "gte",
							"quest": "The Restless Ghost",
							"state": "IN_PROGRESS",
							"type": "quest_state"
						}
					],
					"entities": [
						{
							"field": "/complete_when/0/quest",
							"match": "name",
							"method": "rule",
							"ref": "quest:the_restless_ghost",
							"source_text": "The Restless Ghost"
						},
						{
							"field": "/text",
							"match": "article",
							"method": "text",
							"ref": "quest:the_restless_ghost",
							"source_text": "Restless ghost",
							"span": [
								6
							]
						}
					],
					"id": "1.1.19",
					"layers": [
						"classic"
					],
					"links": [
						{
							"text": "The Restless Ghost",
							"type": "wiki",
							"url": "https://oldschool.runescape.wiki/w/The_Restless_Ghost"
						}
					],
					"location": "Lumbridge",
					"location_ref": "location:lumbridge",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Start Restless ghost",
								"span": [
									0
								]
							},
							"key": "quest:the_restless_ghost@started",
							"role": "starts",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.19"
					},
					"tags": [
						"quest"
					],
					"text": "Start Restless ghost by talking to Father Aereck"
				},
				{
					"id": "1.1.21",
					"layers": [
						"classic"
					],
					"links": [
						{
							"text": "Safespot location",
							"type": "image",
							"url": "https://i.imgur.com/EJimQCW.png"
						}
					],
					"location": "West of Lumbridge",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Kill a rat (safespot with wind strike) for its meat",
								"span": [
									0
								]
							},
							"key": "item:2134x1",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.21"
					},
					"text": "Kill a rat (safespot with wind strike) for its meat"
				}
			],
			"title": "Bank leg from 1.1.15"
		},
		{
			"bank_actions": [
				{
					"position": "end",
					"source": {
						"confidence": "high",
						"evidence": "keep 10 in bank",
						"method": "llm",
						"step_ids": [
							"1.1.48"
						]
					},
					"text": "keep 10 in bank",
					"type": "deposit"
				}
			],
			"id": "seg-998e2c8c65c6",
			"kind": "bank_leg",
			"loadout": {
				"status": "unknown"
			},
			"source": {
				"confidence": "high",
				"evidence": "segment begins at an explicit bank action",
				"method": "derived",
				"step_ids": [
					"1.1.48"
				]
			},
			"steps": [
				{
					"id": "1.1.48",
					"layers": [
						"classic"
					],
					"location": "Ardougne",
					"location_ref": "location:ardougne",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "keep 10 in bank for quests",
								"span": [
									30
								]
							},
							"key": "item:950x10",
							"role": "uses",
							"source": {
								"confidence": "medium",
								"confirmed_by": "owner",
								"decision": "consumed-items-as-requirements",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.48"
					},
					"text": "Sell your silk for 60gp each, keep 10 in bank for quests"
				},
				{
					"id": "1.1.49",
					"layers": [
						"classic"
					],
					"location": "Ardougne",
					"location_ref": "location:ardougne",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Buy a Player-owned house",
								"span": [
									38
								]
							},
							"key": "unlock:player_owned_house@done",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.49"
					},
					"text": "You should have 10k GP at this point. Buy a Player-owned house"
				},
				{
					"id": "1.1.50",
					"layers": [
						"classic"
					],
					"location": "Ardougne",
					"location_ref": "location:ardougne",
					"modes": [
						"ironman"
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.50"
					},
					"text": "Use the xp lamp from X marks the spot on Construction"
				},
				{
					"entities": [
						{
							"field": "/items/0",
							"match": "name",
							"method": "item",
							"ref": "item:954",
							"source_text": "Rope"
						}
					],
					"id": "1.1.51",
					"items": [
						"Rope"
					],
					"layers": [
						"classic"
					],
					"location": "Ardougne",
					"location_ref": "location:ardougne",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "2 ropes",
								"span": [
									4
								]
							},
							"key": "item:954x2",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						},
						{
							"evidence": {
								"field": "/text",
								"quote": "30 balls of wool",
								"span": [
									22
								]
							},
							"key": "item:1759x30",
							"role": "completes",
							"source": {
								"confidence": "medium",
								"confirmed_by": "owner",
								"decision": "item-shorthand-to-catalog-name",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"requirements": [
						{
							"item_id": 954,
							"name": "Rope",
							"source_text": "Rope"
						},
						{
							"family": "cake",
							"name": "cakes",
							"source_text": "Few cakes"
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.51"
					},
					"text": "Buy 2 ropes, 5 vials, 30 balls of wool and 7 papyrus from ardy general store"
				},
				{
					"id": "1.1.52",
					"layers": [
						"classic"
					],
					"links": [
						{
							"text": "Clue hunter location",
							"type": "image",
							"url": "http://i.imgur.com/sOD5Zhz.png"
						}
					],
					"location": "North of Ardougne",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Dig up clue hunter gloves and boots",
								"span": [
									0
								]
							},
							"key": "item:19691x1",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.52"
					},
					"text": "Dig up clue hunter gloves and boots north of Ardy"
				},
				{
					"id": "1.1.53",
					"layers": [
						"classic"
					],
					"location": "North of Ardougne",
					"modes": [
						"ironman"
					],
					"objectives": [
						{
							"evidence": {
								"field": "/text",
								"quote": "Buy greenman's ale",
								"span": [
									0
								]
							},
							"key": "item:1909x1",
							"role": "completes",
							"source": {
								"confidence": "high",
								"method": "llm",
								"skill_version": "annotate-guide@1"
							}
						}
					],
					"source": {
						"imported_at": "2026-11-03",
						"origin": "ironman.guide",
						"original_id": "1.1.53"
					},
					"text": "Buy greenman's ale from Rasolo"
				}
			],
			"title": "Bank leg from 1.1.48"
		}
	],
	"slug": "early-game",
	"steps": [
		{
			"guide": "oziris-main",
			"id": "1.1.1",
			"layers": [
				"classic"
			],
			"manual": true,
			"modes": [
				"ironman"
			],
			"names": {},
			"notes": [
				{
					"kind": "tip",
					"text": "Female char is for Recruitment Drive quest"
				}
			],
			"position": 1,
			"ref": "1.1.1",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.1"
			},
			"text": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island",
			"total": 564
		},
		{
			"complete_when": [
				{
					"level": 77,
					"meta": {
						"confidence": "medium",
						"inferred": true,
						"match": "UNTIL 77 herb",
						"origin": "text"
					},
					"skill": "herblore",
					"type": "skill_level"
				}
			],
			"entities": [
				{
					"field": "/complete_when/0/skill",
					"match": "name",
					"method": "rule",
					"ref": "skill:herblore",
					"source_text": "herblore"
				}
			],
			"guide": "oziris-main",
			"id": "1.1.2",
			"layers": [
				"classic"
			],
			"manual": false,
			"modes": [
				"ironman"
			],
			"names": {},
			"position": 2,
			"ref": "1.1.2",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.2"
			},
			"tags": [
				"hcim"
			],
			"text": "Use every book and lamp on Herblore UNTIL 77 herb, after that they should be used on prayer (HCIM only), agility or RC, whichever you dislike the most",
			"total": 564
		},
		{
			"guide": "oziris-main",
			"id": "1.1.3",
			"layers": [
				"classic"
			],
			"manual": true,
			"modes": [
				"ironman"
			],
			"names": {},
			"position": 3,
			"ref": "1.1.3",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.3"
			},
			"text": "Use Authenticator AND 2-step verification on registered e-mail",
			"total": 564
		},
		{
			"guide": "oziris-main",
			"id": "1.1.4",
			"layers": [
				"classic"
			],
			"location": "Lumbridge",
			"location_ref": "location:lumbridge",
			"manual": true,
			"modes": [
				"ironman"
			],
			"names": {
				"location:lumbridge": "Lumbridge"
			},
			"position": 4,
			"ref": "1.1.4",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.4"
			},
			"text": "As soon as you spawn, sell bronze dagger, sword, axe, wooden shield and shortbow to the general store",
			"total": 564
		},
		{
			"also": {
				"item:952x1": [
					{
						"guide": "bruhsailer-main",
						"key": "item:952x1",
						"position": 2,
						"ref": "CH1.2",
						"role": "completes",
						"section": "bruh-1.1",
						"step": "bruh-1.1-26cbe64705fd",
						"title": "Grab coins, a knife, 24 logs, your mind and air runes. Pickpocket men to 235 gp total, suiciding if you die (HCIM can grab cabbages by world…",
						"total": 226
					}
				],
				"quest:x_marks_the_spot@started": [
					{
						"guide": "bruhsailer-main",
						"key": "quest:x_marks_the_spot@started",
						"position": 2,
						"ref": "CH1.2",
						"role": "starts",
						"section": "bruh-1.1",
						"step": "bruh-1.1-26cbe64705fd",
						"title": "Grab coins, a knife, 24 logs, your mind and air runes. Pickpocket men to 235 gp total, suiciding if you die (HCIM can grab cabbages by world…",
						"total": 226
					}
				]
			},
			"complete_when": [
				{
					"meta": {
						"confidence": "high",
						"origin": "questStatus"
					},
					"op": "gte",
					"quest": "X Marks the Spot",
					"state": "IN_PROGRESS",
					"type": "quest_state"
				}
			],
			"entities": [
				{
					"field": "/complete_when/0/quest",
					"match": "name",
					"method": "rule",
					"ref": "quest:x_marks_the_spot",
					"source_text": "X Marks the Spot"
				},
				{
					"field": "/text",
					"match": "name",
					"method": "text",
					"ref": "quest:x_marks_the_spot",
					"source_text": "X marks the spot",
					"span": [
						19
					]
				}
			],
			"guide": "oziris-main",
			"id": "1.1.5",
			"layers": [
				"classic"
			],
			"links": [
				{
					"text": "X Marks the Spot",
					"type": "wiki",
					"url": "https://oldschool.runescape.wiki/w/X_Marks_the_Spot"
				}
			],
			"location": "Lumbridge",
			"location_ref": "location:lumbridge",
			"manual": false,
			"modes": [
				"ironman"
			],
			"names": {
				"item:952": "Spade",
				"location:lumbridge": "Lumbridge",
				"quest:x_marks_the_spot": "X Marks the Spot"
			},
			"objectives": [
				{
					"evidence": {
						"field": "/text",
						"quote": "start X marks the spot quest",
						"span": [
							13
						]
					},
					"key": "quest:x_marks_the_spot@started",
					"role": "starts",
					"source": {
						"confidence": "high",
						"method": "llm",
						"skill_version": "annotate-guide@1"
					}
				}
			],
			"position": 5,
			"ref": "1.1.5",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.5"
			},
			"tags": [
				"quest"
			],
			"text": "Buy a spade, start X marks the spot quest",
			"total": 564
		},
		{
			"also": {
				"item:1965x1": [
					{
						"guide": "bruhsailer-main",
						"key": "item:1965x1",
						"position": 1,
						"ref": "CH1.1",
						"role": "completes",
						"section": "bruh-1.1",
						"step": "bruh-1.1-e8d3227fd203",
						"title": "Choose a swag look for your account (older guides will mention that you need to start as a female character - this is no longer required). M…",
						"total": 226
					}
				]
			},
			"guide": "oziris-main",
			"id": "1.1.7",
			"layers": [
				"classic"
			],
			"location": "Lumbridge Castle",
			"location_ref": "location:lumbridge_castle",
			"manual": false,
			"modes": [
				"ironman"
			],
			"names": {
				"item:1965": "Cabbage",
				"location:lumbridge_castle": "Lumbridge Castle"
			},
			"objectives": [
				{
					"evidence": {
						"field": "/text",
						"quote": "don't forget the cabbage",
						"span": [
							70
						]
					},
					"key": "item:1965x1",
					"role": "completes",
					"source": {
						"confidence": "high",
						"method": "llm",
						"skill_version": "annotate-guide@1"
					}
				}
			],
			"position": 7,
			"ref": "1.1.7",
			"section": "1.1",
			"source": {
				"imported_at": "2026-11-03",
				"origin": "ironman.guide",
				"original_id": "1.1.7"
			},
			"text": "Pick up every item spawn in the castle, including the ones in cellar, don't forget the cabbage",
			"total": 564
		}
	],
	"title": "Early quests, wintertodt and ardy cloak 1"
} satisfies ContentSection;
