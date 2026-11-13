// A real answer of GET /api/workers, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { WorkersView } from '../microbot';

export const fixture = {
	"attached": [
		{
			"account": "acc_contract",
			"connected": true,
			"host": "zaros",
			"id": "9c106a9f5bdb2f55",
			"identity": "dev",
			"kind": "attached",
			"since": "2026-11-12T20:19:16.173765202-03:00"
		}
	],
	"enabled": true,
	"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
	"version": "test",
	"workers": [
		{
			"account": "acc_worker_a",
			"agent": {
				"account": "acc_worker_a",
				"bridge": "1.11.0",
				"connected": true,
				"host": "chronicle-worker-acc-worker-a-7d9f-x2k4q",
				"id": "25f66382f43ce27e",
				"identity": "dev",
				"kind": "managed",
				"since": "2026-11-12T20:19:16.173764292-03:00",
				"worker": "chronicle-worker-acc-worker-a-7d9f-x2k4q"
			},
			"desired": "running",
			"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
			"outdated": false,
			"pod": {
				"deployment": "chronicle-worker-acc-worker-a",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
				"pod": "chronicle-worker-acc-worker-a-7d9f-x2k4q",
				"restarts": 1,
				"seen": "2026-11-12T23:19:16Z",
				"state": "running",
				"version": "v1.2.3"
			},
			"rsn": "Worker A",
			"session": {
				"account_id": "acc_worker_a",
				"character": "Worker A",
				"key_id": "4bb06f8e",
				"last_login_at": "2026-11-12T20:19:16.149828-03:00",
				"last_login_result": "ok",
				"pushed_at": "2026-11-12T20:19:16.14683-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-12T20:19:16.14839-03:00",
			"world": 330
		},
		{
			"account": "acc_worker_b",
			"agent": null,
			"desired": "stopped",
			"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000",
			"last_error": "the session was refused",
			"outdated": true,
			"pod": {
				"deployment": "chronicle-worker-acc-worker-b",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000",
				"reason": "stopped",
				"seen": "2026-11-12T23:19:16Z",
				"state": "terminating"
			},
			"rsn": "Worker B",
			"session": {
				"account_id": "acc_worker_b",
				"character": "Worker B",
				"key_id": "4bb06f8e",
				"pushed_at": "2026-11-12T20:19:16.150574-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-12T20:19:16.151676-03:00",
			"stopped_at": "2026-11-12T20:19:16.152777-03:00"
		},
		{
			"account": "acc_worker_c",
			"agent": null,
			"desired": "stopped",
			"outdated": false,
			"pod": null,
			"rsn": "Worker C",
			"session": null,
			"stopped_at": "2026-11-12T20:19:16.155536-03:00"
		}
	]
} satisfies WorkersView;
