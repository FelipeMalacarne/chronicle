// A real answer of GET /api/progress, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { ProgressSummary } from '../../progress/types';

export const fixture = {
	"account": "acc_contract",
	"active_guide": "oziris-main",
	"routes": [
		{
			"active": false,
			"counts": {
				"auto_done": 0,
				"blocked": 14,
				"done": 0,
				"implied": 0,
				"manual_done": 0,
				"pinned_open": 0,
				"remaining": 226,
				"skipped": 0,
				"total": 226
			},
			"coverage": {
				"checkbox": 28,
				"checkbox_done": 0,
				"checkbox_skipped": 0,
				"met": 5,
				"objectives": 777
			},
			"cursor": {
				"cause": "auto",
				"step_id": "bruh-1.1-e8d3227fd203"
			},
			"cursor_ref": "CH1.1",
			"cursor_section": "bruh-1.1",
			"guide": "bruhsailer-main",
			"sections": [
				{
					"counts": {
						"auto_done": 0,
						"blocked": 0,
						"done": 0,
						"implied": 0,
						"manual_done": 0,
						"pinned_open": 0,
						"remaining": 18,
						"skipped": 0,
						"total": 18
					},
					"id": "bruh-1.1",
					"title": "Tutorial island up to and including Wintertodt",
					"track": false
				}
			],
			"segments": 102,
			"source": {
				"author": "So Iron BRUH and ParasailerOSRS",
				"imported_at": "2026-11-04",
				"license": "No explicit licence declared upstream; personal, self-hosted use only. Keep attribution; do not redistribute publicly or use commercially without the authors' permission.",
				"origin": "other",
				"original_id": "b697c0a63f120a9e35233503f13c1aeaf224bb2d",
				"url": "https://github.com/Osrsper/BRUHsailer/tree/b697c0a63f120a9e35233503f13c1aeaf224bb2d"
			},
			"steps": 226,
			"title": "BRUHsailer Ironman Guide",
			"version": "2026.11.4"
		}
	],
	"sources": [
		{
			"facts": 13,
			"last_observed_at": "2026-11-05T21:35:55Z",
			"source": "plugin"
		}
	]
} satisfies ProgressSummary;
