// A real answer of GET /api/plans, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { PlansResponse } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"active": "pln_8474f08880cd2b07",
	"plans": [
		{
			"guide": "oziris-main",
			"id": "route:oziris-main",
			"items": [],
			"kind": "route",
			"title": "OSRS Ironman Efficiency Guide (OzirisLoL)"
		},
		{
			"id": "pln_8474f08880cd2b07",
			"items": [
				{
					"kind": "task",
					"task": {
						"items": [
							{
								"item_id": 314,
								"name": "Feather",
								"qty": 10
							}
						],
						"type": "bank"
					}
				},
				{
					"kind": "task",
					"task": {
						"item": "item:1351",
						"qty": 1,
						"shop": "shop:bobs_brilliant_axes",
						"type": "buy"
					}
				},
				{
					"kind": "task",
					"task": {
						"location": "location:draynor_village",
						"type": "travel"
					}
				},
				{
					"guide": "oziris-main",
					"kind": "step",
					"step": "1.1.2"
				}
			],
			"kind": "custom",
			"title": "Logística"
		}
	]
} satisfies PlansResponse;
