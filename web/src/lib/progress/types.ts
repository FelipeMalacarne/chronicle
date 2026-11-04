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

export interface Readiness {
	guide: string;
	step?: string;
	current: SegmentReadiness | null;
	next: SegmentReadiness | null;
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
