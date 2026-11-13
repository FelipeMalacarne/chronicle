// A real answer of GET /api/plan/next, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { PlanNext } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"item": {
		"key": "task:2",
		"kind": "task",
		"label": "Ir até Draynor Village",
		"position": 2
	},
	"plan": {
		"id": "pln_8474f08880cd2b07",
		"kind": "custom",
		"title": "Logística"
	},
	"skipped": [
		{
			"executable": true,
			"index": 0,
			"item": "task:0",
			"key": "task:0#0",
			"label": "Banco: pegar 10 Feather",
			"risks": [],
			"state": "met",
			"supervised": false,
			"task": {
				"items": [
					{
						"item_id": 314,
						"name": "Feather",
						"qty": 10
					}
				],
				"type": "bank"
			},
			"type": "bank",
			"until": "inventário e equipamento prontos"
		},
		{
			"executable": true,
			"index": 0,
			"item": "task:1",
			"key": "task:1#0",
			"label": "Comprar Bronze axe em Bob's Brilliant Axes.",
			"risks": [],
			"state": "met",
			"supervised": false,
			"task": {
				"item": "item:1351",
				"qty": 1,
				"shop": "shop:bobs_brilliant_axes",
				"type": "buy"
			},
			"type": "buy",
			"until": "Bronze axe"
		}
	],
	"status": "task",
	"task": {
		"executable": true,
		"index": 0,
		"item": "task:2",
		"key": "task:2#0",
		"label": "Ir até Draynor Village",
		"reason": "sem posição: ligue Send position",
		"risks": [],
		"state": "unknown",
		"supervised": false,
		"task": {
			"location": "location:draynor_village",
			"type": "travel"
		},
		"type": "travel",
		"until": "na área do local"
	}
} satisfies PlanNext;
