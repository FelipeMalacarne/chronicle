// A real answer of GET /api/levels, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { FleetLevels } from '../../progress/types';

export const fixture = {
	"events": [
		{
			"account": "acc_contract",
			"at": "2026-11-13T04:11:35.385817Z",
			"level": 99,
			"skill": "attack"
		}
	],
	"since": "2026-11-12T04:11:35.454659311Z",
	"total": 461
} satisfies FleetLevels;
