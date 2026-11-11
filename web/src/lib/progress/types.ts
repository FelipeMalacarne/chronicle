/**
 * Contrato HTTP do progresso sobre objetivos e do conteúdo composto (internal/httpapi:
 * progress.go, content_index.go, handlers.go). Mantido à mão; ver docs/04, seção 2.5.
 */

export type Verdict = 'met' | 'unmet' | 'unknown';
export type Role = 'completes' | 'starts' | 'requires' | 'uses';
export type StepStatus =
	'pending' | 'auto_done' | 'manual_done' | 'skipped' | 'pinned_open' | 'implied';
export type MarkKind = 'manual_done' | 'skipped' | 'pinned_open' | 'implied';
export type MarkAction = 'manual_done' | 'skipped' | 'pinned_open' | 'confirm_implied' | 'clear';

export interface Mark {
	kind: MarkKind;
	implied_by_step_id?: string;
	note?: string;
}

export interface BoardObjective {
	key: string;
	role: Role;
	verdict: Verdict;
	regressed?: boolean;
}

export interface BoardStep {
	id: string;
	/** Referência do autor (1.1.75, CH1.6); o id interno nunca aparece na tela. */
	ref: string;
	title: string;
	section_id: string;
	status: StepStatus;
	mark?: Mark;
	manual: boolean;
	main: boolean;
	visible: boolean;
	/** Requisitos em unmet de um passo pendente: o cursor passa dele (docs/09 seção 3.5). */
	blocked_by?: string[];
	objectives: BoardObjective[];
}

export interface Counts {
	total: number;
	done: number;
	auto_done: number;
	manual_done: number;
	implied: number;
	skipped: number;
	pinned_open: number;
	remaining: number;
	/** Passos restantes com requisito em unmet. */
	blocked: number;
}

export interface SectionCounts {
	id: string;
	title: string;
	track: boolean;
	counts: Counts;
}

export interface Coverage {
	objectives: number;
	met: number;
	checkbox: number;
	checkbox_done: number;
	checkbox_skipped: number;
}

export interface Cursor {
	step_id: string;
	cause: 'pin' | 'auto';
}

export interface Board {
	guide: string;
	account: string;
	active: boolean;
	cursor: Cursor | null;
	orphans: { step_id: string; mark: Mark }[];
	steps: BoardStep[];
	counts: Counts;
	sections: SectionCounts[];
	coverage: Coverage;
	/** Passos marcados por um lote (marcar tudo antes) e como desfazê-lo. */
	marked?: string[];
	undo?: MarkRestore[];
	/** O comando que esta escrita registrou; um desfazer o cita em `revokes`. */
	command_id?: string;
}

/** Um passo do desfazer de um lote: a marca que ele tinha (null: nenhuma). */
export interface MarkRestore {
	step_id: string;
	mark: Mark | null;
}

export interface GuideSource {
	origin?: string;
	original_id?: string;
	author?: string;
	url?: string;
	license?: string;
	imported_at?: string;
}

export interface RouteSummary {
	guide: string;
	title: string;
	version?: string;
	source?: GuideSource;
	steps: number;
	segments: number;
	active: boolean;
	cursor: Cursor | null;
	cursor_ref?: string;
	cursor_section?: string;
	counts: Counts;
	coverage: Coverage;
	sections: SectionCounts[];
}

export interface SourceView {
	source: string;
	last_observed_at: string;
	facts: number;
}

export interface ProgressSummary {
	account: string;
	active_guide: string;
	routes: RouteSummary[];
	sources: SourceView[];
}

export interface StepEventView {
	step_id: string;
	from?: StepStatus;
	to: StepStatus;
	cause: 'engine' | 'user' | 'implied' | 'migration' | 'import';
	source?: string;
	at: string;
}

export interface ObjectiveEventView {
	key: string;
	from?: Verdict;
	to: Verdict;
	source?: string;
	at: string;
}

