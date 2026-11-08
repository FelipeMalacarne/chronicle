import type {
	AccountMode,
	AccountState,
	AccountView,
	Board,
	ContentSection,
	EventsView,
	Goal,
	GoalPlan,
	Graph,
	GuideInfo,
	MarkAction,
	MarkRestore,
	ProgressSummary,
	Readiness,
	SetupLookup,
	SyncRun,
	WorldEntity,
	WorldEntityDetail
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

/**
 * Cabeçalho anti-CSRF (docs/04, seção 2.5): em AUTH_MODE=oidc toda escrita autenticada pelo
 * cookie de sessão precisa dele; outro site não consegue mandá-lo sem preflight de CORS.
 */
const CSRF_HEADER = 'X-Ironman-CSRF';

const LOGIN_AT = 'ironman.login-redirect-at';
/** Um 401 logo depois de voltar do login é cookie recusado (ex.: Secure em http): não repete. */
const LOGIN_LOOP_MS = 15_000;

/** Sessão ausente ou vencida (só em AUTH_MODE=oidc): vai para o login e volta para esta tela. */
function redirectToLogin() {
	if (typeof window === 'undefined') return;
	try {
		const last = Number(sessionStorage.getItem(LOGIN_AT) ?? 0);
		if (Date.now() - last < LOGIN_LOOP_MS) return;
		sessionStorage.setItem(LOGIN_AT, String(Date.now()));
	} catch {
		/* sem storage: segue para o login */
	}
	const here = window.location.pathname + window.location.search + window.location.hash;
	window.location.assign(`/auth/login?return=${encodeURIComponent(here)}`);
}

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
			const method = (init?.method ?? 'GET').toUpperCase();
			res = await fetch(url, {
				...init,
				headers: {
					accept: 'application/json',
					...(method === 'GET' || method === 'HEAD' ? {} : { [CSRF_HEADER]: '1' }),
					...(init?.headers as Record<string, string>)
				}
			});
		} catch (e) {
			throw new ApiError(e instanceof Error ? e.message : 'falha de rede', 0, url);
		}
		if (res.status === 401) redirectToLogin();
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

/**
 * Progresso sobre objetivos (docs/04, seção 2.5). Rotas por conta: em AUTH_MODE=dev sem login; em
 * oidc com a sessão, só para as contas do usuário (401 leva ao login).
 */
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
	graph: (account: string, focus: string, depth: number) =>
		request<Graph>(`/graph?account=${enc(account)}&focus=${enc(focus)}&depth=${depth}`),
	removeGoal: (account: string, id: string) =>
		request<null>(`/goals/${enc(id)}?account=${enc(account)}`, json('DELETE')),
	world: (account: string) =>
		request<{ entities: WorldEntity[] }>(`/world?account=${enc(account)}`),
	accountState: (account: string) =>
		request<AccountState>(`/account/state?account=${enc(account)}`),
	entity: (account: string, ref: string) =>
		request<WorldEntityDetail>(`/world/entity?account=${enc(account)}&ref=${enc(ref)}`),
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
