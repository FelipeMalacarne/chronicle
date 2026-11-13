// A real answer of POST /api/guides/oziris-main/marks, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { Board } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"active": true,
	"command_id": "mc_8ce4f6989c17a9bfdf579c7b",
	"counts": {
		"auto_done": 4,
		"blocked": 13,
		"done": 73,
		"implied": 0,
		"manual_done": 69,
		"pinned_open": 0,
		"remaining": 492,
		"skipped": 1,
		"total": 566
	},
	"coverage": {
		"checkbox": 227,
		"checkbox_done": 30,
		"checkbox_skipped": 1,
		"met": 5,
		"objectives": 464
	},
	"cursor": {
		"cause": "auto",
		"step_id": "1.1.73"
	},
	"guide": "oziris-main",
	"marked": [
		"1.1.3"
	],
	"orphans": [],
	"sections": [
		{
			"counts": {
				"auto_done": 4,
				"blocked": 0,
				"done": 73,
				"implied": 0,
				"manual_done": 69,
				"pinned_open": 0,
				"remaining": 215,
				"skipped": 1,
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
			"mark": {
				"kind": "manual_done"
			},
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
			"status": "manual_done",
			"title": "Pick up every item spawn in the castle, including the ones in cellar, don't forget the cabbage",
			"visible": true
		},
		{
			"id": "1.1.9",
			"main": true,
			"manual": false,
			"mark": {
				"kind": "manual_done"
			},
			"objectives": [
				{
					"key": "quest:rune_mysteries@started",
					"role": "starts",
					"verdict": "unknown"
				}
			],
			"ref": "1.1.9",
			"section_id": "1.1",
			"status": "manual_done",
			"title": "Talk to the duke to start Rune mysteries quest",
			"visible": true
		},
		{
			"id": "1.1.11",
			"main": true,
			"manual": false,
			"objectives": [
				{
					"key": "skill:firemaking\u003e=15",
					"observed": {
						"target": 15,
						"value": 30
					},
					"role": "completes",
					"verdict": "met"
				}
			],
			"ref": "1.1.11",
			"section_id": "1.1",
			"status": "auto_done",
			"title": "Light the 4 logs until 15 Firemaking",
			"visible": true
		},
		{
			"id": "1.1.73",
			"main": true,
			"manual": true,
			"objectives": [],
			"ref": "1.1.73",
			"section_id": "1.1",
			"status": "pending",
			"title": "Home tele, Lumby",
			"visible": true
		}
	],
	"undo": [
		{
			"mark": null,
			"step_id": "1.1.3"
		}
	]
} satisfies Board;