export interface EventsView {
	guide: string;
	account: string;
	steps: StepEventView[];
	objectives: ObjectiveEventView[];
}

/* ------------------------------------------------------------------ conteúdo composto */

/** Como um dado estruturado do conteúdo foi obtido (content.Provenance). */
export interface Provenance {
	method: string;
	confidence: string;
	step_ids: string[] | null;
	evidence?: string;
}

export interface Objective {
	key: string;
	role: Role;
	evidence?: { field: string; quote?: string; span?: number[] };
	source?: {
		method: string;
		confidence: string;
		skill_version?: string;
		confirmed_by?: string;
		decision?: string;
	};
}

export interface ItemRequirement {
	name: string;
	item_id?: number;
	any_of?: number[];
	family?: string;
	qty?: number;
	noted?: boolean;
	optional?: boolean;
	source_text?: string;
}

export interface BankAction {
	type: string;
	position: string;
	text: string;
	source?: Provenance;
}

export interface Occurrence {
	guide: string;
	step: string;
	ref: string;
	title?: string;
	section: string;
	position?: number;
	total: number;
	key: string;
	role: Role;
}

export interface Note {
	kind: string;
	text: string;
}

export interface Link {
	type: string;
	url: string;
	text?: string;
}

/** Trecho colorido do texto da fonte (BRUHsailer): span em bytes UTF-8 do texto (view.ts converte). */
export interface TextMark {
	span: [number, number];
	color: 'red' | 'orange' | 'green';
}

/**
 * O passo como o conteúdo o guarda (content.Step): é o que os trechos de uma seção trazem.
 * Campos que a web não lê ficam como `unknown`.
 */
export interface GuideStep {
	id: string;
	text?: string;
	location?: string;
	notes?: Note[];
	items?: (
		string | { name: string; item_id?: number; qty?: number; equip?: boolean; note?: string }
	)[];
	links?: Link[];
	complete_when?: unknown;
	prerequisites?: unknown;
	requires?: string[];
	recommended_after?: string[];
	unlocks?: string[];
	tags?: string[];
	modes?: string[];
	layers?: string[];
	phase?: string;
	optional?: boolean;
	estimate_minutes?: number;
	replaces?: string[];
	source?: GuideSource;
	entities?: unknown[];
	objectives?: Objective[];
	location_ref?: string;
	requirements?: ItemRequirement[];
	bank_actions?: BankAction[];
	marks?: TextMark[];
}

export interface ContentStep {
	id: string;
	text: string;
	location?: string;
	notes?: Note[];
	items?: (
		string | { name: string; item_id?: number; qty?: number; equip?: boolean; note?: string }
	)[];
	links?: Link[];
	marks?: TextMark[];
	complete_when?: unknown;
	prerequisites?: unknown;
	requires?: string[];
	recommended_after?: string[];
	unlocks?: string[];
	tags?: string[];
	phase?: string;
	optional?: boolean;
	estimate_minutes?: number;
	replaces?: string[];
	source?: GuideSource;
	entities?: unknown[];
	layers?: string[];
	modes?: string[];
	objectives?: Objective[];
	location_ref?: string;
	requirements?: ItemRequirement[];
	bank_actions?: BankAction[];
	section: string;
	guide: string;
	manual: boolean;
	ref: string;
	position?: number;
	total: number;
	names: Record<string, string>;
	also?: Record<string, Occurrence[]>;
}

export interface Loadout {
	status: 'known' | 'partial' | 'unknown';
	inventory?: ItemRequirement[];
	equipment?: ItemRequirement[];
	empty_slots?: number;
	source?: Provenance;
}

export interface ContentSegment {
	id: string;
	kind: 'bank_leg' | 'sequence';
	title?: string;
	bank_actions?: BankAction[];
	loadout?: Loadout;
	source?: Provenance;
	steps: GuideStep[];
}

