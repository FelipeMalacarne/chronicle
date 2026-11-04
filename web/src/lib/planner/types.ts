/**
 * View-model do planner multi-guia. Não é o contrato HTTP: é o que as telas consomem,
 * produzido por `adapter.ts` (hoje com fixtures locais; depois, pela API).
 */

export type GuideId = string;

export interface GuideSummary {
	id: GuideId;
	/** Nome curto para seletores e faixas (ex.: "Oziris"). */
	label: string;
	title: string;
	/** Autor/atribuição exibida junto dos passos do guia. */
	author: string;
	steps: number;
}

export interface StepVM {
	/** Chave global `<guideId>::<localId>`: nunca colide entre guias. */
	key: string;
	guideId: GuideId;
	/** ID do passo dentro do guia (pode repetir entre guias). */
	localId: string;
	text: string;
	location?: string;
	/** Chave de marco compartilhado entre guias (ex.: `quest:dragon-slayer-i`). */
	milestone?: string;
	/** Nome legível do marco, igual nos dois guias. */
	milestoneLabel?: string;
	/** Passo em que a rota passa por um banco. */
	bank?: boolean;
}

export interface SectionVM {
	key: string;
	guideId: GuideId;
	id: string;
	title: string;
	steps: StepVM[];
}

export interface GuidePlan {
	guide: GuideSummary;
	sections: SectionVM[];
	/** Quantos passos (na ordem do guia) já foram feitos; fixture, nunca persistido. */
	doneCount: number;
}

export type BankFreshness = 'unavailable' | 'restored' | 'live';

export interface BankStatus {
	freshness: BankFreshness;
	/** Epoch ms da captura; ausente quando `unavailable`. */
	capturedAt: number | null;
}

export type Verdict = 'met' | 'unmet' | 'unknown';

export interface LoadoutItem {
	name: string;
	qty: number;
	/** Quantidade conhecida no banco; null quando o banco não é confiável/indisponível. */
	inBank: number | null;
	/** false quando o guia não diz a quantidade (qty vira 1). */
	qtyStated?: boolean;
	optional?: boolean;
	/** Se o item está no inventário/equipamento live agora (readiness), não no banco. */
	carried?: Verdict;
	/** Posse confirmada só por um banco restaurado. */
	ownedStale?: boolean;
}

/** Status do servidor (`internal/core/planning`). */
export type ReadinessStatus = 'ready' | 'listed_ready' | 'not_ready' | 'unknown' | 'no_loadout';

export interface LegReadiness {
	status: ReadinessStatus;
	/** Explica de onde vem (ou por que falta) a verificação. */
	detail?: string;
	/** Loadout do guia: `partial` lista só parte do setup. */
	loadout: 'known' | 'partial' | 'unknown';
}

export interface BankLeg {
	guideId: GuideId;
	/** Chave do passo em que o banco é visitado. */
	atStepKey: string;
	location: string;
	withdraw: LoadoutItem[];
	deposit: string[];
	/** Local do passo de banco (ex.: `1.1.75`). */
	atStepId: string;
	/** Quantos passos de campo até o próximo banco. */
	stepsUntilNextBank: number;
	bank: BankStatus;
	/** Ausente nas fixtures antigas; vem da rota de readiness do servidor. */
	readiness?: LegReadiness;
}

export interface Band {
	/** Marco compartilhado que abre a banda; null para a banda inicial. */
	milestone: string | null;
	leftAnchor: StepVM | null;
	rightAnchor: StepVM | null;
	left: StepVM[];
	right: StepVM[];
}

export type LoadState<T> =
	| { status: 'loading' }
	| { status: 'error'; message: string }
	| { status: 'empty' }
	| { status: 'ready'; data: T };

/** Cenário de mock selecionável via `?mock=` para exercitar os estados da UI. */
export type MockScenario = 'ready' | 'loading' | 'error' | 'empty';

/* ---------------------------------------------------------------------------------------------
 * Entidades: tudo que o jogador persegue (quest, level, item, diary, meta…) é um nó do grafo.
 * ------------------------------------------------------------------------------------------- */

export type EntityKind = 'quest' | 'skill' | 'item' | 'diary' | 'unlock' | 'boss' | 'goal';

/** Estado observado na conta. `available`/`blocked` são derivados do grafo, nunca gravados. */
export type EntityState = 'done' | 'in_progress' | 'available' | 'blocked';

