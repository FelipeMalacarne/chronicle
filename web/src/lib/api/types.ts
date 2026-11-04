/**
 * Tipos do contrato HTTP do núcleo em Go (ver docs/04-arquitetura.md).
 * Mantidos à mão; os campos opcionais seguem o JSON normalizado em data/guide/normalized.
 */

export type Layer = 'classic' | 'enhanced';
export type Mode = 'ironman' | 'hcim' | 'uim' | 'gim';
export type QuestState = 'NOT_STARTED' | 'IN_PROGRESS' | 'FINISHED';
export type DiaryTier = 'easy' | 'medium' | 'hard' | 'elite';

export type Skill =
	| 'attack'
	| 'strength'
	| 'defence'
	| 'ranged'
	| 'prayer'
	| 'magic'
	| 'runecraft'
	| 'construction'
	| 'hitpoints'
	| 'agility'
	| 'herblore'
	| 'thieving'
	| 'crafting'
	| 'fletching'
	| 'slayer'
	| 'hunter'
	| 'mining'
	| 'smithing'
	| 'fishing'
	| 'cooking'
	| 'firemaking'
	| 'woodcutting'
	| 'farming'
	| 'sailing';

export interface SkillLevelRule {
	type: 'skill_level';
	skill: Skill;
	level: number;
	boosted?: boolean;
	inferred?: boolean;
	confidence?: 'low' | 'medium' | 'high';
}
export interface SkillXpRule {
	type: 'skill_xp';
	skill: Skill;
	xp: number;
}
export interface QuestStateRule {
	type: 'quest_state';
	quest: string;
	state: QuestState;
	op?: 'eq' | 'gte';
	or_finished?: boolean;
	confidence?: 'low' | 'medium' | 'high';
}
export interface DiaryTaskRule {
	type: 'diary_task';
	area: string;
	tier: DiaryTier;
	task: number | string;
}
export interface DiaryTierRule {
	type: 'diary_tier';
	area: string;
	tier: DiaryTier;
}
export interface CombatAchievementRule {
	type: 'combat_achievement';
	name?: string;
	tier?: 'easy' | 'medium' | 'hard' | 'elite' | 'master' | 'grandmaster';
	min_count?: number;
}
export interface BossKcRule {
	type: 'boss_kc';
	boss: string;
	kc: number;
}
export interface ActivityScoreRule {
	type: 'activity_score';
	activity: string;
	score: number;
}
export interface CollectionLogItemRule {
	type: 'collection_log_item';
	item_id?: number;
	item_name?: string;
	qty?: number;
}
export interface VarbitRule {
	type: 'varbit';
	id: number;
	eq?: number;
	min?: number;
	bit?: number;
	label?: string;
}
export interface VarpRule {
	type: 'varp';
	id: number;
	eq?: number;
	min?: number;
	bit?: number;
	label?: string;
}
export interface ItemOwnedRule {
	type: 'item_owned';
	item_id?: number;
	item_name?: string;
	qty?: number;
}
export interface ManualRule {
	type: 'manual';
	label?: string;
}

export type LeafRule =
	| SkillLevelRule
	| SkillXpRule
	| QuestStateRule
	| DiaryTaskRule
	| DiaryTierRule
	| CombatAchievementRule
	| BossKcRule
	| ActivityScoreRule
	| CollectionLogItemRule
	| VarbitRule
	| VarpRule
	| ItemOwnedRule
	| ManualRule;

export type Rule = LeafRule | { all: Rule[] } | { any: Rule[] } | { not: Rule };

export interface Link {
	type: 'image' | 'video' | 'guide' | 'wiki' | 'map' | 'other' | string;
	url: string;
	text?: string;
}

export interface QuestRef {
	name: string;
	status: string;
	wiki?: string;
}

export interface Step {
	id: string;
	text: string;
	location?: string | null;
	note?: string | null;
	update_note?: string | null;
	items?: string[];
	links?: Link[];
	layer: Layer;
	modes: Mode[];
	tags?: string[];
	quest?: QuestRef | null;
	complete_when?: Rule[];
	requires?: string[];
	phase?: string | null;
	track_via?: string;
}

export interface SectionSummary {
	id: string;
	slug: string;
	title: string;
	steps: number;
	track: boolean;
}

export interface Section {
	id: string;
	slug: string;
	title: string;
	description?: string;
	track: boolean;
	steps: Step[];
}

export interface Health {
	status: string;
	version: string;
	content: { guides: number; steps: number };
	db: string;
}

export interface Progress {
	done: number;
	total: number;
}

export interface Now {
	current: Step;
	next: Step[];
	progress: Progress;
}

/** Estado local de um passo. Só `pending` existe até o banco entrar. */
export type StepStatus = 'pending' | 'auto_done' | 'manual_done' | 'skipped';
