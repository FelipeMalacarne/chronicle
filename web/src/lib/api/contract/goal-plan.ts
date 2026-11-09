// A real answer of GET /api/goals/goal_41ffebd7d2ab2439169ba601/plan, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { GoalPlanResponse } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"goal": {
		"created_at": "2026-11-05T21:36:56.104479Z",
		"id": "goal_41ffebd7d2ab2439169ba601",
		"kind": "skill",
		"label": "Magic",
		"name": "Magic",
		"objective_key": "skill:magic\u003e=43",
		"priority": 100,
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
				"total": 564
			}
		],
		"status": "active",
		"verdict": "unmet"
	},
	"plan": {
		"bottlenecks": [
			{
				"key": "skill:magic\u003e=43",
				"level": 25,
				"met": false,
				"skill": "magic",
				"step": {
					"guide_id": "oziris-main",
					"ref": "1.1.219",
					"role": "completes",
					"section_id": "1.1",
					"section_pos": 222,
					"step_id": "1.1.219",
					"title": "Finish The Grand tree"
				},
				"target": 43
			}
		],
		"goal": "skill:magic\u003e=43",
		"met": 0,
		"nodes": [
			{
				"depth": 0,
				"group": "blocked",
				"key": "skill:magic\u003e=43",
				"label": "Magic",
				"ref": "skill:magic",
				"step": {
					"guide_id": "oziris-main",
					"ref": "1.1.219",
					"role": "completes",
					"section_id": "1.1",
					"section_pos": 222,
					"step_id": "1.1.219",
					"title": "Finish The Grand tree"
				},
				"verdict": "unmet"
			}
		],
		"presumed": 0,
		"requirements": false,
		"routes": [
			{
				"done": true,
				"guide_id": "oziris-main",
				"ref": "1.1.126",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 126,
				"step_id": "1.1.126",
				"title": "Safespot barbarians in the bar for 13 magic"
			},
			{
				"guide_id": "oziris-main",
				"ref": "1.1.165",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 166,
				"step_id": "1.1.165",
				"title": "Train 42 magic at Moss giants near fishing guild"
			},
			{
				"goal": true,
				"guide_id": "oziris-main",
				"ref": "1.1.219",
				"role": "completes",
				"section_id": "1.1",
				"section_pos": 222,
				"step_id": "1.1.219",
				"title": "Finish The Grand tree"
			}
		],
		"total": 0
	}
} satisfies GoalPlanResponse;
