// A real answer of GET /api/account/state, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { AccountState } from '../../progress/types';

export const fixture = {
	"account": {
		"guide_id": "oziris-main",
		"id": "acc_contract",
		"layer": "enhanced",
		"mode": "ironman",
		"rsn": "ironman260"
	},
	"collection_log_pages": [],
	"combat_achievements": [],
	"containers": [
		{
			"complete": true,
			"container": "inventory",
			"items": [
				{
					"id": 314,
					"name": "Feather",
					"qty": 60
				}
			],
			"observed_at": "2026-11-16T00:02:16-03:00",
			"state": "live"
		},
		{
			"complete": true,
			"container": "equipment",
			"items": [],
			"observed_at": "2026-11-15T23:48:16-03:00",
			"state": "live"
		},
		{
			"complete": true,
			"container": "bank",
			"items": [
				{
					"id": 995,
					"name": "Coins",
					"qty": 25000
				},
				{
					"id": 1351,
					"name": "Bronze axe",
					"qty": 1,
					"slot": "weapon"
				}
			],
			"observed_at": "2026-11-15T23:48:16-03:00",
			"state": "live"
		}
	],
	"diaries": [],
	"minigame": [
		{
			"key": "mta.alchemist",
			"observed_at": "2026-11-15T23:48:16-03:00",
			"source": "plugin",
			"value": 140
		}
	],
	"quests": [
		{
			"id": "cooks_assistant",
			"name": "Cook's Assistant",
			"observed_at": "2026-11-15T23:48:16-03:00",
			"source": "plugin",
			"state": "FINISHED"
		}
	],
	"skills": [
		{
			"level": 99,
			"observed_at": "2026-11-16T00:03:18.807477-03:00",
			"skill": "attack",
			"source": "hiscores",
			"xp": 13082918
		}
	],
	"total_level": 1428
} satisfies AccountState;
