// A real answer of GET /api/account/levels, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { AccountLevels } from '../../progress/types';

export const fixture = {
	"account": "acc_levels",
	"events": [
		{
			"account": "acc_levels",
			"at": "2026-11-13T03:11:35Z",
			"level": 39,
			"skill": "farming"
		}
	],
	"since": "2026-11-12T04:11:35.453497698Z",
	"skills": [
		{
			"from_level": 75,
			"skill": "fishing",
			"to_level": 76,
			"xp": 124862
		}
	],
	"total": 3
} satisfies AccountLevels;
