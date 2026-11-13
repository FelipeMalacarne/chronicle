/*
 * Atividade (página 8 do canvas, V7Atividade1440): o que pede uma decisão do dono e o que a frota
 * fez sozinha. As execuções de hoje de todas as contas, o XP de hoje por conta e o Diário dos níveis.
 * Puro, testado com node --test.
 */
import type { Run } from '../api/microbot.ts';
import { compact } from './ritmo.ts';
import { hhmm, runItem, skillName, type ResultTone } from './historico.ts';
import type { AccountView, FleetRhythm, LevelEvent } from './types.ts';

export interface FleetRunRow {
	key: string;
	at: number;
	time: string;
	account: string;
	rsn: string;
	name: string;
	/** A skill que o run subiu (o sprite); sem ela, o glifo. */
	skill?: string;
	/** "você iniciou", "filhote cresceu · tempo esgotado". */
	sub: string;
	span: string;
	/** "feito", "falhou", "rodando". */
	end: { label: string; tone: ResultTone };
}

/**
 * As execuções de todas as contas desde `since`, da mais nova à mais antiga; um run que começou antes
 * e ainda roda também entra. `levels` são os eventos de nível da conta, para dizer o que ela subiu
 * durante cada run.
 */
export function fleetRuns(
	per: { account: string; rsn: string; runs: Run[]; levels?: LevelEvent[] }[],
	since: number,
	now: number
): FleetRunRow[] {
	return per
		.flatMap(({ account, rsn, runs, levels }) =>
			runs.map((r) => {
				const i = runItem(r, now, levels);
				return {
					key: `${account}:${i.key}`,
					at: i.at,
					time: i.time,
					account,
					rsn,
					name: i.title,
					skill: i.skill,
					sub: i.sub,
					span: i.span ?? '',
					end: i.result ?? { label: '', tone: 'going' as const }
				};
			})
		)
		.filter((r) => Number.isFinite(r.at) && (r.at >= since || r.end.tone === 'going'))
		.sort((a, b) => b.at - a.at || a.key.localeCompare(b.key));
}

/** "hoje · 11 feitas · 1 falhou"; sem execução, só "hoje". */
export function runsHeader(rows: FleetRunRow[]): string {
	const done = rows.filter((r) => r.end.tone === 'ok').length;
	const failed = rows.filter((r) => r.end.tone === 'bad').length;
	const going = rows.filter((r) => r.end.tone === 'going').length;
	return [
		'hoje',
		done ? `${done} ${done === 1 ? 'feita' : 'feitas'}` : '',
		failed ? `${failed} ${failed === 1 ? 'falhou' : 'falharam'}` : '',
		going ? `${going} rodando` : ''
	]
		.filter(Boolean)
		.join(' · ');
}

export interface XpRow {
	account: string;
	rsn: string;
	xp: number;
	/** "114k". */
	label: string;
	/** A barra, relativa à conta que mais ganhou. */
	pct: number;
}

/**
 * O XP de hoje por conta, de quem mais ganhou para quem menos. Conta sem XP hoje fica de fora (sem
 * dado, a linha some). `only` restringe a uma conta.
 */
export function xpRows(
	rh: FleetRhythm | null,
	names: Record<string, string>,
	only?: string
): { rows: XpRow[]; total: number } {
	const list = (rh?.accounts ?? []).filter((a) => a.today.xp > 0 && (!only || a.account === only));
	const top = Math.max(0, ...list.map((a) => a.today.xp));
	const rows = list
		.map((a) => ({
			account: a.account,
			rsn: names[a.account] ?? a.account,
			xp: a.today.xp,
			label: compact(a.today.xp),
			pct: top ? Math.max(2, Math.round((100 * a.today.xp) / top)) : 0
		}))
		.sort((a, b) => b.xp - a.xp || a.rsn.localeCompare(b.rsn));
	return { rows, total: rows.reduce((s, r) => s + r.xp, 0) };
}

export interface DiaryRow {
	key: string;
	time: string;
	account: string;
	rsn: string;
	skill: string;
	/** "subiu para Fishing 76". */
	text: string;
}

/** O Diário: os níveis das contas, do mais novo ao mais antigo, como o servidor manda. */
export function diary(
	events: LevelEvent[],
	names: Record<string, string>,
	only?: string
): DiaryRow[] {
	return events
		.filter((e) => !only || e.account === only)
		.map((e) => ({
			key: `${e.account}:${e.skill}:${e.level}`,
			time: hhmm(Date.parse(e.at)),
			account: e.account,
			rsn: names[e.account] ?? e.account,
			skill: e.skill,
			text: `subiu para ${skillName(e.skill)} ${e.level}`
		}));
}

/**
 * As pílulas de conta do topo: todas até `max`; com mais, as `max - 1` primeiras e um "+N" que abre
 * o resto. A escolhida nunca some atrás do "+N".
 */
export function accountPills(
	accounts: AccountView[],
	chosen: string | null,
	open: boolean,
	max = 4
): { shown: AccountView[]; more: number } {
	if (open || accounts.length <= max) return { shown: accounts, more: 0 };
	const shown = accounts.slice(0, max - 1);
	const pick = accounts.find((a) => a.id === chosen);
	if (pick && !shown.includes(pick)) shown[shown.length - 1] = pick;
	return { shown, more: accounts.length - shown.length };
}
