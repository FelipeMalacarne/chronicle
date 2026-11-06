// A real answer of GET /api/world/graph, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { GraphResponse } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"bottlenecks": [
		{
			"boostable": true,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "skill:magic\u003e=59",
			"level": 25,
			"met": false,
			"skill": "magic",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.4.45",
				"role": "completes",
				"section_id": "1.4",
				"section_pos": 47,
				"step_id": "1.4.45",
				"title": "Should get to 70+ mage by the time you're done with agility for now"
			},
			"target": 59
		},
		{
			"boostable": true,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "skill:firemaking\u003e=50",
			"level": 30,
			"met": false,
			"skill": "firemaking",
			"step": {
				"done": true,
				"guide_id": "oziris-main",
				"ref": "1.1.72",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 72,
				"step_id": "1.1.72",
				"title": "Run south-west of castle wars, cut and firemake teak logs until 50 Firemaking"
			},
			"target": 50
		},
		{
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "skill:agility\u003e=48",
			"level": null,
			"met": false,
			"skill": "agility",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.2.16",
				"role": "completes",
				"section_id": "1.2",
				"section_pos": 16,
				"step_id": "1.2.16",
				"title": "Get 50 agility from barb fishing, should be at 74 fishing"
			},
			"target": 48
		},
		{
			"boostable": true,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "skill:cooking\u003e=70",
			"level": null,
			"met": false,
			"reach": 15,
			"reach_step": {
				"guide_id": "oziris-main",
				"ref": "1.1.153",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 154,
				"step_id": "1.1.153",
				"title": "Cook whatever low level raw food you got from wintertodt for 15 cooking on the fire"
			},
			"skill": "cooking",
			"target": 70
		},
		{
			"boostable": true,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "skill:crafting\u003e=40",
			"level": null,
			"met": false,
			"skill": "crafting",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.3.105",
				"role": "completes",
				"section_id": "1.3",
				"section_pos": 107,
				"step_id": "1.3.105",
				"title": "Make molten glass and cut gems until 45 crafting. You'll need 45 for maple birdhouse when you get the 44 hunter"
			},
			"target": 40
		},
		{
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"key": "qp\u003e=175",
			"level": null,
			"met": false,
			"skill": "qp",
			"target": 175
		}
	],
	"critical": [
		{
			"key": "quest:recipe_for_disaster@finished",
			"label": "Recipe for Disaster"
		}
	],
	"depth": 3,
	"focus": "quest:recipe_for_disaster@finished",
	"hidden": 0,
	"kind": "quest",
	"label": "Recipe for Disaster",
	"max_depth": 1,
	"met": 1,
	"nodes": [
		{
			"boostable": true,
			"depth": 1,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "blocked",
			"have": 30,
			"key": "skill:firemaking\u003e=50",
			"kind": "skill",
			"label": "Firemaking",
			"ref": "skill:firemaking",
			"step": {
				"done": true,
				"guide_id": "oziris-main",
				"ref": "1.1.72",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 72,
				"step_id": "1.1.72",
				"title": "Run south-west of castle wars, cut and firemake teak logs until 50 Firemaking"
			},
			"verdict": "unmet"
		},
		{
			"boostable": true,
			"depth": 1,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "unknown",
			"key": "skill:fletching\u003e=10",
			"kind": "skill",
			"label": "Fletching",
			"ref": "skill:fletching",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.1.81",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 83,
				"step_id": "1.1.81",
				"title": "Should now have 15 fletching and 20 construction as well, this will increase the xp you get in these skills from wintertodt"
			},
			"verdict": "unknown"
		},
		{
			"critical": true,
			"depth": 1,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "unknown",
			"key": "skill:cooking\u003e=10",
			"kind": "skill",
			"label": "Cooking",
			"ref": "skill:cooking",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.1.153",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 154,
				"step_id": "1.1.153",
				"title": "Cook whatever low level raw food you got from wintertodt for 15 cooking on the fire"
			},
			"verdict": "unknown"
		},
		{
			"depth": 1,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "done",
			"key": "quest:cooks_assistant@finished",
			"kind": "quest",
			"label": "Cook's Assistant",
			"ref": "quest:cooks_assistant",
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
			"verdict": "met"
		},
		{
			"depth": 1,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "unknown",
			"key": "skill:agility\u003e=48",
			"kind": "skill",
			"label": "Agility",
			"ref": "skill:agility",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.2.16",
				"role": "completes",
				"section_id": "1.2",
				"section_pos": 16,
				"step_id": "1.2.16",
				"title": "Get 50 agility from barb fishing, should be at 74 fishing"
			},
			"verdict": "unknown"
		},
		{
			"boostable": true,
			"depth": 1,
			"during_quest": true,
			"for": [
				"quest:recipe_for_disaster@finished"
			],
			"group": "blocked",
			"have": 25,
			"key": "skill:magic\u003e=59",
			"kind": "skill",
			"label": "Magic",
			"ref": "skill:magic",
			"step": {
				"guide_id": "oziris-main",
				"ref": "1.4.45",
				"role": "completes",
				"section_id": "1.4",
				"section_pos": 47,
				"step_id": "1.4.45",
				"title": "Should get to 70+ mage by the time you're done with agility for now"
			},
			"verdict": "unmet"
		}
	],
	"presumed": 0,
	"requirements": true,
	"routes": [
		{
			"guide_id": "oziris-main",
			"ref": "1.1.178",
			"role": "starts",
			"section_id": "1.1",
			"section_pos": 180,
			"step_id": "1.1.178",
			"title": "Start RFD and watch the cutscene to unlock the bank chest"
		},
		{
			"goal": true,
			"guide_id": "oziris-main",
			"ref": "1.5.17",
			"role": "completes",
			"section_id": "1.5",
			"section_pos": 21,
			"step_id": "1.5.17",
			"title": "Finish RFD, buy B gloves"
		}
	],
	"total": 17,
	"unlocks": [],
	"unlocks_total": 0
} satisfies GraphResponse;
