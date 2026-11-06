// A real answer of GET /api/guides/oziris-main/events, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { EventsView } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"guide": "oziris-main",
	"objectives": [
		{
			"at": "2026-11-05T21:36:56.086749Z",
			"key": "skill:woodcutting\u003e=80",
			"source": "plugin",
			"to": "unmet"
		}
	],
	"steps": [
		{
			"at": "2026-11-05T21:36:56.211821Z",
			"cause": "user",
			"from": "pending",
			"step_id": "1.1.72",
			"to": "manual_done"
		}
	]
} satisfies EventsView;
