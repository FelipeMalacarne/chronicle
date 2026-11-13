/**
 * Objetivo com progresso (V7Fundamentos; V7Rota1440 e V7Rota640): no lugar do chip de uma linha, o
 * quanto falta ("240 / 1.250") quando o board traz o valor observado. "Não sabemos" continua
 * pontilhado e nunca vira "falta"; sem número conhecido, só o veredito. Puro, testado em node.
 */
import type { Action } from './passo.ts';
import type { BoardObjective, ObservedValue, Role } from './types.ts';
import { parseKey, type Chip } from './view.ts';

export interface GoalView {
	key: string;
	/** Ref da entidade, para o ícone e o painel de entidade ("" em quest points). */
	ref: string;
	role: Role;
	/** O nome sem a quantidade quando o número vai na barra ("Nature rune"). */
	name: string;
	/** "met", "unk" ou "" (o que falta, com ou sem número). */
	cls: '' | 'met' | 'unk';
	/** Largura da barra, 0 a 100; um objetivo que falta nunca enche. */
	pct: number;
	/** "240 / 1.250", "feito", "falta" ou "não sabemos". */
	text: string;
	title: string;
	/** Marcado pelo dono: "feito" sem evidência, com a opção de desfazer. */
	ownerMarked: boolean;
}

export interface GoalGroup {
	role: Role;
	label: string;
	goals: GoalView[];
}

const GROUPS: [Role, string][] = [
	['completes', 'Conclui'],
	['starts', 'Inicia'],
	['requires', 'Requer'],
	['uses', 'Usa']
];

const VERDICT_TEXT = { met: 'feito', unmet: 'falta', unknown: 'não sabemos' } as const;

/** "1.250", "78", "1,59M", "784k": o número do objetivo em pt-BR, curto quando passa de 100 mil. */
export function countLabel(n: number): string {
	const abs = Math.abs(n);
	if (abs >= 1_000_000) {
		const v = n / 1_000_000;
		return `${v
			.toFixed(abs >= 10_000_000 ? 0 : 2)
			.replace(/\.?0+$/, '')
			.replace('.', ',')}M`;
	}
	if (abs >= 100_000) return `${Math.floor(n / 1000)}k`;
	return n.toLocaleString('pt-BR');
}

/** O nome do alvo sem a quantidade na frente: "Nature rune", "Magic", "Quest points". */
export function goalName(chip: Pick<Chip, 'key' | 'label'>): string {
	const k = parseKey(chip.key);
	if (!k) return chip.label;
	if (k.kind === 'qp') return 'Quest points';
	if (k.alternatives) return chip.label;
	if (k.n !== undefined && (k.kind === 'item' || k.kind === 'skill' || k.kind === 'boss'))
		return chip.label.replace(/^\d+ /, '');
	return chip.label;
}

/** Um objetivo do passo como o cartão da v7 mostra. */
export function goalView(chip: Chip, observed?: ObservedValue): GoalView {
	const base = {
		key: chip.key,
		ref: chip.ref,
		role: chip.role,
		ownerMarked: chip.ownerMarked
	};
	const hint = chip.view.hint ? ` · ${chip.view.hint}` : '';
	if (chip.verdict === 'met')
		return {
			...base,
			name: observed ? goalName(chip) : chip.label,
			cls: 'met',
			pct: 100,
			text: VERDICT_TEXT.met,
			title: `${chip.roleLabel} ${chip.label} · ${chip.ownerMarked ? 'marcado por você' : 'feito'}${hint}`
		};
	if (chip.verdict === 'unknown')
		return {
			...base,
			name: chip.label,
			cls: 'unk',
			pct: 0,
			text: VERDICT_TEXT.unknown,
			title: `${chip.roleLabel} ${chip.label} · não sabemos${hint}`
		};
	if (observed && observed.target > 0) {
		const pct = Math.max(0, Math.min(99, Math.floor((100 * observed.value) / observed.target)));
		const text = `${countLabel(observed.value)} / ${countLabel(observed.target)}`;
		return {
			...base,
			name: goalName(chip),
			cls: '',
			pct,
			text,
			title: `${chip.roleLabel} ${chip.label} · tem ${observed.value.toLocaleString('pt-BR')}`
		};
	}
	return {
		...base,
		name: chip.label,
		cls: '',
		pct: 0,
		text: VERDICT_TEXT.unmet,
		title: `${chip.roleLabel} ${chip.label} · falta`
	};
}

/** Os objetivos do passo por papel, cada grupo com o rótulo que abre a sua linha. */
export function goalGroups(
	chips: Chip[],
	objectives: Pick<BoardObjective, 'key' | 'role' | 'observed'>[] | undefined
): GoalGroup[] {
	const seen = new Map((objectives ?? []).map((o) => [`${o.role}|${o.key}`, o.observed]));
	return GROUPS.map(([role, label]) => ({
		role,
		label,
		goals: chips
			.filter((c) => c.role === role)
			.map((c) => goalView(c, seen.get(`${role}|${c.key}`)))
	})).filter((g) => g.goals.length > 0);
}

/** A ação que vem agora no passo a passo: a primeira que ainda falta (-1 se nenhuma). */
export function nextAction(actions: Pick<Action, 'state'>[]): number {
	return actions.findIndex((a) => a.state === 'open');
}

/** "Passo a passo · 5 ações · 3 com o que falta": o resumo do botão que abre a lista a 640. */
export function actionsSummary(actions: Pick<Action, 'state'>[]): string {
	const open = actions.filter((a) => a.state === 'open').length;
	const n = actions.length;
	return [
		`Passo a passo · ${n} ${n === 1 ? 'ação' : 'ações'}`,
		open ? `${open} com o que falta` : ''
	]
		.filter(Boolean)
		.join(' · ');
}