export interface EntityVM {
	/** `<kind>:<slug>`, ex.: `quest:dragon-slayer-i`, `skill:agility:50`. */
	id: string;
	kind: EntityKind;
	name: string;
	/** Linha curta abaixo do nome (ex.: "Quest · Experienced · 2 QP"). */
	summary?: string;
	/** Para `skill`: level exigido e level atual. */
	level?: number;
	current?: number;
	/** Fato bruto da conta: concluído/em progresso. O resto é derivado. */
	observed: 'done' | 'in_progress' | 'none';
	/** De onde veio a evidência (hiscores, plugin, manual) e quando. */
	evidence?: { source: string; at: number };
	wiki?: string;
	tags?: string[];
}

/** Aresta "`from` é requisito de `to`" (from → to, no sentido do progresso). */
export interface EdgeVM {
	from: string;
	to: string;
}

export interface EntityGraph {
	entities: Record<string, EntityVM>;
	edges: EdgeVM[];
}

/** Onde uma entidade aparece em cada rota (a "comparação" entre guias vive aqui). */
export interface GuideRef {
	guideId: GuideId;
	guideLabel: string;
	stepKey: string;
	localId: string;
	sectionId: string;
	text: string;
	done: boolean;
}

export interface GoalVM {
	id: string;
	/** Entidade-alvo (ex.: `item:barrows-gloves`). */
	target: string;
	pinned: boolean;
	note?: string;
}

/* Conta */

export interface SkillVM {
	skill: string;
	level: number;
	xp: number;
	/** Próximo marco da rota ativa para esta skill. */
	next?: { level: number; stepId: string };
}

export interface BankItemVM {
	name: string;
	qty: number;
	/** Passo da rota que consome o item (consciência de "não vender"). */
	reservedFor?: string;
}

export interface SessionVM {
	id: string;
	start: number;
	end: number;
	changes: { kind: 'level' | 'quest' | 'step' | 'unlock' | 'kc'; text: string }[];
}

export interface SourceVM {
	name: string;
	at: number | null;
	state: 'ok' | 'stale' | 'off' | 'error';
	detail: string;
}

export interface AccountVM {
	rsn: string;
	mode: string;
	totalLevel: number;
	combat: number;
	questPoints: number;
	skills: SkillVM[];
	diaries: { area: string; tiers: ('done' | 'available' | 'blocked')[] }[];
	bank: BankStatus;
	bankItems: BankItemVM[];
	sessions: SessionVM[];
	sources: SourceVM[];
}

/* Microbot (controle remoto) */

export type ClientState = 'offline' | 'starting' | 'login_screen' | 'logged_in' | 'error';
export type ScriptState = 'running' | 'paused' | 'stopped' | 'failed';

export interface ScriptVM {
	name: string;
	version: string;
	state: ScriptState;
	runtimeMs: number;
	/** Estado atual da state machine (`/debug/snapshot`). */
	machineState?: string;
	health?: 'ok' | 'warn' | 'error';
	result?: string;
}

export interface RoutineStepVM {
	kind:
		| 'client_start'
		| 'login'
		| 'plugin_config'
		| 'plugin_start'
		| 'wait_until'
		| 'plugin_stop'
		| 'logout'
		| 'client_stop';
	label: string;
}

export interface RoutineVM {
	id: string;
	name: string;
	enabled: boolean;
	schedule?: string;
	nextRunAt?: number;
	steps: RoutineStepVM[];
	lastRun?: { at: number; status: 'ok' | 'failed' | 'running'; detail: string };
}

export interface AuditEventVM {
	at: number;
	actor: string;
	action: string;
	result: 'ok' | 'failed';
}

export interface MicrobotVM {
	/** `MICROBOT_CONTROL`: desligado por padrão no servidor. */
	controlEnabled: boolean;
	agentConnected: boolean;
	client: ClientState;
	world?: number;
	character?: string;
	position?: { region: string; x: number; y: number; plane: number };
	hp?: { now: number; max: number };
	prayer?: { now: number; max: number };
	run?: number;
	inventoryUsed?: number;
	/** Humano jogando: o agent não inicia ações. */
	humanInput: boolean;
	scripts: ScriptVM[];
	routines: RoutineVM[];
	audit: AuditEventVM[];
	screenshotAt: number | null;
}