export interface ContentSection {
	id: string;
	slug: string;
	title: string;
	description?: string;
	kind?: string;
	related?: unknown[];
	tags?: string[];
	guide: string;
	segments?: ContentSegment[];
	steps: ContentStep[];
	names: Record<string, string>;
}

export interface GuideInfo {
	id: string;
	slug?: string;
	title: string;
	version?: string;
	source?: GuideSource;
	steps: number;
	segments: number;
	sections: {
		id: string;
		guide: string;
		slug: string;
		title: string;
		kind: string;
		track: boolean;
		steps: number;
	}[];
}

/* ------------------------------------------------------------------ readiness (doc 04, 6.4.1) */

export type ContainerState = 'unavailable' | 'restored' | 'live';

export interface Ownership {
	verdict: Verdict;
	stale?: boolean;
	held: number;
	required?: number;
	containers: Partial<
		Record<
			string,
			{
				state?: ContainerState;
				observed: boolean;
				complete?: boolean;
				qty: number;
				observed_at?: string;
			}
		>
	>;
	reason?: string;
}

export interface ReadinessItem {
	name: string;
	variant: 'exact' | 'any_of' | 'family' | 'unresolved';
	item_id?: number;
	any_of?: number[];
	family?: string;
	noted?: boolean;
	matched_id?: number;
	slot: 'inventory' | 'equipment';
	required: number;
	qty_stated: boolean;
	optional?: boolean;
	source_text?: string;
	verdict: Verdict;
	carried: number;
	owned?: Ownership;
	reason?: string;
}

export interface SegmentReadiness {
	guide_id: string;
	segment_id: string;
	section_id: string;
	kind: 'bank_leg' | 'sequence';
	title?: string;
	first_step_id?: string;
	status: 'ready' | 'listed_ready' | 'not_ready' | 'unknown' | 'no_loadout';
	loadout_status: 'known' | 'partial' | 'unknown';
	items: ReadinessItem[];
	empty_slots?: {
		required: number;
		free_min?: number;
		free_max?: number;
		verdict: Verdict;
		reason?: string;
	};
	bank_actions: BankAction[];
	freshness: Record<
		string,
		{
			state?: ContainerState;
			observed: boolean;
			complete: boolean;
			live: boolean;
			observed_at?: string;
		}
	>;
	reason?: string;
}

export interface AheadItem extends ReadinessItem {
	also_at?: string[];
}

/** Passo do primeiro uso; offset conta os passos depois de from_step_id. */
export interface AheadGroup {
	step_id: string;
	text: string;
	offset: number;
	items: AheadItem[];
}

/** Item que um passo anterior da janela obtém (objetivo de item). */
export interface AheadObtained {
	name: string;
	item_id: number;
	qty: number;
	step_id: string;
	use_step_id: string;
	use_offset: number;
}

/** Prévia derivada dos passos quando o guia não lista o setup do trecho; nunca é readiness. */
export interface Ahead {
	from_step_id: string;
	until_step_id: string;
	steps: number;
	groups: AheadGroup[];
	obtained: AheadObtained[];
}

export interface Readiness {
	guide: string;
	step?: string;
	current: SegmentReadiness | null;
	next: SegmentReadiness | null;
	ahead?: Ahead;
}

/* ------------------------------------------------------------------ metas (F9) e Mundo */

export interface Goal {
	id: string;
	objective_key: string;
	label: string;
	priority: number;
	status: 'active' | 'achieved' | 'abandoned';
	created_at: string;
	achieved_at?: string;
	verdict: Verdict;
	regressed?: boolean;
	name: string;
	ref: string;
	kind: string;
	/** Onde cada rota conclui a entidade da meta (o primeiro passo de cada uma). */
	routes: Occurrence[];
}

export interface WorldObjective {
	key: string;
	verdict: Verdict;
	regressed?: boolean;
}

export interface WorldEntity {
	ref: string;
	kind: string;
	name: string;
	wiki?: string;
	objectives: WorldObjective[];
	routes: Occurrence[];
}

