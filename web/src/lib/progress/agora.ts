import type { EventsView, StepStatus } from './types.ts';
import { ago, type Chip } from './view.ts';

/**
 * Pedaços puros do Agora da v6 (V6Agora1440/640): quem fez os passos anteriores, a linha
 * "Nesta sessão" do rodapé e o chip único da linha de "Em seguida".
 */

const SOURCE_LABEL: Record<string, string> = {
	plugin: 'plugin',
	hiscores: 'hiscores',
	wom: 'wise old man',
	wikisync: 'wikisync'
};

const DONE: StepStatus[] = ['auto_done', 'manual_done', 'implied'];

/** "plugin · 12 min", "você · 9 min" ou "presumido": o último evento que fechou o passo. */
export function whoDid(ev: EventsView | null, stepId: string, now: number): string {
	const last = (ev?.steps ?? [])
		.filter((e) => e.step_id === stepId && DONE.includes(e.to))
		.sort((a, b) => Date.parse(b.at) - Date.parse(a.at))[0];
	if (!last) return '';
	const when = ago(Date.parse(last.at), now).replace('há ', '');
	if (last.to === 'implied') return `presumido · ${when}`;
	if (last.cause === 'user') return `você · ${when}`;
	return `${SOURCE_LABEL[last.source ?? ''] ?? last.source ?? 'fonte'} · ${when}`;
}

/** Janela da "sessão" do rodapé: as últimas horas de jogo. */
export const SESSION_MS = 6 * 3600_000;

/** "+3 passos, +1 level" nas últimas horas, ou "" quando nada mudou. */
export function sessionLine(ev: EventsView | null, now: number): string {
	if (!ev) return '';
	const recent = (at: string) => now - Date.parse(at) <= SESSION_MS;
	const steps = new Set(
		ev.steps.filter((e) => recent(e.at) && DONE.includes(e.to)).map((e) => e.step_id)
	).size;
	const levels = ev.objectives.filter(
		(e) => recent(e.at) && e.to === 'met' && e.key.startsWith('skill:')
	).length;
	const parts: string[] = [];
	if (steps) parts.push(`+${steps} ${steps === 1 ? 'passo' : 'passos'}`);
	if (levels) parts.push(`+${levels} ${levels === 1 ? 'level' : 'levels'}`);
	return parts.join(', ');
}

const RANK: Record<string, number> = { completes: 0, starts: 1, requires: 2, uses: 3 };

/** O chip visível de uma linha (conclui > inicia > requer > usa) e quantos sobram ("+N"). */
export function rowChip(chips: Chip[]): { chip: Chip | null; more: number } {
	if (chips.length === 0) return { chip: null, more: 0 };
	const sorted = [...chips].sort((a, b) => (RANK[a.role] ?? 9) - (RANK[b.role] ?? 9));
	return { chip: sorted[0], more: chips.length - 1 };
}

/** Os `n` primeiros objetivos da linha da rota inteira da v7 (ícones) e quantos sobram ("+N"). */
export function rowChips(chips: Chip[], n: number): { shown: Chip[]; more: number } {
	const sorted = [...chips].sort((a, b) => (RANK[a.role] ?? 9) - (RANK[b.role] ?? 9));
	return { shown: sorted.slice(0, n), more: Math.max(0, chips.length - n) };
}
