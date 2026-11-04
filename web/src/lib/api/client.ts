import type { Health, Now, Section, SectionSummary, Step } from './types';
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
				headers: { accept: 'application/json', ...init?.headers }
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
		const data = (await res.json()) as T;
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

export const api = {
	health: () => request<Health>('/health'),
	guide: () => request<SectionSummary[]>('/guide'),
	section: (section: string) => request<Section>(`/guide/${encodeURIComponent(section)}`),
	step: (id: string) => request<Step>(`/steps/${encodeURIComponent(id)}`),
	now: () => request<Now>('/now')
};

/** "sincronizado há 2 min" / "há 3 h" / "agora". */
export function relativeTime(at: number | null, now: number = Date.now()): string {
	if (!at) return 'nunca';
	const s = Math.max(0, Math.round((now - at) / 1000));
	if (s < 10) return 'agora';
	if (s < 60) return `há ${s} s`;
	const m = Math.round(s / 60);
	if (m < 60) return `há ${m} min`;
	const h = Math.round(m / 60);
	if (h < 48) return `há ${h} h`;
	return `há ${Math.round(h / 24)} d`;
}