/** Requisito de uma entidade, ou entidade que ela destrava, com o veredito da conta. */
export interface WorldLink {
	key: string;
	ref?: string;
	label: string;
	verdict: Verdict;
	boostable?: boolean;
	during_quest?: boolean;
}

/** Uma entidade para o painel (GET /api/world/entity): requer → destrava. */
export interface WorldEntityDetail extends WorldEntity {
	requires: WorldLink[];
	combat_level?: number;
	unlocks: WorldLink[];
}

/* ------------------------------------------------------------------ sync (hiscores) */

export interface SyncRun {
	id: string;
	source: string;
	trigger: 'scheduled' | 'manual' | 'push' | 'boot';
	status: 'queued' | 'running' | 'ok' | 'unchanged' | 'skipped' | 'rate_limited' | 'error';
	started_at?: string;
	finished_at?: string;
	http_status?: number;
	error?: string;
	snapshot_id?: string;
	steps_changed: number;
	next_allowed_at?: string;
	created_at: string;
}

/* ------------------------------------------------------------------ estado da conta (V6Conta) */

export interface FactSource {
	source: string;
	observed_at: string;
}

export interface SkillView extends FactSource {
	skill: string;
	level: number;
	xp: number;
}

export interface QuestView extends FactSource {
	id?: string;
	name: string;
	state: 'NOT_STARTED' | 'IN_PROGRESS' | 'FINISHED';
}

export interface DiaryView extends FactSource {
	area: string;
	tier: 'easy' | 'medium' | 'hard' | 'elite';
	complete: boolean;
}

export interface ContainerView {
	container: 'inventory' | 'equipment' | 'bank' | string;
	state: ContainerState;
	observed_at: string;
	complete: boolean;
	/** slot: o slot de equipamento do item no catálogo ("head", "2h", ...). */
	items: { id: number; name: string; qty: number; slot?: string }[];
}

/** GET /api/account/state: o que a Conta mostra, cada fato com fonte e idade. */
export interface AccountState {
	account: AccountView;
	total_level?: number;
	quest_points?: number;
	skills: SkillView[];
	quests: QuestView[];
	diaries: DiaryView[];
	/** Por tier, na ordem do jogo; vazio até o plugin mandar os varps dos CA. */
	combat_achievements: CATierView[];
	collection_log?: CollectionLogView;
	containers: ContainerView[];
}

/** Um tier de combat achievements: feitos pelos bits dos varps, e o total que o próprio jogo guarda. */
export interface CATierView {
	tier: 'easy' | 'medium' | 'hard' | 'elite' | 'master' | 'grandmaster';
	done: number;
	total: number;
	game_done?: number;
	source: string;
	observed_at: string;
}

/** Um relógio do Time Tracking que o plugin mandou (decisão 31 do doc 01: recorrentes com ready_at). */
export interface TimerView {
	key: string;
	label?: string;
	state: 'COMPLETED' | 'IN_PROGRESS' | 'OCCUPIED' | 'EMPTY' | 'UNKNOWN';
	/** Quando fica (ou ficou) pronto, em segundos desde a época; só crescendo ou pronto. */
	ready_at?: number;
	source: string;
	observed_at: string;
}

/** GET /api/guides/{guide}/nearby: os passos abertos da rota perto do personagem (item 16). */
export interface NearbyView {
	/** A última posição que o plugin mandou (opt-in "Send position"); null até mandar. */
	position: {
		world: number;
		x: number;
		y: number;
		plane: number;
		region: number;
		source: string;
		observed_at: string;
	} | null;
	/** Até onde conta como perto, em tiles. */
	radius: number;
	steps: {
		id: string;
		ref: string;
		title: string;
		section_id: string;
		location: string;
		location_name: string;
		distance: number;
	}[];
}

/** GET /api/account/timers: os Recorrentes do Agora. */
export interface TimersView {
	timers: TimerView[];
}

