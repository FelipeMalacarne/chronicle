// A real answer of POST /api/sync, written by
// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:
// svelte-check verifies it against the web's type (docs/10 section 12).
import type { SyncRun } from '../../progress/types';

export const fixture = {
	"created_at": "2026-11-05T21:36:56.263027Z",
	"finished_at": "2026-11-05T21:36:56.263883Z",
	"http_status": 200,
	"id": "sync_073f1a3bb2d9c7bf9ce5c05d",
	"snapshot_id": "snap_39d455be1dff228955e8ffab",
	"source": "hiscores",
	"started_at": "2026-11-05T21:36:56.263027Z",
	"status": "ok",
	"steps_changed": 20,
	"trigger": "manual"
} satisfies SyncRun;
