/*
 * O ritmo da v7 (fatia V5; artboards V7Frota1440, V7Conta1440): a skill que treina e quando vem o
 * próximo nível, XP por hora, a sessão, o dia, o GP, os itens ganhos e os que vão acabando, e o
 * "Até a pausa" do Break Handler. Só formata o que o servidor mede (`/api/account/rhythm`,
 * `/api/rhythm`, o `break` do status do cliente): sem dado, a linha some. Lógica pura, testada com
 * node --test.
 */

import { toNext } from './conta.ts';
import { skillName } from './historico.ts';
import type { AccountRhythm, FleetRhythm, RhythmItem, RhythmRunOut } from './types.ts';
import { duration } from './relogios.ts';

/** O sinal de menos da tipografia, como no artboard ("−1,1k"). */
const MINUS = '−';

/** "412", "1,3k", "38,2k", "412k", "6,1M", "1 204M": uma casa decimal até três dígitos inteiros. */
export function compact(n: number): string {
	const a = Math.abs(n);
	const sign = n < 0 ? MINUS : '';
	const one = (v: number, unit: string) =>
		`${sign}${v < 100 ? (Math.floor(v * 10) / 10).toLocaleString('pt-BR') : Math.floor(v).toLocaleString('pt-BR')}${unit}`;
	if (a >= 1_000_000) return one(a / 1_000_000, 'M');
	if (a >= 1000) return one(a / 1000, 'k');
	return `${sign}${Math.floor(a)}`;
}

/** "+84k", "−1,1k"; zero sem sinal. */
export function signed(n: number): string {
	return n > 0 ? `+${compact(n)}` : compact(n);
}

/** "38,2k xp/h"; nada quando a janela foi curta demais para medir. */
export function xpPerHour(r: AccountRhythm): string | null {
	const t = r.training;
	return t && t.xp_per_hour > 0 ? `${compact(t.xp_per_hour)} xp/h` : null;
}

/** O próximo nível da skill que treina: "77 em 22 min", "69 em 1 h 50"; nada no 99 ou sem ritmo. */
export function nextLevel(r: AccountRhythm): string | null {
	const t = r.training;
	if (!t || t.next_level_in_s === undefined) return null;
	return `${t.level + 1} em ${duration(t.next_level_in_s)}`;
}

/** "26,7k xp" da sessão; nada sem sessão. */
export function sessionXp(r: AccountRhythm): string | null {
	return r.session ? `${compact(r.session.xp)} xp` : null;
}

/** O nome que a tela mostra para um item, ou o id quando o catálogo não o conhece. */
export function itemName(i: { item: number; name?: string }): string {
	return i.name ?? `item ${i.item}`;
}

/** Os itens que a sessão rendeu, os `n` maiores; vazio quando nada veio ou não se sabe. */
export function gained(r: AccountRhythm, n = 3): RhythmItem[] {
	return (r.session?.gained ?? []).slice(0, n);
}

/** O item que acaba primeiro: "Feather acabam em 3 h 05"; nada quando nada se gasta. */
export function runningOut(r: AccountRhythm): { item: RhythmRunOut; text: string } | null {
	const first = r.session?.running_out?.[0];
	if (!first) return null;
	return { item: first, text: `${itemName(first)} acabam em ${duration(first.in_s)}` };
}

/** O GP de hoje: "+22k"; nada quando o começo do dia não foi visto. */
export function gpToday(r: AccountRhythm): string | null {
	return r.gp?.today !== undefined ? signed(r.gp.today) : null;
}

/** O break do Break Handler, como o status do cliente o traz (Chronicle Link 1.12+). */
export interface BreakInfo {
	active: boolean;
	in_s?: number;
	left_s?: number;
}

/**
 * "Até a pausa": quanto falta para o próximo break, ou, num break, quanto falta para voltar. Nada
 * quando o cliente não diz (ponte antiga, Break Handler desligado ou V2).
 */
export function untilBreak(
	b: BreakInfo | null | undefined
): { label: string; text: string } | null {
	if (!b) return null;
	if (b.active) {
		return b.left_s !== undefined
			? { label: 'Em pausa', text: `volta em ${duration(b.left_s)}` }
			: null;
	}
	return b.in_s !== undefined ? { label: 'Até a pausa', text: duration(b.in_s) } : null;
}

/** O resumo da frota: "412k" de XP hoje com "7 níveis · 3 contas", e o GP com o de hoje. */
export function fleetSummary(f: FleetRhythm): {
	xp: string;
	xpSub: string;
	gp: string | null;
	gpSub: string | null;
} {
	const t = f.totals;
	const levels =
		t.levels_today === 0
			? 'nenhum nível'
			: `${t.levels_today} ${t.levels_today === 1 ? 'nível' : 'níveis'}`;
	const parts = [levels, `${t.gaining} ${t.gaining === 1 ? 'conta' : 'contas'}`];
	return {
		xp: compact(t.xp_today),
		xpSub: t.gaining === 0 ? 'nada ainda hoje' : parts.join(' · '),
		gp: t.gp !== undefined ? compact(t.gp) : null,
		gpSub: t.gp_today !== undefined ? `${signed(t.gp_today)} hoje` : null
	};
}

/** A meia-noite de hoje no fuso do navegador, o `since` das rotas do ritmo. */
export function todaySince(now: Date): string {
	const d = new Date(now);
	d.setHours(0, 0, 0, 0);
	return d.toISOString();
}

/** A skill que treina, no quadro e na barra: o nível, o caminho até o próximo (0 a 100) e quando vem. */
export interface TrainingBar {
	skill: string;
	/** "Fishing", como o jogo escreve. */
	name: string;
	level: number;
	pct: number;
	/** "77 em 22 min"; null no 99 ou sem ritmo. */
	next: string | null;
}

