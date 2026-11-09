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
	/** Quem o servidor autenticou: "dev" (loopback) ou o subject do machine user do Zitadel. */
	identity?: string;
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

export type MicrobotAction = 'client/start' | 'client/stop' | 'login' | 'logout' | 'world';

/** O personagem com que o cliente de dev sobe (mb-account) e os que o launcher conhece. */
export interface ClientAccount {
	current: string;
	characters: string[];
	error?: string;
}

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

/** O formulário de config de um plugin, exportado pelo plugin Chronicle Link no cliente de dev (doc 08). */
export interface PluginSchema {
	className: string;
	name: string;
	group: string;
	sections: {
		key: string;
		name: string;
		position: number;
		description?: string;
		closedByDefault?: boolean;
	}[];
	items: SchemaItem[];
}

export interface SchemaItem {
	key: string;
	name: string;
	description?: string;
	position: number;
	section?: string;
	hidden?: boolean;
	warning?: string;
	type: 'boolean' | 'int' | 'double' | 'string' | 'enum' | 'color' | 'keybind' | 'other';
	values?: { value: string; label: string }[];
	min?: number;
	max?: number;
	units?: string;
}

export interface ConfigValues {
	group: string;
	values: Record<string, string | null>;
}

export interface Pos {
	x: number;
	y: number;
	plane: number;
}

/** Item do inventário (GET /inventory traz as ações) ou do banco. */
export interface GameItem {
	id: number;
	name: string;
	quantity: number;
	slot?: number;
	actions?: string[];
	noted?: boolean;
	stackable?: boolean;
}

export interface NearNpc {
	id: number;
	index?: number;
	name: string;
	combatLevel?: number;
	distance?: number;
	interacting?: boolean;
	position?: Pos;
}

export interface NearObject {
	id: number;
	name: string;
	type?: string;
	reachable?: boolean;
	position?: Pos;
}

export interface GroundItem {
	id: number;
	name: string;
	quantity: number;
	lootable?: boolean;
	position?: Pos;
}

export interface Dialogue {
	inDialogue: boolean;
	hasContinue?: boolean;
	hasOptions?: boolean;
	dialogueText?: string;
	question?: string;
	options?: string[];
}

/** GET /observe do Agent Server: o /state mais inventário, equipamento, diálogo, chat do jogo
 * (nunca o chat de jogadores), erros recentes, heartbeats e o que está por perto. */
export interface Observe extends GameState {
	inventory?: { count: number; capacity: number; freeSlots: number; items: GameItem[] };
	equipment?: { slot: string; id: number; name: string; quantity: number }[];
	dialogue?: Dialogue;
	npcs?: NearNpc[];
	objects?: NearObject[];
	chat?: { cursor: number; messages: { type: string; message: string; seq: number; at: number }[] };
	errors?: { cursor: number; recent: { logger?: string; message?: string; at?: number }[] };
	scripts?: { scriptClassName: string; loopCount?: number; stalledMs?: number; status?: string }[];
}

export interface StateMachine {
	name: string;
	currentState: string | null;
	loopCount?: number;
	transitionCount?: number;
	msInCurrentState?: number;
}

/** As ações no jogo da lista fechada do agent (internal/agent/game.go). */
export type GameCommand =
	| 'walk'
	| 'inventory_use'
	| 'inventory_drop'
	| 'bank_open'
	| 'bank_close'
	| 'bank_deposit'
	| 'bank_withdraw'
	| 'npc_interact'
	| 'object_interact'
	| 'ground_pickup'
	| 'dialogue_continue'
	| 'dialogue_select'
	| 'equipment_use'
	| 'prayer_toggle'
	| 'spell_cast'
	| 'bank_deposit_x'
	| 'item_use_on';

/** No que um feitiço é lançado: `unsupported` é um alvo que a ponte ainda não alcança (enchant). */
export type SpellTarget = 'item' | 'npc' | 'none' | 'unsupported';

/** O que a ponte de controle (plugin Chronicle Link) acrescenta: os menus reais, as orações e o livro de feitiços. */
export interface BridgeView {
	entities?: {
		npcs: {
			id: number;
			index?: number;
			name: string;
			combatLevel?: number;
			actions: string[];
			position?: Pos;
		}[];
		objects: { id: number; name: string; actions: string[]; position?: Pos }[];
	};
	equipment?: {
		items: { slot: string; id: number; name: string; quantity: number; actions: string[] }[];
	};
	prayers?: {
		level: number;
		points: number;
		prayers: { key: string; name: string; level: number; unlocked: boolean; active: boolean }[];
	};
	spells?: {
		spellbook: string | null;
		spells: {
			key: string;
			name: string;
			level: number;
			castable: boolean;
			/** No que o feitiço é lançado (Chronicle Link 1.5+); antes só `onItem`. */
			target?: SpellTarget;
			onItem: boolean;
		}[];
	};
}

/** GET /api/microbot/game: uma leitura de tudo o que o cockpit mostra do jogo. */
export interface GameView {
	observe?: Observe;
	inventory?: { count: number; capacity: number; items: GameItem[] };
	skills?: {
		totalLevel?: number;
		skills: { name: string; level: number; boostedLevel: number; xp: number }[];
	};
	bank?: { open: boolean; count?: number; items?: GameItem[] };
	ground?: { items: GroundItem[] };
	machines?: { count: number; scripts: StateMachine[] };
	bridge?: BridgeView;
	errors?: Record<string, string>;
}

/** Um relógio do Time Tracking do RuneLite (doc 08, fase 4): uma aba de farming, as birdhouses ou o contrato. */
export interface Timer {
	key: string;
	label: string;
	state: 'COMPLETED' | 'IN_PROGRESS' | 'OCCUPIED' | 'EMPTY' | 'UNKNOWN';
	/** Quando fica (ou ficou) pronto, em segundos desde a época; só crescendo ou pronto. */
	readyAt?: number;
}

/** GET /api/microbot/timers: os relógios pela ponte (Chronicle Link 1.6+). */
export interface Timers {
	/** O relógio da máquina do jogo na leitura, em segundos. */
	now: number;
	/** false: o plugin Time Tracking está desligado no cliente. */
	enabled: boolean;
	timers: Timer[];
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
	game: () => call<GameView>('/game'),
	timers: () => call<Timers>('/timers'),
	/** O fim do log do cliente, com o que parece segredo mascarado pelo agent. */
	logs: (lines = 200) => call<{ file: string; lines: string[] }>(`/client/logs?lines=${lines}`),
	account: () => call<ClientAccount>('/account'),
	accountUse: (index: number) =>
		call<{ ok: boolean; result: unknown }>('/account', {
			method: 'POST',
			body: JSON.stringify({ index })
		}),
	/** Uma ação no jogo (doc 08): validada no servidor e no agent, e auditada. */
	gameAction: (command: GameCommand, body?: Record<string, unknown>) =>
		call<{ ok: boolean; result: unknown }>(`/game/${command}`, {
			method: 'POST',
			body: JSON.stringify(body ?? {})
		}),
	hub: (q: string) => call<HubList>(`/hub?q=${encodeURIComponent(q)}`),
	scriptDetail: (className: string) =>
		call<ScriptDetail>(`/scripts/detail?className=${encodeURIComponent(className)}`),
	configSchema: (className: string) =>
		call<PluginSchema>(`/config/schema?className=${encodeURIComponent(className)}`),
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
