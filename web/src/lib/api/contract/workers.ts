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
			"id": "cbab50bcbd58c9ac",
			"identity": "dev",
			"kind": "attached",
			"since": "2026-11-13T01:13:10.364281246-03:00"
		}
	],
	"capacity": {
		"cpu_free_cores": 1.48,
		"cpu_hard_cores": 2.2,
		"fits": 2,
		"limited_by": "requests.cpu",
		"memory_free_bytes": 2348810240,
		"memory_hard_bytes": 3489660928,
		"seen": "2026-11-13T04:13:10Z",
		"worker_cpu_cores": 0.7,
		"worker_memory_bytes": 1073741824
	},
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
				"id": "29858af5677915d2",
				"identity": "dev",
				"kind": "managed",
				"since": "2026-11-13T01:13:10.364280606-03:00",
				"worker": "chronicle-worker-acc-worker-a-7d9f-x2k4q"
			},
			"auto_login": false,
			"desired": "running",
			"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
			"outdated": false,
			"pod": {
				"deployment": "chronicle-worker-acc-worker-a",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
				"pod": "chronicle-worker-acc-worker-a-7d9f-x2k4q",
				"restarts": 1,
				"seen": "2026-11-13T04:13:10Z",
				"state": "running",
				"version": "v1.2.3"
			},
			"rsn": "Worker A",
			"session": {
				"account_id": "acc_worker_a",
				"character": "Worker A",
				"key_id": "4bb06f8e",
				"last_login_at": "2026-11-13T01:13:10.335543-03:00",
				"last_login_result": "ok",
				"pushed_at": "2026-11-13T01:13:10.332246-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-13T01:13:10.33398-03:00",
			"usage": {
				"cpu_cores": 0.404,
				"measured_at": "2026-11-13T04:12:55Z",
				"memory_bytes": 877350912
			},
			"world": 330
		},
		{
			"account": "acc_worker_b",
			"agent": null,
			"auto_login": false,
			"desired": "stopped",
			"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000",
			"last_error": "the session was refused",
			"outdated": true,
			"pod": {
				"deployment": "chronicle-worker-acc-worker-b",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000",
				"reason": "stopped",
				"seen": "2026-11-13T04:13:10Z",
				"state": "terminating"
			},
			"rsn": "Worker B",
			"session": {
				"account_id": "acc_worker_b",
				"character": "Worker B",
				"key_id": "4bb06f8e",
				"last_login_at": "2026-11-13T01:13:10.337567-03:00",
				"last_login_result": "Invalid credentials",
				"login_failed": true,
				"pushed_at": "2026-11-13T01:13:10.336342-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-13T01:13:10.338328-03:00",
			"stopped_at": "2026-11-13T01:13:10.339527-03:00"
		},
		{
			"account": "acc_worker_c",
			"agent": null,
			"auto_login": false,
			"desired": "stopped",
			"outdated": false,
			"pod": null,
			"rsn": "Worker C",
			"session": null,
			"stopped_at": "2026-11-13T01:13:10.342518-03:00"
		}
	]
} satisfies WorkersView;