/** O collection log pelos contadores do jogo (o plugin não abre o log). */
export interface CollectionLogView {
	obtained: number;
	total?: number;
	source: string;
	observed_at: string;
}

/* ------------------------------------------------------------------ setup (V4Setup) */

export type AccountMode = 'ironman' | 'hcim' | 'uim' | 'gim';

export interface AccountView {
	id: string;
	rsn: string;
	mode: AccountMode;
	guide_id: string;
	layer: 'classic' | 'enhanced';
}

export interface SetupLookup {
	rsn: string;
	account: AccountView | null;
	found: boolean;
	/** Modo da tabela mais específica que conhece o nome; vazio quando só a normal conhece. */
	mode: '' | 'ironman' | 'hcim' | 'uim';
	total_level?: number;
	skills: { skill: string; level: number }[];
	hiscores_error?: string;
}

/* ------------------------------------------------------------------ plano da meta (F9 parte B) */

export type GoalGroup = 'done' | 'presumed' | 'available' | 'blocked' | 'unknown';

/** Onde uma rota faz algo: o passo e a posição na seção. */
export interface StepPlace {
	guide_id: string;
	step_id: string;
	ref: string;
	title: string;
	section_id: string;
	section_pos: number;
	role?: string;
	/** O passo conclui a própria meta. */
	goal?: boolean;
	done?: boolean;
}

export interface GoalNode {
	key: string;
	ref?: string;
	label: string;
	depth: number;
	verdict: Verdict;
	presumed?: boolean;
	group: GoalGroup;
	boostable?: boolean;
	during_quest?: boolean;
	requires?: string[];
	missing?: string[];
	for?: string[];
	step?: StepPlace;
}

/** Um cartão do grafo por foco (GET /api/graph): um objetivo ou os níveis de uma quest juntos. */
export interface GraphNode {
	key: string;
	ref?: string;
	/** quest, skill, item, unlock, diary, qp ou levels (os níveis de um nó, num cartão só). */
	kind: string;
	label: string;
	depth: number;
	verdict: Verdict;
	group: GoalGroup;
	/** Os nós da coluna mais perto do foco que pedem este. */
	for?: string[];
	/** Os requisitos diretos ainda não cumpridos. */
	missing?: string[];
	requires?: number;
	step?: StepPlace;
	critical?: boolean;
	levels?: GraphLevel[];
}

export interface GraphLevel {
	key: string;
	skill: string;
	target: number;
	level?: number;
	verdict: Verdict;
}

export interface GraphColumn {
	depth: number;
	nodes: GraphNode[];
	done: GraphNode[];
}

/** Grafo por foco (F10, V6Grafo1440/640): colunas de requisitos, o foco e o que ele destrava. */
export interface Graph {
	focus: GraphNode;
	depth: number;
	max_depth: number;
	columns: GraphColumn[];
	unlocks: GraphNode[];
	critical: string[];
	nodes: number;
	done_nodes: number;
	routes: StepPlace[];
}

export interface Bottleneck {
	key: string;
	skill: string;
	level: number | null;
	target: number;
	met: boolean;
	boostable?: boolean;
	during_quest?: boolean;
	for?: string[];
	step?: StepPlace;
	reach?: number;
	reach_step?: StepPlace;
}

export interface GoalPlan {
	goal: string;
	requirements: boolean;
	met: number;
	total: number;
	presumed: number;
	nodes: GoalNode[];
	bottlenecks: Bottleneck[];
	routes: StepPlace[];
}

/* ------------------------------------------------------------------ envelopes das respostas */

export interface GoalsResponse {
	account: string;
	goals: Goal[];
}

export interface GoalPlanResponse {
	account: string;
	goal: Goal;
	plan: GoalPlan;
}

export interface WorldResponse {
	account: string;
	entities: WorldEntity[];
}

export interface SyncRunsResponse {
	account: string;
	runs: SyncRun[];
}
