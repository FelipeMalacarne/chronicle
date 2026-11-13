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

/** O valor atual de um objetivo com número ("240 de 1 250"): nível, unidades, quest points, kc. */
export interface ObservedValue {
	value: number;
	target: number;
}

export interface BoardObjective {
	key: string;
	role: Role;
	verdict: Verdict;
	regressed?: boolean;
	/** Ausente sem número ou quando o valor atual não é sabido (nunca vira "faltando"). */
	observed?: ObservedValue;
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
	/** As páginas do collection log que o plugin leu com o log aberto no jogo; vazio até abrir. */
	collection_log_pages: CollectionLogPageView[];
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

/** O collection log pelos contadores do jogo (varps COLLECTION_COUNT e _MAX). */
export interface CollectionLogView {
	obtained: number;
	total?: number;
	source: string;
	observed_at: string;
}

/**
 * Uma página do collection log como o plugin a leu da última vez, com o log aberto no jogo. Página
 * nunca aberta não aparece: os itens dela ficam desconhecidos, nunca faltando.
 */
export interface CollectionLogPageView {
	page: string;
	/** Aba que lista a página; ausente quando o plugin não soube dizer. */
	tab?: 'BOSSES' | 'RAIDS' | 'CLUES' | 'MINIGAMES' | 'OTHER';
	obtained: number;
	total: number;
	/** Itens obtidos, com quantos. */
	items: CollectionLogItemView[];
	/** Espaços que a página mostrou sem o item. */
	missing: CollectionLogItemView[];
	source: string;
	observed_at: string;
}

export interface CollectionLogItemView {
	id: number;
	name: string;
	/** Só nos obtidos. */
	qty?: number;
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

/* ------------------------------------------------------------------ níveis (Histórico e Atividade da v7) */

/**
 * Uma skill que subiu de nível ("14:06 Felipe Pesca subiu para Fishing 76"), na primeira observação
 * que mostrou o nível. Um por nível: dois níveis entre duas observações são dois eventos no mesmo instante.
 */
export interface LevelEvent {
	account: string;
	skill: string;
	level: number;
	at: string;
}

/** O avanço de uma skill na janela ("Fishing 71 → 76 +412k"). */
export interface SkillGain {
	skill: string;
	from_level: number;
	to_level: number;
	/** XP ganho na janela. */
	xp: number;
}

/** GET /api/account/levels: os níveis da conta desde `since` (padrão: as últimas 24 h; no máximo 7 dias). */
export interface AccountLevels {
	account: string;
	since: string;
	/** Quantos níveis a janela tem; `events` traz os `limit` mais novos. */
	total: number;
	/** Do mais novo para o mais antigo. */
	events: LevelEvent[];
	/** Só as skills que ganharam XP na janela, a que mais ganhou primeiro. */
	skills: SkillGain[];
}

/** GET /api/levels: os níveis de todas as contas do usuário, juntos e do mais novo para o mais antigo (o Diário). */
export interface FleetLevels {
	since: string;
	total: number;
	events: LevelEvent[];
}

/* ------------------------------------------------------------------ Mundo da frota (V7Mundo1440, fatia V10) */

/** Uma meta que uma conta fixou, como a frota a mostra. */
export interface PinnedGoal {
	id: string;
	/** Vazio quando a conta não deu nome à meta: vale o `name` do objetivo. */
	label: string;
	priority: number;
	status: 'active' | 'achieved' | 'abandoned';
	created_at: string;
	achieved_at?: string;
}

/**
 * Os requisitos do plano da meta para uma conta: feitos, presumidos pela marca, desconhecidos e o
 * total; o resto é sabidamente não cumprido.
 */
export interface RequirementCount {
	met: number;
	presumed: number;
	unknown: number;
	total: number;
	/** Quests requisito sabidamente não feitas. */
	quests_left: number;
}

/** Um nível (ou os quest points, skill "qp") que o plano pede e a conta sabidamente não tem. */
export interface Shortfall {
	key: string;
	skill: string;
	/** Nulo sem o fato do nível; sempre nulo nos quest points, por enquanto. */
	level: number | null;
	target: number;
}

/** Uma conta diante de um objetivo da frota. */
export interface FleetStanding {
	account: string;
	/** Veredito guardado (travado); unknown nunca aparece como unmet. */
	verdict: Verdict;
	regressed?: boolean;
	/** A meta da conta para este objetivo; ausente quando ela não o persegue. */
	goal?: PinnedGoal;
	requirements: RequirementCount;
	/** Nível da conta na skill de uma meta de nível, dos fatos dela. */
	level?: number;
	short: Shortfall[];
	/** Primeiro passo da rota ativa que conclui o objetivo. */
	step?: StepPlace;
	/** Passos da rota ativa nem feitos nem pulados até `step`, ele incluído; 0 quando ele está feito ou a conta já tem o objetivo. */
	steps_left?: number;
	/** O cursor da conta na rota ativa; ausente quando a rota principal acabou. */
	cursor?: StepPlace;
}

/** Um objetivo na frota: as metas de todas as contas e as chaves pedidas em `keys=`. */
export interface FleetGoal {
	objective_key: string;
	/** Contas que o perseguem (fixaram a meta), que o têm (met) e sem veredito (unknown). */
	pinned: number;
	met: number;
	unknown: number;
	/** A conta mais perto entre as que não o têm (entre as que o perseguem, se alguma persegue). */
	closest?: string;
	/** Todas as contas, na ordem de `accounts` da resposta. */
	accounts: FleetStanding[];
	name: string;
	ref: string;
	kind: string;
	wiki?: string;
	/** Onde cada rota conclui a entidade (o primeiro passo de cada uma). */
	routes: Occurrence[];
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

/** GET /api/fleet/goals: as contas do usuário (a ordem de /api/me) e os objetivos da frota. */
export interface FleetGoalsResponse {
	accounts: AccountView[];
	goals: FleetGoal[];
}

export interface SyncRunsResponse {
	account: string;
	runs: SyncRun[];
}

/** O ritmo de uma conta (fatia V5 da v7): a skill que treina e quão rápido, a sessão, o dia e o GP. */
export interface RhythmTraining {
	skill: string;
	level: number;
	xp: number;
	/** 0 quando a janela é curta demais para medir. */
	xp_per_hour: number;
	/** Ausente no 99 ou sem ritmo. */
	next_level_in_s?: number;
	last_gain_at: string;
}

export interface RhythmSession {
	started_at: string;
	xp: number;
	/**
	 * Os itens que a conta tem a mais do que no começo da sessão (banco, inventário e equipamento
	 * juntos), do maior para o menor; null quando o banco ou o inventário não eram conhecidos nas duas pontas.
	 */
	gained: RhythmItem[] | null;
	/** Os itens que a sessão vai gastando, o que acaba primeiro antes; null como `gained`. */
	running_out: RhythmRunOut[] | null;
}

export interface RhythmItem {
	item: number;
	name?: string;
	qty: number;
}

export interface RhythmRunOut {
	item: number;
	name?: string;
	used: number;
	left: number;
	/** Quando acabam, no ritmo da sessão. */
	in_s: number;
}

export interface RhythmSkillDay {
	skill: string;
	xp: number;
	levels: number;
	level: number;
}

export interface RhythmToday {
	since: string;
	xp: number;
	levels: number;
	skills: RhythmSkillDay[];
}

export interface RhythmGP {
	value: number;
	/** Ausente quando nenhuma observação mostra o que havia no começo do dia. */
	today?: number;
}

export interface AccountRhythm {
	account: string;
	training: RhythmTraining | null;
	session: RhythmSession | null;
	today: RhythmToday;
	gp: RhythmGP | null;
}

export interface FleetRhythm {
	since: string;
	accounts: AccountRhythm[];
	totals: {
		xp_today: number;
		levels_today: number;
		gaining: number;
		gp?: number;
		gp_today?: number;
	};
}

/**
 * Planos (docs/10, seção 9): a lista que a conta segue e a próxima tarefa tipada dela
 * (internal/core/plan, internal/httpapi/plans.go). Ainda sem tela: a dos planos
 * customizados é da fase 5 e pede o canvas antes.
 */
export type PlanKind = 'route' | 'goal' | 'custom';
export type PlanItemKind = 'step' | 'recipe' | 'task';
export type TaskType = 'travel' | 'bank' | 'buy' | 'talk' | 'interact' | 'train' | 'manual';

/** Uma tarefa tipada (content/schema/task.schema.json): só refs do catálogo, números e ids. */
export interface TypedTask {
	type: TaskType;
	location?: string;
	via?: string;
	items?: ItemRequirement[];
	shop?: string;
	item?: string;
	qty?: number;
	npc?: string;
	options?: string[];
	object?: string;
	action?: string;
	skill?: string;
	note?: string;
	until?: { objective?: string; state?: string };
}

export interface PlanItem {
	kind: PlanItemKind;
	guide?: string;
	step?: string;
	recipe?: string;
	task?: TypedTask;
}

export interface Plan {
	id: string;
	kind: PlanKind;
	title: string;
	guide?: string;
	goal?: string;
	items: PlanItem[];
}

export interface PlansResponse {
	account: string;
	active: string;
	plans: Plan[];
}

export interface PlanItemView {
	kind: PlanItemKind;
	key: string;
	position: number;
	guide?: string;
	step?: string;
	ref?: string;
	title?: string;
	status?: StepStatus;
	/** A linha de crédito do guia (CLAUDE.md regra 6), num item de passo. */
	attribution?: string;
	recipe?: string;
	label?: string;
}

export interface PlanTask {
	key: string;
	item: string;
	index: number;
	type: TaskType;
	label: string;
	until: string;
	/** `unknown` nunca é "a fazer" por padrão, mas só `met` é pulado. */
	state: Verdict;
	reason?: string;
	recipe?: string;
	for?: string;
	risks: string[];
	/** HCIM com risco de morte: só roda com o dono olhando (E9). */
	supervised: boolean;
	executable: boolean;
	not_executable?: string;
	task: TypedTask;
}

export interface PlanNext {
	account: string;
	plan: { id: string; kind: PlanKind; title: string; guide?: string };
	status: 'task' | 'waiting' | 'manual' | 'done';
	reason?: string;
	item: PlanItemView | null;
	task: PlanTask | null;
	skipped: PlanTask[];
}
