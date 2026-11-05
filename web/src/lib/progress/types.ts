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

export interface Objective {
	key: string;
	role: Role;
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

export interface ContentStep {
	id: string;
	text: string;
	location?: string;
	notes?: Note[];
	items?: (string | { name: string; qty?: number })[];
	links?: Link[];
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
}

export interface ContentSegment {
	id: string;
	kind: 'bank_leg' | 'sequence';
	title?: string;
	bank_actions?: BankAction[];
	loadout?: Loadout;
	steps: { id: string }[];
}

export interface ContentSection {
	id: string;
	slug: string;
	title: string;
	description?: string;
	kind?: string;
	guide: string;
	segments?: ContentSegment[];
	steps: ContentStep[];
	names: Record<string, string>;
}

export interface GuideInfo {
	id: string;
	title: string;
	version?: string;
	source?: GuideSource;
	steps: number;
	segments: number;
	sections: {
		id: string;
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
	held?: number;
	containers: Partial<Record<string, { state?: ContainerState; observed: boolean; qty: number }>>;
	reason?: string;
}

export interface ReadinessItem {
	name: string;
	variant: 'exact' | 'any_of' | 'family' | 'unresolved';
	item_id?: number;
	any_of?: number[];
	family?: string;
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
	first_step_id?: string;
	status: 'ready' | 'listed_ready' | 'not_ready' | 'unknown' | 'no_loadout';
	loadout_status: 'known' | 'partial' | 'unknown';
	items: ReadinessItem[];
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
	containers: ContainerView[];
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
