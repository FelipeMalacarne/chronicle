// A real answer of PATCH /api/guides/oziris-main/steps/1.1.2/mark, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { Board } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"active": true,
	"command_id": "mc_767ae2a8c6de9f0d2d06bef2",
	"counts": {
		"auto_done": 4,
		"blocked": 13,
		"done": 5,
		"implied": 0,
		"manual_done": 1,
		"pinned_open": 0,
		"remaining": 558,
		"skipped": 1,
		"total": 564
	},
	"coverage": {
		"checkbox": 227,
		"checkbox_done": 1,
		"checkbox_skipped": 1,
		"met": 5,
		"objectives": 464
	},
	"cursor": {
		"cause": "auto",
		"step_id": "1.1.3"
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
				"remaining": 280,
				"skipped": 1,
				"total": 286
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
			"id": "1.1.3",
			"main": true,
			"manual": true,
			"objectives": [],
			"ref": "1.1.3",
			"section_id": "1.1",
			"status": "pending",
			"title": "Use Authenticator AND 2-step verification on registered e-mail",
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
				}
			],
			"ref": "1.1.5",
			"section_id": "1.1",
			"status": "auto_done",
			"title": "Buy a spade, start X marks the spot quest",
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
