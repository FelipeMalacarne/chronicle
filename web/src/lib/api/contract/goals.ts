// A real answer of GET /api/goals, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { GoalsResponse } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"goals": [
		{
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
		}
	]
} satisfies GoalsResponse;
