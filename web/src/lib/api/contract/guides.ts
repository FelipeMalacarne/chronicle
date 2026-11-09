// A real answer of GET /api/guides, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { GuideInfo } from '../../progress/types';

export const fixture = {
	"guides": [
		{
			"id": "bruhsailer-main",
			"sections": [
				{
					"guide": "bruhsailer-main",
					"id": "bruh-1.1",
					"kind": "main",
					"slug": "tutorial-island-up-to-and-including-wintertodt",
					"steps": 18,
					"title": "Tutorial island up to and including Wintertodt",
					"track": false
				}
			],
			"segments": 102,
			"slug": "bruhsailer-main",
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
	]
} satisfies { guides: GuideInfo[] };
