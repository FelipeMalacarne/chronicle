// A real answer of GET /api/rhythm, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { FleetRhythm } from '../../progress/types';

export const fixture = {
	"accounts": [
		{
			"account": "acc_contract",
			"gp": {
				"today": 0,
				"value": 25000
			},
			"session": {
				"gained": [
					{
						"item": 1511,
						"name": "Logs",
						"qty": 5
					}
				],
				"running_out": [
					{
						"in_s": 1352,
						"item": 314,
						"left": 60,
						"name": "Feather",
						"used": 40
					}
				],
				"started_at": "2026-11-13T12:14:10Z",
				"xp": 314811223
			},
			"today": {
				"levels": 458,
				"since": "2026-11-13T00:00:00Z",
				"skills": [
					{
						"level": 99,
						"levels": 89,
						"skill": "defence",
						"xp": 199998846
					}
				],
				"xp": 314811223
			},
			"training": {
				"last_gain_at": "2026-11-13T12:29:11.501499Z",
				"level": 99,
				"skill": "defence",
				"xp": 200000000,
				"xp_per_hour": 798662948
			}
		},
		{
			"account": "acc_worker_a",
			"gp": null,
			"session": null,
			"today": {
				"levels": 0,
				"since": "2026-11-13T00:00:00Z",
				"skills": [],
				"xp": 0
			},
			"training": null
		},
		{
			"account": "acc_levels",
			"gp": {
				"today": 0,
				"value": 0
			},
			"session": null,
			"today": {
				"levels": 3,
				"since": "2026-11-13T00:00:00Z",
				"skills": [
					{
						"level": 76,
						"levels": 1,
						"skill": "fishing",
						"xp": 124862
					}
				],
				"xp": 130747
			},
			"training": null
		}
	],
	"since": "2026-11-13T00:00:00Z",
	"totals": {
		"gaining": 2,
		"gp": 25000,
		"gp_today": 0,
		"levels_today": 461,
		"xp_today": 314941970
	}
} satisfies FleetRhythm;