/** A barra da skill que treina; nada quando nenhuma ganhou XP nos últimos minutos. */
export function trainingBar(r: AccountRhythm | null | undefined): TrainingBar | null {
	const t = r?.training;
	if (!t) return null;
	return {
		skill: t.skill,
		name: skillName(t.skill),
		level: t.level,
		pct: Math.round(100 * toNext(t.level, t.xp)),
		next: nextLevel(r)
	};
}

/** Um item na linha do ritmo: o sprite pelo id, a quantidade curta e o que ela quer dizer no título. */
export interface RhythmChip {
	item: number;
	name: string;
	qty: string;
	title: string;
}

function gainedChip(i: RhythmItem): RhythmChip {
	return {
		item: i.item,
		name: itemName(i),
		qty: compact(i.qty),
		title: `${itemName(i)}: +${i.qty.toLocaleString('pt-BR')} nesta sessão`
	};
}

function outChip(o: RhythmRunOut): RhythmChip {
	return {
		item: o.item,
		name: itemName(o),
		qty: compact(-o.used),
		title: `${itemName(o)}: ${o.used.toLocaleString('pt-BR')} gastos nesta sessão, sobram ${o.left.toLocaleString('pt-BR')}; acabam em ${duration(o.in_s)} neste ritmo`
	};
}

/** O ritmo de uma raia da Frota (V7Frota1440): a barra da skill, XP por hora, o que ganhou e o que gasta. */
export interface LaneRhythm {
	bar: TrainingBar | null;
	rate: string | null;
	gained: RhythmChip[];
	/** O que acaba primeiro, com o que a sessão já gastou ("−1,1k"). */
	out: RhythmChip | null;
}

/**
 * O ritmo da raia, só enquanto a conta joga (ou ganhou XP há pouco): numa conta parada, a sessão é a
 * última, não a de agora. Nada quando não há o que mostrar.
 */
export function laneRhythm(
	r: AccountRhythm | null | undefined,
	playing: boolean,
	n = 3
): LaneRhythm | null {
	if (!r || (!playing && !r.training)) return null;
	const first = r.session?.running_out?.[0];
	const out: LaneRhythm = {
		bar: trainingBar(r),
		rate: xpPerHour(r),
		gained: gained(r, n).map(gainedChip),
		out: first ? outChip(first) : null
	};
	return out.bar || out.rate || out.gained.length || out.out ? out : null;
}

/** Uma célula de "Fazendo agora" (V7Conta1440): XP por hora, Nesta sessão, Até a pausa. */
export interface NowStat {
	k: string;
	v: string;
}

/** As células com dado; a que não tem some. A pausa só vem do cliente ligado. */
export function nowStats(r: AccountRhythm | null | undefined, b?: BreakInfo | null): NowStat[] {
	const out: NowStat[] = [];
	const rate = r?.training?.xp_per_hour;
	if (rate && rate > 0) out.push({ k: 'XP por hora', v: compact(rate) });
	const session = r ? sessionXp(r) : null;
	if (session) out.push({ k: 'Nesta sessão', v: session });
	const pause = untilBreak(b);
	if (pause) out.push({ k: pause.label, v: pause.text });
	return out;
}

/** "Ganhou nesta sessão": os itens, e por último o que acaba primeiro. */
export function sessionItems(
	r: AccountRhythm | null | undefined,
	n = 3
): { gained: RhythmChip[]; out: (RhythmChip & { text: string }) | null } {
	if (!r) return { gained: [], out: null };
	const first = r.session?.running_out?.[0];
	const ro = runningOut(r);
	return {
		gained: gained(r, n).map(gainedChip),
		out: first && ro ? { ...outChip(first), text: ro.text } : null
	};
}

/** Uma linha do cartão Hoje: a skill, "74 → 76" quando subiu (o nível, quando não) e o XP do dia. */
export interface DayRow {
	skill: string;
	name: string;
	up: boolean;
	levels: string;
	xp: string;
}

/**
 * O cartão Hoje (V7Conta1440): as skills que ganharam XP desde a meia-noite, da que mais ganhou, até
 * `max`; o resto vira "mais N". O GP com o ganho do dia, quando o banco foi visto.
 */
export function dayCard(
	r: AccountRhythm | null | undefined,
	max = 6
): {
	rows: DayRow[];
	more: { n: number; xp: string } | null;
	gp: { value: string; today: string | null } | null;
} {
	const skills = (r?.today.skills ?? []).filter((s) => s.xp > 0);
	const rows = skills.slice(0, max).map((s) => ({
		skill: s.skill,
		name: skillName(s.skill),
		up: s.levels > 0,
		levels: s.levels > 0 ? `${s.level - s.levels} → ${s.level}` : String(s.level),
		xp: `+${compact(s.xp)}`
	}));
	const rest = skills.slice(max);
	return {
		rows,
		more: rest.length
			? { n: rest.length, xp: `+${compact(rest.reduce((a, s) => a + s.xp, 0))}` }
			: null,
		gp: r?.gp ? { value: compact(r.gp.value), today: gpToday(r) } : null
	};
}

/**
 * O que subiu hoje, para o Personagem (V7Personagem1440): os níveis ganhos por skill e a frase
 * "Fishing +2, Farming +1"; nada quando nada subiu.
 */
export function upToday(r: AccountRhythm | null | undefined): {
	levels: Record<string, number>;
	text: string | null;
} {
	const up = (r?.today.skills ?? []).filter((s) => s.levels > 0);
	return {
		levels: Object.fromEntries(up.map((s) => [s.skill, s.levels])),
		text: up.length ? up.map((s) => `${skillName(s.skill)} +${s.levels}`).join(', ') : null
	};
}
