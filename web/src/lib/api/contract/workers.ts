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
			"id": "4bc1f5e089aa7610",
			"identity": "dev",
			"kind": "attached",
			"since": "2026-11-26T11:31:51.997177084-03:00"
		}
	],
	"capacity": {
		"cpu_free_cores": 1.48,
		"cpu_hard_cores": 2.2,
		"fits": 2,
		"limited_by": "requests.cpu",
		"memory_free_bytes": 2348810240,
		"memory_hard_bytes": 3489660928,
		"seen": "2026-11-26T14:31:51Z",
		"worker_cpu_cores": 0.7,
		"worker_memory_bytes": 1073741824
	},
	"cluster": true,
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
				"id": "9a6f5e2b693efbc7",
				"identity": "dev",
				"kind": "managed",
				"since": "2026-11-26T11:31:51.997147144-03:00",
				"worker": "chronicle-worker-acc-worker-a-7d9f-x2k4q"
			},
			"auto_login": false,
			"desired": "running",
			"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
			"outdated": false,
			"placement": "cluster",
			"pod": {
				"deployment": "chronicle-worker-acc-worker-a",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678",
				"pod": "chronicle-worker-acc-worker-a-7d9f-x2k4q",
				"restarts": 1,
				"seen": "2026-11-26T14:31:51Z",
				"state": "running",
				"version": "v1.2.3"
			},
			"rsn": "Worker A",
			"session": {
				"account_id": "acc_worker_a",
				"character": "Worker A",
				"key_id": "4bb06f8e",
				"last_login_at": "2026-11-26T11:31:51.963299-03:00",
				"last_login_result": "ok",
				"pushed_at": "2026-11-26T11:31:51.96032-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-26T11:31:51.96183-03:00",
			"usage": {
				"cpu_cores": 0.404,
				"measured_at": "2026-11-26T14:31:36Z",
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
			"placement": "cluster",
			"pod": {
				"deployment": "chronicle-worker-acc-worker-b",
				"image": "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000",
				"reason": "stopped",
				"seen": "2026-11-26T14:31:51Z",
				"state": "terminating"
			},
			"rsn": "Worker B",
			"session": {
				"account_id": "acc_worker_b",
				"character": "Worker B",
				"key_id": "4bb06f8e",
				"last_login_at": "2026-11-26T11:31:51.965203-03:00",
				"last_login_result": "Invalid credentials",
				"login_failed": true,
				"pushed_at": "2026-11-26T11:31:51.964149-03:00",
				"pushed_by": "dev"
			},
			"started_at": "2026-11-26T11:31:51.965934-03:00",
			"stopped_at": "2026-11-26T11:31:51.967066-03:00"
		},
		{
			"account": "acc_worker_c",
			"agent": null,
			"auto_login": false,
			"desired": "stopped",
			"outdated": false,
			"placement": "zaros",
			"pod": null,
			"rsn": "Worker C",
			"session": null,
			"stopped_at": "2026-11-26T11:31:51.970059-03:00"
		}
	]
} satisfies WorkersView;
