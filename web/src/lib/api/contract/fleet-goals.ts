// A real answer of GET /api/fleet/goals, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { FleetGoalsResponse } from '../../progress/types';

export const fixture = {
	"accounts": [
		{
			"guide_id": "oziris-main",
			"id": "acc_contract",
			"layer": "enhanced",
			"mode": "ironman",
			"rsn": "ironman260"
		}
	],
	"goals": [
		{
			"accounts": [
				{
					"account": "acc_contract",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.73",
						"section_id": "1.1",
						"section_pos": 73,
						"step_id": "1.1.73",
						"title": "Home tele, Lumby"
					},
					"goal": {
						"achieved_at": "2026-11-13T01:15:36.173353-03:00",
						"created_at": "2026-11-13T01:15:36.043001-03:00",
						"id": "goal_88b38f7f9ba3e51feb535420",
						"label": "",
						"priority": 100,
						"status": "achieved"
					},
					"level": 99,
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 0,
						"unknown": 0
					},
					"short": [],
					"step": {
						"guide_id": "oziris-main",
						"ref": "1.1.219",
						"role": "completes",
						"section_id": "1.1",
						"section_pos": 222,
						"step_id": "1.1.219",
						"title": "Finish The Grand tree"
					},
					"steps_left": 0,
					"verdict": "met"
				},
				{
					"account": "acc_worker_a",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.1",
						"section_id": "1.1",
						"section_pos": 1,
						"step_id": "1.1.1",
						"title": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island"
					},
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 0,
						"unknown": 0
					},
					"short": [],
					"step": {
						"guide_id": "oziris-main",
						"ref": "1.1.219",
						"role": "completes",
						"section_id": "1.1",
						"section_pos": 222,
						"step_id": "1.1.219",
						"title": "Finish The Grand tree"
					},
					"steps_left": 222,
					"verdict": "unknown"
				}
			],
			"kind": "skill",
			"met": 1,
			"name": "Magic",
			"objective_key": "skill:magic\u003e=43",
			"pinned": 1,
			"ref": "skill:magic",
			"routes": [
				{
					"guide": "oziris-main",
					"key": "skill:magic\u003e=13",
					"position": 126,
					"ref": "1.1.126",
					"role": "completes",
					"section": "1.1",
					"step": "1.1.126",
					"title": "Safespot barbarians in the bar for 13 magic",
					"total": 566
				}
			],
			"unknown": 3,
			"wiki": "https://oldschool.runescape.wiki/w/Magic"
		},
		{
			"accounts": [
				{
					"account": "acc_contract",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.73",
						"section_id": "1.1",
						"section_pos": 73,
						"step_id": "1.1.73",
						"title": "Home tele, Lumby"
					},
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 1,
						"unknown": 1
					},
					"short": [],
					"step": {
						"guide_id": "oziris-main",
						"ref": "1.3.52",
						"role": "completes",
						"section_id": "1.3",
						"section_pos": 52,
						"step_id": "1.3.52",
						"title": "Finish dragon slayer and buy a green d'hide top from Oziach"
					},
					"steps_left": 296,
					"verdict": "unknown"
				},
				{
					"account": "acc_worker_a",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.1",
						"section_id": "1.1",
						"section_pos": 1,
						"step_id": "1.1.1",
						"title": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island"
					},
					"goal": {
						"created_at": "2026-11-13T01:15:36.048178-03:00",
						"id": "goal_ba63660c2c05232d58974b4a",
						"label": "",
						"priority": 100,
						"status": "active"
					},
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 1,
						"unknown": 1
					},
					"short": [],
					"step": {
						"guide_id": "oziris-main",
						"ref": "1.3.52",
						"role": "completes",
						"section_id": "1.3",
						"section_pos": 52,
						"step_id": "1.3.52",
						"title": "Finish dragon slayer and buy a green d'hide top from Oziach"
					},
					"steps_left": 374,
					"verdict": "unknown"
				}
			],
			"kind": "quest",
			"met": 0,
			"name": "Dragon Slayer I",
			"objective_key": "quest:dragon_slayer_i@finished",
			"pinned": 1,
			"ref": "quest:dragon_slayer_i",
			"routes": [
				{
					"guide": "bruhsailer-main",
					"key": "quest:dragon_slayer_i@started",
					"position": 39,
					"ref": "CH1.39",
					"role": "starts",
					"section": "bruh-1.3",
					"step": "bruh-1.3-886f0784d3aa",
					"title": "Chronicle teleport to Varrock, buy a rune sword (not a longsword!), green d’hide chaps, green d’hide vambraces from Scavvo in the Champions’…",
					"total": 226
				}
			],
			"unknown": 4,
			"wiki": "https://oldschool.runescape.wiki/w/Dragon_Slayer_I"
		},
		{
			"accounts": [
				{
					"account": "acc_contract",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.73",
						"section_id": "1.1",
						"section_pos": 73,
						"step_id": "1.1.73",
						"title": "Home tele, Lumby"
					},
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 0,
						"unknown": 0
					},
					"short": [],
					"step": {
						"done": true,
						"guide_id": "oziris-main",
						"ref": "1.1.177",
						"role": "completes",
						"section_id": "1.1",
						"section_pos": 179,
						"step_id": "1.1.177",
						"title": "Do cook's assistant"
					},
					"steps_left": 0,
					"verdict": "met"
				},
				{
					"account": "acc_worker_a",
					"cursor": {
						"guide_id": "oziris-main",
						"ref": "1.1.1",
						"section_id": "1.1",
						"section_pos": 1,
						"step_id": "1.1.1",
						"title": "Start as a female character, don't forget to make yourself an ironman before leaving tutorial island"
					},
					"requirements": {
						"met": 0,
						"presumed": 0,
						"quests_left": 0,
						"total": 0,
						"unknown": 0
					},
					"short": [],
					"step": {
						"guide_id": "oziris-main",
						"ref": "1.1.177",
						"role": "completes",
						"section_id": "1.1",
						"section_pos": 179,
						"step_id": "1.1.177",
						"title": "Do cook's assistant"
					},
					"steps_left": 179,
					"verdict": "unknown"
				}
			],
			"kind": "quest",
			"met": 1,
			"name": "Cook's Assistant",
			"objective_key": "quest:cooks_assistant@finished",
			"pinned": 0,
			"ref": "quest:cooks_assistant",
			"routes": [
				{
					"guide": "bruhsailer-main",
					"key": "quest:cooks_assistant@started",
					"position": 42,
					"ref": "CH1.42",
					"role": "starts",
					"section": "bruh-1.3",
					"step": "bruh-1.3-de6f39e70583",
					"title": "Teleport to Lumbridge, make a necklace of passage and two rings of dueling at the furnace. If you have extra gold/silver bars and the corres…",
					"total": 226
				}
			],
			"unknown": 3,
			"wiki": "https://oldschool.runescape.wiki/w/Cook's_Assistant"
		}
	]
} satisfies FleetGoalsResponse;
