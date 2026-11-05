import type {
	AccountMode,
	AccountView,
	Board,
	ContentSection,
	EventsView,
	Goal,
	GoalPlan,
	GuideInfo,
	MarkAction,
	MarkRestore,
	ProgressSummary,
	Readiness,
	SetupLookup,
	SyncRun,
	WorldEntity
} from '../progress/types';
import { untrack } from 'svelte';
import { sync, markOk, markFail } from './sync.svelte';

export { sync, type SyncState } from './sync.svelte';

/** Erro de API com status HTTP (0 = rede/offline). */
export class ApiError extends Error {
	readonly status: number;
	readonly url: string;
	constructor(message: string, status: number, url: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.url = url;
	}
	get offline(): boolean {
		return this.status === 0 || this.status === 502 || this.status === 503 || this.status === 504;
	}
}

const BASE = '/api';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const url = `${BASE}${path}`;
	// untrack: quem chama pode estar dentro de um $effect; ler sync aqui criaria dependência e loop.
	untrack(() => {
		sync.inflight += 1;
		if (sync.state === 'idle') sync.state = 'loading';
	});
	try {
		let res: Response;
		try {
			res = await fetch(url, {
				...init,
				headers: { accept: 'application/json', ...(init?.headers as Record<string, string>) }
			});
		} catch (e) {
			throw new ApiError(e instanceof Error ? e.message : 'falha de rede', 0, url);
		}
		if (!res.ok) {
			let detail = res.statusText;
			try {
				const body = (await res.json()) as { error?: string; message?: string };
				detail = body.error ?? body.message ?? detail;
			} catch {
				/* corpo não-JSON */
			}
			throw new ApiError(detail || `HTTP ${res.status}`, res.status, url);
		}
		const data = (res.status === 204 ? null : await res.json()) as T;
		untrack(markOk);
		return data;
	} catch (e) {
		untrack(() => markFail(e));
		throw e;
	} finally {
		untrack(() => {
			sync.inflight -= 1;
		});
	}
}

const enc = encodeURIComponent;
const json = (method: string, body?: unknown): RequestInit => ({
	method,
	headers: { 'content-type': 'application/json' },
	body: body === undefined ? undefined : JSON.stringify(body)
});

/** Progresso sobre objetivos (docs/04, seção 2.5). Rotas por conta: só em AUTH_MODE=dev até existir sessão. */
export const progressApi = {
	guides: () => request<{ guides: GuideInfo[] }>('/guides'),
	section: (guide: string, section: string) =>
		request<ContentSection>(`/guides/${enc(guide)}/sections/${enc(section)}`),
	summary: (account: string) => request<ProgressSummary>(`/progress?account=${enc(account)}`),
	board: (guide: string, account: string) =>
		request<Board>(`/guides/${enc(guide)}/board?account=${enc(account)}`),
	events: (guide: string, account: string, limit = 20) =>
		request<EventsView>(`/guides/${enc(guide)}/events?account=${enc(account)}&limit=${limit}`),
	readiness: (guide: string, account: string, step?: string) =>
		request<Readiness>(
			`/guides/${enc(guide)}/readiness?account=${enc(account)}${step ? `&step=${enc(step)}` : ''}`
		),
	mark: (guide: string, step: string, account: string, kind: MarkAction, note?: string) =>
		request<Board>(
			`/guides/${enc(guide)}/steps/${enc(step)}/mark?account=${enc(account)}`,
			json('PATCH', note === undefined ? { kind } : { kind, note })
		),
	markBefore: (guide: string, account: string, step: string) =>
		request<Board>(
			`/guides/${enc(guide)}/marks?account=${enc(account)}`,
			json('POST', { before_step_id: step })
		),
	restoreMarks: (guide: string, account: string, restore: MarkRestore[]) =>
		request<Board>(
			`/guides/${enc(guide)}/marks?account=${enc(account)}`,
			json('POST', { restore })
		),
	confirmImplied: (guide: string, account: string) =>
		request<Board>(`/guides/${enc(guide)}/implied/confirm?account=${enc(account)}`, json('POST')),
	pin: (guide: string, account: string, step: string | null) =>
		request<Board>(
			`/guides/${enc(guide)}/cursor?account=${enc(account)}`,
			json('PUT', { step_id: step })
		),
	goals: (account: string) => request<{ goals: Goal[] }>(`/goals?account=${enc(account)}`),
	addGoal: (account: string, key: string) =>
		request<Goal>(`/goals?account=${enc(account)}`, json('POST', { objective_key: key })),
	goalPlan: (account: string, id: string) =>
		request<{ goal: Goal; plan: GoalPlan }>(`/goals/${enc(id)}/plan?account=${enc(account)}`),
	removeGoal: (account: string, id: string) =>
		request<null>(`/goals/${enc(id)}?account=${enc(account)}`, json('DELETE')),
	world: (account: string) =>
		request<{ entities: WorldEntity[] }>(`/world?account=${enc(account)}`),
	sync: (account: string) => request<SyncRun>(`/sync?account=${enc(account)}`, json('POST')),
	syncRuns: (account: string, limit = 5) =>
		request<{ runs: SyncRun[] }>(`/sync/runs?account=${enc(account)}&limit=${limit}`),
	setupLookup: (rsn: string) => request<SetupLookup>(`/setup/lookup?rsn=${enc(rsn)}`),
	createAccount: (input: { rsn: string; mode: AccountMode; guide_id: string; layer: string }) =>
		request<AccountView>('/accounts', json('POST', input)),
	setActiveGuide: (account: string, guide: string) =>
		request<ProgressSummary>(
			`/account/guide?account=${enc(account)}`,
			json('PUT', { guide_id: guide })
		)
};
