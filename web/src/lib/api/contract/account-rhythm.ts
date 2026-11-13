// A real answer of GET /api/account/rhythm, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { AccountRhythm } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"gp": {
		"today": 0,
		"value": 25000
	},
	"session": {
		"started_at": "2026-11-13T03:51:27Z",
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
		"last_gain_at": "2026-11-13T03:52:27.894447Z",
		"level": 99,
		"skill": "defence",
		"xp": 200000000,
		"xp_per_hour": 0
	}
} satisfies AccountRhythm;
