import type { LeafRule, Rule, Skill, SkillLevelRule, Step } from './types';

export interface RuleBadge {
	/** Texto curto, ex.: "firemaking ≥ 50", "quest FINISHED". */
	label: string;
	tone: 'accent' | 'muted' | 'warning';
	/** true quando a regra pode ser verificada por sync (não é manual). */
	auto: boolean;
}

function isLeaf(r: Rule): r is LeafRule {
	return 'type' in r;
}

/** Achata all/any/not em folhas (prefixando "não " quando negado). */
export function flattenRules(rules: Rule[] | undefined): { rule: LeafRule; negated: boolean }[] {
	const out: { rule: LeafRule; negated: boolean }[] = [];
	const walk = (r: Rule, negated: boolean) => {
		if (isLeaf(r)) out.push({ rule: r, negated });
		else if ('all' in r) r.all.forEach((x) => walk(x, negated));
		else if ('any' in r) r.any.forEach((x) => walk(x, negated));
		else if ('not' in r) walk(r.not, !negated);
	};
	(rules ?? []).forEach((r) => walk(r, false));
	return out;
}

function leafLabel(r: LeafRule): string {
	switch (r.type) {
		case 'skill_level':
			return `${r.skill} ≥ ${r.level}`;
		case 'skill_xp':
			return `${r.skill} ≥ ${r.xp.toLocaleString('pt-BR')} xp`;
		case 'quest_state':
			return `quest ${r.state}`;
		case 'diary_tier':
			return `diary ${r.area} ${r.tier}`;
		case 'diary_task':
			return `diary ${r.area} ${r.tier} #${r.task}`;
		case 'combat_achievement':
			return r.name ? `CA ${r.name}` : `CA ${r.tier ?? ''} ×${r.min_count ?? 1}`.trim();
		case 'boss_kc':
			return `${r.boss} kc ≥ ${r.kc}`;
		case 'activity_score':
			return `${r.activity} ≥ ${r.score}`;
		case 'collection_log_item':
			return `clog ${r.item_name ?? r.item_id}`;
		case 'item_owned':
			return `item ${r.item_name ?? r.item_id}${r.qty && r.qty > 1 ? ` ×${r.qty}` : ''}`;
		case 'varbit':
			return r.label ?? `varbit ${r.id}`;
		case 'varp':
			return r.label ?? `varp ${r.id}`;
		case 'manual':
			return r.label ?? 'manual';
	}
}

/** Badges derivados de complete_when. Regras `manual` não geram badge. */
export function ruleBadges(step: Pick<Step, 'complete_when'>): RuleBadge[] {
	return flattenRules(step.complete_when)
		.filter(({ rule }) => rule.type !== 'manual')
		.map(({ rule, negated }) => ({
			label: (negated ? 'não ' : '') + leafLabel(rule),
			tone: rule.type === 'skill_level' || rule.type === 'skill_xp' ? 'accent' : 'muted',
			auto: true
		}));
}

/** Passo tem pelo menos uma regra automática (não manual)? */
export function hasAutoRule(step: Pick<Step, 'complete_when'>): boolean {
	return flattenRules(step.complete_when).some(({ rule }) => rule.type !== 'manual');
}

export interface SkillMilestone {
	skill: Skill;
	level: number;
	stepId: string;
	stepText: string;
}

/** Próximos marcos de skill: menor nível pedido por skill nos passos dados, em ordem de aparição. */
export function skillMilestones(steps: Step[], limit = 6): SkillMilestone[] {
	const seen = new Map<Skill, SkillMilestone>();
	for (const s of steps) {
		for (const { rule, negated } of flattenRules(s.complete_when)) {
			if (negated || rule.type !== 'skill_level') continue;
			const r = rule as SkillLevelRule;
			const prev = seen.get(r.skill);
			if (!prev || r.level < prev.level) {
				seen.set(r.skill, { skill: r.skill, level: r.level, stepId: s.id, stepText: s.text });
			}
		}
	}
	return [...seen.values()].slice(0, limit);
}

/** Agrupa passos por location preservando a ordem do guia; sem location vira "—". */
export function groupByLocation(steps: Step[]): { location: string; steps: Step[] }[] {
	const groups: { location: string; steps: Step[] }[] = [];
	for (const s of steps) {
		const loc = s.location?.trim() || 'Sem local';
		const last = groups[groups.length - 1];
		if (last && last.location === loc) last.steps.push(s);
		else groups.push({ location: loc, steps: [s] });
	}
	return groups;
}
