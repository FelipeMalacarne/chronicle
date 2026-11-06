// A real answer of GET /api/guides/oziris-main/readiness, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { Readiness } from '../../progress/types';

export const fixture = {
	"ahead": {
		"from_step_id": "1.1.2",
		"groups": [
			{
				"items": [
					{
						"carried": 0,
						"item_id": 1935,
						"name": "Jug",
						"owned": {
							"containers": {
								"bank": {
									"complete": true,
									"observed": true,
									"observed_at": "2026-11-05T21:35:55Z",
									"qty": 0,
									"state": "live"
								},
								"equipment": {
									"complete": true,
									"observed": true,
									"observed_at": "2026-11-05T21:35:55Z",
									"qty": 0,
									"state": "live"
								},
								"inventory": {
									"complete": true,
									"observed": true,
									"observed_at": "2026-11-05T21:35:55Z",
									"qty": 0,
									"state": "live"
								}
							},
							"held": 0,
							"required": 1,
							"verdict": "unmet"
						},
						"qty_stated": true,
						"required": 1,
						"slot": "inventory",
						"source_text": "a jug",
						"variant": "exact",
						"verdict": "unmet"
					}
				],
				"offset": 6,
				"step_id": "1.1.8",
				"text": "Fill a jug with water"
			}
		],
		"obtained": [],
		"steps": 7,
		"until_step_id": "1.1.9"
	},
	"current": {
		"bank_actions": [],
		"first_step_id": "1.1.1",
		"freshness": {
			"bank": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			},
			"equipment": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			},
			"inventory": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			}
		},
		"guide_id": "oziris-main",
		"items": [],
		"kind": "sequence",
		"loadout_status": "unknown",
		"reason": "the guide does not specify a loadout for this segment",
		"section_id": "1.1",
		"segment_id": "seg-2e7358630635",
		"status": "no_loadout",
		"title": "Sequence from 1.1.1"
	},
	"guide": "oziris-main",
	"next": {
		"bank_actions": [
			{
				"position": "end",
				"source": {
					"confidence": "high",
					"evidence": "bank everything",
					"method": "llm",
					"step_ids": [
						"1.1.10"
					]
				},
				"text": "bank everything",
				"type": "deposit_all"
			}
		],
		"first_step_id": "1.1.10",
		"freshness": {
			"bank": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			},
			"equipment": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			},
			"inventory": {
				"complete": true,
				"live": true,
				"observed": true,
				"observed_at": "2026-11-05T21:35:55Z",
				"state": "live"
			}
		},
		"guide_id": "oziris-main",
		"items": [],
		"kind": "bank_leg",
		"loadout_status": "unknown",
		"reason": "the guide does not specify a loadout for this segment",
		"section_id": "1.1",
		"segment_id": "seg-2db1e200a25c",
		"status": "no_loadout",
		"title": "Bank leg from 1.1.10"
	},
	"step": "1.1.2"
} satisfies Readiness;
