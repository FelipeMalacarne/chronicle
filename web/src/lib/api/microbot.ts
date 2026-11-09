/**
 * Cliente das rotas /api/microbot (doc 08). Fica fora do `request` comum de propósito: o cockpit
 * consulta o estado a cada poucos segundos, e isso não deve mexer no indicador de sincronização
 * da conta. Com o módulo desligado (MICROBOT_CONTROL=off) as rotas não existem: `status` devolve
 * null.
 */

export interface AgentInfo {
	connected: boolean;
	since?: string;
	host?: string;
	version?: string;
}

/** O que o agent diz do cliente: o /state e o /login do Agent Server como vieram. */
export interface ClientStatus {
	running: boolean;
	headless: boolean;
	vnc: boolean;
	state?: GameState;
	login?: LoginState;
	error?: string;
}

export interface GameState {
	loggedIn?: boolean;
	gameState?: string;
	player?: {
		name?: string;
		combatLevel?: number;
		healthRatio?: number;
		healthScale?: number;
		animating?: boolean;
		moving?: boolean;
		interacting?: boolean;
		position?: { x: number; y: number; plane: number };
	};
	scriptsPaused?: boolean;
	instance?: { pid?: number; microbotVersion?: string; uptimeMs?: number };
}

export interface LoginState {
	loggedIn?: boolean;
	gameState?: string;
	currentWorld?: number;
	loginIndex?: number;
	loginError?: string;
	activeProfile?: { name?: string; isMember?: boolean; selectedWorld?: number };
}

export interface MicrobotStatus {
	enabled: boolean;
	/** AUTH_MODE=dev: o controle só responde a esta máquina. */
	dev: boolean;
	agent: AgentInfo;
	client?: ClientStatus;
	error?: string;
}

export interface AuditEntry {
	id: number;
	at: string;
	actor: string;
	origin: string;
	action: string;
	params: Record<string, unknown>;
	ok: boolean;
	error?: string;
}

export type MicrobotAction = 'client/start' | 'client/stop' | 'login' | 'logout';

/** Uma linha do GET /scripts do Agent Server: todo plugin do Microbot carregado no cliente. */
export interface ScriptEntry {
	name: string;
	className: string;
	active: boolean;
	enabled: boolean;
}

/** Uma entrada do Plugin Hub (GET /plugins). */
export interface HubPlugin {
	internalName: string;
	name: string;
	description?: string;
	latestVersion?: string;
	installedVersion?: string | null;
	loaded: boolean;
	health?: string;
	installable?: boolean;
}

export interface HubList {
	count: number;
	total: number;
	plugins: HubPlugin[];
}

export interface PluginsView {
	scripts: { count: number; scripts: ScriptEntry[] };
	installed: HubList;
}

/** Detalhe de um plugin: cada parte é o JSON do Agent Server; a que falhou vem em `errors`. */
export interface ScriptDetail {
	status?: { status?: string; active?: boolean; startedAt?: string; runtimeMs?: number };
	health?: unknown;
	results?: { count?: number; results?: Record<string, unknown>[] };
	machines?: unknown;
	errors?: Record<string, string>;
}

export interface ConfigValues {
	group: string;
	values: Record<string, string | null>;
}

/** Ações de plugins, scripts e configuração (doc 08, fase 3); todas vão para a auditoria. */
export type PluginAction =
	| { path: 'plugins/install' | 'plugins/remove'; body: { name: string; version?: string } }
	| { path: 'scripts/start' | 'scripts/stop'; body: { className: string } }
	| { path: 'scripts/pause' | 'scripts/resume'; body?: undefined }
	| { path: 'config'; body: { group: string; key: string; value: string | null } };

export class MicrobotError extends Error {
	constructor(
		message: string,
		readonly status: number
	) {
		super(message);
	}
}

const BASE = '/api/microbot';

async function call<T>(path: string, init?: RequestInit): Promise<T> {
	const method = (init?.method ?? 'GET').toUpperCase();
	let res: Response;
	try {
		res = await fetch(BASE + path, {
			...init,
			headers: {
				accept: 'application/json',
				...(method === 'GET' ? {} : { 'X-Ironman-CSRF': '1', 'content-type': 'application/json' })
			}
		});
	} catch (e) {
		throw new MicrobotError(e instanceof Error ? e.message : 'falha de rede', 0);
	}
	if (!res.ok) {
		let detail = res.statusText;
		try {
			detail = ((await res.json()) as { error?: string }).error ?? detail;
		} catch {
			/* corpo não-JSON */
		}
		throw new MicrobotError(detail || `HTTP ${res.status}`, res.status);
	}
	return (await res.json()) as T;
}

export const microbotApi = {
	/** null: o módulo está desligado neste servidor. */
	async status(): Promise<MicrobotStatus | null> {
		try {
			return await call<MicrobotStatus>('/status');
		} catch (e) {
			if (e instanceof MicrobotError && e.status === 404) return null;
			throw e;
		}
	},
	action: (action: MicrobotAction, body?: { world?: number }) =>
		call<{ ok: boolean; result: unknown }>(`/${action}`, {
			method: 'POST',
			body: JSON.stringify(body ?? {})
		}),
	audit: (limit = 30) => call<{ entries: AuditEntry[] }>(`/audit?limit=${limit}`),
	plugins: () => call<PluginsView>('/plugins'),
	hub: (q: string) => call<HubList>(`/hub?q=${encodeURIComponent(q)}`),
	scriptDetail: (className: string) =>
		call<ScriptDetail>(`/scripts/detail?className=${encodeURIComponent(className)}`),
	config: (group: string, keys: string[]) =>
		call<ConfigValues>(
			`/config?group=${encodeURIComponent(group)}&keys=${encodeURIComponent(keys.join(','))}`
		),
	plugin: (a: PluginAction) =>
		call<{ ok: boolean; result: unknown }>(`/${a.path}`, {
			method: 'POST',
			body: JSON.stringify(a.body ?? {})
		}),
	/** URL do screenshot; `t` força uma imagem nova a cada pedido. */
	screenshotURL: (t: number) => `${BASE}/screenshot?t=${t}`
};
