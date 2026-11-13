// A real answer of GET /api/guides/oziris-main/board, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { Board } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"active": true,
	"counts": {
		"auto_done": 4,
		"blocked": 13,
		"done": 5,
		"implied": 0,
		"manual_done": 1,
		"pinned_open": 0,
		"remaining": 561,
		"skipped": 0,
		"total": 566
	},
	"coverage": {
		"checkbox": 227,
		"checkbox_done": 1,
		"checkbox_skipped": 0,
		"met": 5,
		"objectives": 464
	},
	"cursor": {
		"cause": "auto",
		"step_id": "1.1.2"
	},
	"guide": "oziris-main",
	"orphans": [],
	"sections": [
		{
			"counts": {
				"auto_done": 4,
				"blocked": 0,
				"done": 5,
				"implied": 0,
				"manual_done": 1,
				"pinned_open": 0,
				"remaining": 284,
				"skipped": 0,
				"total": 289
			},
			"id": "1.1",
			"title": "Early quests, wintertodt and ardy cloak 1",
			"track": false
		}
	],
	"steps": [
		{
			"id": "1.1.1",
			"main": true,
			"manual": true,
			"mark": {
				"kind": "manual_done"
			},
			"objectives": [],
			"ref": "1.1.1",
			"section_id": "1.1",
			"status": "manual_done",
			"title": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island",
			"visible": true
		},
		{
			"id": "1.1.2",
			"main": true,
			"manual": true,
			"objectives": [],
			"ref": "1.1.2",
			"section_id": "1.1",
			"status": "pending",
			"title": "Use every book and lamp on Herblore UNTIL 77 herb, after that they should be used on prayer (HCIM only), agility or RC, whichever you dislik…",
			"visible": true
		},
		{
			"id": "1.1.5",
			"main": true,
			"manual": false,
			"objectives": [
				{
					"key": "quest:x_marks_the_spot@started",
					"role": "starts",
					"verdict": "met"
				},
				{
					"key": "item:952x1",
					"observed": {
						"target": 1,
						"value": 1
					},
					"role": "completes",
					"verdict": "met"
				}
			],
			"ref": "1.1.5",
			"section_id": "1.1",
			"status": "auto_done",
			"title": "Buy a spade, start X marks the spot quest",
			"visible": true
		},
		{
			"id": "1.1.7",
			"main": true,
			"manual": false,
			"objectives": [
				{
					"key": "item:1965x1",
					"observed": {
						"target": 1,
						"value": 0
					},
					"role": "completes",
					"verdict": "unmet"
				}
			],
			"ref": "1.1.7",
			"section_id": "1.1",
			"status": "pending",
			"title": "Pick up every item spawn in the castle, including the ones in cellar, don't forget the cabbage",
			"visible": true
		},
		{
			"id": "1.1.9",
			"main": true,
			"manual": false,
			"objectives": [
				{
					"key": "quest:rune_mysteries@started",
					"role": "starts",
					"verdict": "unknown"
				}
			],
			"ref": "1.1.9",
			"section_id": "1.1",
			"status": "pending",
			"title": "Talk to the duke to start Rune mysteries quest",
			"visible": true
		},
		{
			"blocked_by": [
				"skill:woodcutting\u003e=35"
			],
			"id": "1.3.66",
			"main": true,
			"manual": false,
			"objectives": [
				{
					"key": "quest:animal_magnetism@finished",
					"role": "completes",
					"verdict": "unknown"
				}
			],
			"ref": "1.3.66",
			"section_id": "1.3",
			"status": "pending",
			"title": "Do Animal magnetism",
			"visible": true
		}
	]
} satisfies Board;
