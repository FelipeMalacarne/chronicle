// A real answer of GET /api/world, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { WorldResponse } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"entities": [
		{
			"kind": "item",
			"name": "Knife",
			"objectives": [
				{
					"key": "item:946x1",
					"verdict": "unmet"
				}
			],
			"ref": "item:946",
			"routes": [
				{
					"guide": "bruhsailer-main",
					"key": "item:946x1",
					"position": 1,
					"ref": "CH1.1",
					"role": "completes",
					"section": "bruh-1.1",
					"step": "bruh-1.1-e8d3227fd203",
					"title": "Choose a swag look for your account (older guides will mention that you need to start as a female character - this is no longer required). M…",
					"total": 226
				}
			],
			"wiki": "https://oldschool.runescape.wiki/w/Knife"
		},
		{
			"kind": "skill",
			"name": "Woodcutting",
			"objectives": [
				{
					"key": "skill:woodcutting\u003e=35",
					"verdict": "unmet"
				}
			],
			"ref": "skill:woodcutting",
			"routes": [
				{
					"guide": "bruhsailer-main",
					"key": "skill:woodcutting\u003e=35",
					"position": 6,
					"ref": "CH1.6",
					"role": "completes",
					"section": "bruh-1.1",
					"step": "bruh-1.1-698825e36f25",
					"title": "TL;DR: do Clock Tower, Monk’s Friend, Hazeel Cult and get level 35 woodcutting, burning the logs and swapping to oaks at 15. Eat your food i…",
					"total": 226
				},
				{
					"guide": "oziris-main",
					"key": "skill:woodcutting\u003e=40",
					"ref": "S.7",
					"role": "requires",
					"section": "S",
					"step": "S.7",
					"title": "Complete Troubled Tortugans (Sailing 45, plus 51 Slayer / 48 Construction / 45 Hunter / 40 Woodcutting / 34 Crafting)",
					"total": 564
				}
			],
			"wiki": "https://oldschool.runescape.wiki/w/Woodcutting"
		},
		{
			"kind": "item",
			"name": "Games necklace",
			"objectives": [
				{
					"key": "item:family/games_necklacex1",
					"verdict": "unmet"
				}
			],
			"ref": "item:family/games_necklace",
			"routes": [
				{
					"guide": "bruhsailer-main",
					"key": "item:family/games_necklacex1",
					"position": 27,
					"ref": "CH1.27",
					"role": "completes",
					"section": "bruh-1.2",
					"step": "bruh-1.2-6092b29396cf",
					"title": "Fishing trawler teleport, do the fishing trawler minigame for a complete Angler’s outfit (48 games average, you can use an axe+hammer instea…",
					"total": 226
				}
			]
		},
		{
			"kind": "unlock",
			"name": "Sailing teleport focus",
			"objectives": [
				{
					"key": "unlock:sailing_teleport_focus@done",
					"verdict": "unknown"
				}
			],
			"ref": "unlock:sailing_teleport_focus",
			"routes": [
				{
					"guide": "oziris-main",
					"key": "unlock:sailing_teleport_focus@done",
					"ref": "S.9",
					"role": "completes",
					"section": "S",
					"step": "S.9",
					"title": "Build a teleport focus at Sailing 55 (needs 49 Construction) to unlock Sailing teleports",
					"total": 564
				}
			],
			"wiki": "https://oldschool.runescape.wiki/w/Teleport_focus"
		}
	]
} satisfies WorldResponse;
