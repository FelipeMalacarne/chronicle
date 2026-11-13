/*
 * Conta › Histórico (página 8 do canvas, V7Historico1440): o que a conta fez nos últimos 7 dias,
 * por dia. Junta as execuções (os runs guardados no servidor), os níveis (derivados dos snapshots)
 * e as ações do dono (a auditoria do Microbot). Os mesmos pedaços servem à Atividade da frota.
 * Puro, testado com node --test.
 */
import type { AuditEntry, Run } from '../api/microbot.ts';
import { auditLine } from './automacao.ts';
import { compact } from './ritmo.ts';
import { RUN_LABEL, runFinal, runOutcome } from './fazer.ts';
import { duration } from './relogios.ts';
import type { LevelEvent, SkillGain } from './types.ts';

/** Quantos dias o Histórico mostra (o servidor dá os níveis de até 7 dias atrás). */
export const HISTORY_DAYS = 7;
/** Quantos runs o servidor devolve por conta, os mais novos (internal/httpapi, runPage). */
export const RUN_PAGE = 30;

export type FeedKind = 'run' | 'level' | 'action';
export type FeedFilter = 'all' | 'runs' | 'levels' | 'mine' | 'problems';
export type ResultTone = 'ok' | 'bad' | 'off' | 'going';

export interface FeedItem {
	key: string;
	kind: FeedKind;
	/** Quando aconteceu, em ms. */
	at: number;
	/** "14:06", na hora do navegador. */
	time: string;
	/** O sprite de uma skill (a do nível, ou a que o run subiu); sem ela, um glifo. */
	skill?: string;
	/** O nível novo, num nível: dois no mesmo instante ficam do maior para o menor. */
	level?: number;
	glyph?: 'sync' | 'conta' | 'porta';
	/** "Fishing 76", "Cat Swap", "Você entrou no mundo 370". */
	title: string;
	/** "subiu de nível", "você iniciou · +100 death runes". */
	sub: string;
	/** Num run que não terminou: "ainda rodando". */
	live?: string;
	/** Quanto o run levou (ou leva até agora). */
	span?: string;
	result?: { label: string; tone: ResultTone };
	/** Run que falhou ou ação recusada. */
	problem: boolean;
	/** Ação que o dono fez pela web. */
	mine: boolean;
	/** A trilha de um run: o que o agent fez, linha a linha. */
	trail?: { time: string; text: string }[];
}

export interface FeedDay {
	key: string;
	/** "Hoje", "Ontem", "Terça, 10/11". */
	label: string;
	/** "114k xp · 3 execuções · 1 problema": do dia inteiro, com ou sem filtro. */
	summary: string;
	/** Só os que passam no filtro. */
	items: FeedItem[];
}

const pad = (n: number) => String(n).padStart(2, '0');

/** "14:06" na hora local. */
export function hhmm(at: number): string {
	const d = new Date(at);
	return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** A meia-noite local de `back` dias antes de `now`. */
export function dayStart(now: number, back = 0): number {
	const d = new Date(now);
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() - back).getTime();
}

/** O começo da janela de `days` dias, hoje incluído: a meia-noite de `days - 1` dias atrás. */
export function windowStart(now: number, days = HISTORY_DAYS): number {
	return dayStart(now, days - 1);
}

const WEEKDAY = ['Domingo', 'Segunda', 'Terça', 'Quarta', 'Quinta', 'Sexta', 'Sábado'];

/** "Hoje", "Ontem" ou "Terça, 10/11", pelo dia local. */
export function dayLabel(at: number, now: number): string {
	const diff = Math.round((dayStart(now) - dayStart(at)) / 86_400_000);
	if (diff === 0) return 'Hoje';
	if (diff === 1) return 'Ontem';
	const d = new Date(at);
	return `${WEEKDAY[d.getDay()]}, ${pad(d.getDate())}/${pad(d.getMonth() + 1)}`;
}

/** "fishing" → "Fishing": as skills ficam em inglês. */
export const skillName = (skill: string) => skill.charAt(0).toUpperCase() + skill.slice(1);

/** "net.runelite…fm.catswap.CatSwapPlugin" → "Cat Swap". */
export function runName(className: string): string {
	const last = className.split('.').pop() ?? className;
	const bare = last.replace(/Plugin$/, '') || last;
	return bare.replace(/([a-z0-9])([A-Z])/g, '$1 $2');
}

/** O evento do jogo que disparou o run, como a tela diz. */
const TRIGGER_LABEL: Record<string, string> = { kitten_grown: 'filhote cresceu' };

/**
 * Quem começou o run: uma rotina recorrente (pelo relógio do jogo ou pelo intervalo; o run não diz
 * qual), um evento do jogo ou o dono.
 */
function runWho(r: Run): string {
	if (r.routine) return 'pela rotina';
	if (r.trigger) return TRIGGER_LABEL[r.trigger] ?? 'sozinho';
	return 'você iniciou';
}

/**
 * As skills que subiram enquanto o run rodava (do começo ao fim, ou até agora), pelos eventos de nível
 * da mesma conta, a que mais subiu primeiro: o nível antes e o último.
 */
function levelsDuring(
	r: Run,
	events: LevelEvent[],
	now: number
): { skill: string; from: number; to: number }[] {
	const start = Date.parse(r.startedAt);
	const end = r.finishedAt ? Date.parse(r.finishedAt) : now;
	const by = new Map<string, { lo: number; hi: number }>();
	for (const e of events) {
		const at = Date.parse(e.at);
		if (!(at >= start && at <= end)) continue;
		const s = by.get(e.skill);
		by.set(
			e.skill,
			s
				? { lo: Math.min(s.lo, e.level), hi: Math.max(s.hi, e.level) }
				: { lo: e.level, hi: e.level }
		);
	}
	return [...by.entries()]
		.map(([skill, s]) => ({ skill, from: s.lo - 1, to: s.hi }))
		.sort((a, b) => b.to - b.from - (a.to - a.from) || a.skill.localeCompare(b.skill));
}

/** Os níveis que a conta subiu durante o run: "Fishing 74 → 75"; no máximo duas skills. */
export function runLevels(r: Run, events: LevelEvent[], now: number): string {
	return levelsDuring(r, events, now)
		.slice(0, 2)
		.map((l) => `${skillName(l.skill)} ${l.from} → ${l.to}`)
		.join(' · ');
}

/** Quanto o run levou; sem fim, quanto leva até agora. */
export function runSpan(r: Run, now: number): string {
	const end = r.finishedAt ? Date.parse(r.finishedAt) : now;
	return duration(Math.max(0, end - Date.parse(r.startedAt)) / 1000);
}

function runResult(r: Run): { label: string; tone: ResultTone } {
	const tone: ResultTone = !runFinal(r)
		? 'going'
		: r.state === 'done'
			? 'ok'
			: r.state === 'failed'
				? 'bad'
				: 'off';
	return { label: RUN_LABEL[r.state] ?? r.state, tone };
}

/**
 * Uma execução: "Cat Swap · filhote cresceu · +100 death runes", "7 min", "feito". `levels` são os
 * eventos de nível da conta do run, para dizer o que ela subiu enquanto ele rodava.
 */
export function runItem(r: Run, now: number, levels: LevelEvent[] = []): FeedItem {
	const at = Date.parse(r.startedAt);
	const final = runFinal(r);
	const detail = !final ? '' : r.state === 'failed' ? (r.error ?? '') : runOutcome(r);
	return {
		key: `run:${r.id}@${r.startedAt}`,
		kind: 'run',
		at,
		time: hhmm(at),
		// O sprite da skill que o run subiu; sem nível no caminho, o glifo.
		skill: levelsDuring(r, levels, now)[0]?.skill,
		glyph: 'sync',
		title: runName(r.className),
		sub: [runWho(r), detail, runLevels(r, levels, now)].filter(Boolean).join(' · '),
		live: final ? undefined : `ainda ${RUN_LABEL[r.state] ?? r.state}`,
		span: runSpan(r, now),
		result: runResult(r),
		problem: r.state === 'failed',
		mine: false,
		trail: (r.events ?? []).map((e) => ({ time: hhmm(Date.parse(e.at)), text: e.text }))
	};
}

/** Um nível: "Fishing 76 · subiu de nível". */
export function levelItem(e: LevelEvent): FeedItem {
	const at = Date.parse(e.at);
	return {
		key: `level:${e.account}:${e.skill}:${e.level}`,
		kind: 'level',
		at,
		time: hhmm(at),
		skill: e.skill,
		level: e.level,
		title: `${skillName(e.skill)} ${e.level}`,
		sub: 'subiu de nível',
		problem: false,
		mine: false
	};
}

const str = (v: unknown) => (typeof v === 'string' ? v : '');
const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** O que a linha diz de uma ação da web, em frase: [título, detalhe]. */
function sentence(e: AuditEntry): [string, string] {
	const p = e.params ?? {};
	const world = typeof p.world === 'number' ? p.world : undefined;
	const name = str(p.className) ? runName(str(p.className)) : str(p.name);
	switch (e.action) {
		case 'worker_start':
			return ['Worker iniciado', 'você iniciou'];
		case 'worker_stop':
			return ['Worker parado', 'você parou · saiu do jogo'];
		case 'worker_restart':
			return ['Worker reiniciado', 'você reiniciou · saiu do jogo'];
		case 'session_put':
			return ['Sessão do jogo guardada', 'você enviou'];
		case 'session_delete':
			return ['Sessão do jogo apagada', 'você apagou'];
		case 'client_start':
			return ['Cliente aberto', 'pelo Cliente'];
		case 'client_stop':
			return ['Cliente fechado', 'pelo Cliente'];
		case 'login':
			return [world ? `Você entrou no mundo ${world}` : 'Você entrou no jogo', 'pelo Cliente'];
		case 'logout':
			return ['Você saiu do jogo', 'pelo Cliente'];
		case 'world_hop':
			return [world ? `Você trocou para o mundo ${world}` : 'Você trocou de mundo', 'pelo Cliente'];
		case 'script_start':
			return [`Você ligou ${name || 'um plugin'}`, 'pelo Cliente'];
		case 'script_stop':
			return [`Você desligou ${name || 'um plugin'}`, 'pelo Cliente'];
		case 'scripts_pause':
			return ['Você pausou os scripts', 'pelo Cliente'];
		case 'scripts_resume':
			return ['Você retomou os scripts', 'pelo Cliente'];
		case 'plugin_install':
			return [`Você instalou ${[name, str(p.version)].filter(Boolean).join(' ')}`, 'pelo Cliente'];
		case 'plugin_remove':
			return [`Você removeu ${name}`, 'pelo Cliente'];
		case 'config_set':
			return [
				`Você mudou ${str(p.group)}.${str(p.key)}`,
				p.value === null || p.value === undefined ? 'voltou ao padrão' : String(p.value)
			];
		case 'vnc_open':
			return ['Você abriu o jogo ao vivo', 'pelo Cliente'];
		case 'trigger_set':
			return [
				`Gatilho ${p.enabled ? 'ligado' : 'desligado'}`,
				p.event === 'kitten_grown' ? 'quando o filhote crescer' : str(p.event)
			];
		case 'run_start':
			return [`Você pediu ${name || 'um fazer agora'}`, 'não começou'];
		case 'run_cancel':
			return ['Você cancelou o fazer agora', 'pelo Cliente'];
		case 'account_use':
			return ['Você trocou o personagem', 'pelo Cliente'];
		case 'recurring_set':
			return [
				name ? `Você gravou a rotina ${name}` : 'Você gravou uma rotina',
				p.enabled ? 'ligada' : 'desligada'
			];
		case 'recurring_delete':
			return ['Você apagou uma rotina', ''];
		case 'profile_save': {
			const n = Array.isArray(p.keys) ? p.keys.length : 0;
			return [
				`Você guardou ${n ? plural(n, 'ajuste', 'ajustes') : 'ajustes'} do cliente no perfil`,
				p.to === 'account' ? 'só desta conta' : ''
			];
		}
		case 'profile_apply':
			return ['Você aplicou o perfil no cliente', ''];
	}
	const line = auditLine({ ...e, ok: true }).replace(/ · ok$/, '');
	return [line.charAt(0).toUpperCase() + line.slice(1), 'pelo Cliente'];
}

const WORKER = /^(worker|session)_/;

/**
 * Uma ação da auditoria. As que repetem uma execução (o começo e o fim de um run, que já aparece
 * como execução) ficam de fora: null.
 */
export function actionItem(e: AuditEntry): FeedItem | null {
	if (e.action === 'run_end') return null;
	if ((e.action === 'run_start' || e.action === 'run_cancel') && e.ok) return null;
	const at = Date.parse(e.at);
	const mine = e.origin === 'web';
	let [title, sub] = sentence(e);
	if (!mine) {
		const line = auditLine({ ...e, ok: true }).replace(/ · ok$/, '');
		[title, sub] = [line.charAt(0).toUpperCase() + line.slice(1), 'sozinho'];
	}
	return {
		key: `audit:${e.id}`,
		kind: 'action',
		at,
		time: hhmm(at),
		glyph: WORKER.test(e.action) ? 'porta' : 'conta',
		title,
		sub: e.ok ? sub : [sub, e.error].filter(Boolean).join(' · '),
		result: e.ok ? undefined : { label: 'falhou', tone: 'bad' },
		problem: !e.ok,
		mine
	};
}

export interface FeedInput {
	runs: Run[];
	levels: LevelEvent[];
	audit: AuditEntry[];
}

/** Tudo da janela (desde `since`), do mais novo ao mais antigo. */
export function feedItems(input: FeedInput, since: number, now: number): FeedItem[] {
	const all = [
		...input.runs.map((r) => runItem(r, now, input.levels)),
		...input.levels.map(levelItem),
		...input.audit.map(actionItem).filter((i): i is FeedItem => i !== null)
	];
	return all
		.filter((i) => Number.isFinite(i.at) && i.at >= since)
		.sort((a, b) => b.at - a.at || (b.level ?? 0) - (a.level ?? 0));
}

export function matches(i: FeedItem, f: FeedFilter): boolean {
	switch (f) {
		case 'runs':
			return i.kind === 'run';
		case 'levels':
			return i.kind === 'level';
		case 'mine':
			return i.mine;
		case 'problems':
			return i.problem;
		default:
			return true;
	}
}

export function counts(items: FeedItem[]): Record<FeedFilter, number> {
	const out: Record<FeedFilter, number> = { all: 0, runs: 0, levels: 0, mine: 0, problems: 0 };
	for (const i of items)
		for (const f of Object.keys(out) as FeedFilter[]) if (matches(i, f)) out[f] += 1;
	return out;
}

/**
 * O resumo do dia: o XP (só hoje, que é o que o ritmo mede; nos outros dias, os níveis), as execuções
 * e os problemas. Sem nada, vazio.
 */
export function daySummary(items: FeedItem[], xp?: number | null): string {
	const n = (k: FeedKind) => items.filter((i) => i.kind === k).length;
	const problems = items.filter((i) => i.problem).length;
	return [
		xp ? `${compact(xp)} xp` : n('level') ? plural(n('level'), 'nível', 'níveis') : '',
		n('run') ? plural(n('run'), 'execução', 'execuções') : '',
		problems ? plural(problems, 'problema', 'problemas') : ''
	]
		.filter(Boolean)
		.join(' · ');
}

/**
 * Os dias com algo que passa no filtro, do mais novo ao mais antigo. `xpToday` é o XP de hoje
 * pelo ritmo (null sem ele).
 */
export function feedDays(
	items: FeedItem[],
	filter: FeedFilter,
	now: number,
	xpToday: number | null = null
): FeedDay[] {
	const today = dayStart(now);
	const byDay = new Map<number, FeedItem[]>();
	for (const i of items) {
		const d = dayStart(i.at);
		byDay.set(d, [...(byDay.get(d) ?? []), i]);
	}
	return [...byDay.entries()]
		.sort((a, b) => b[0] - a[0])
		.map(([d, all]) => {
			const date = new Date(d);
			return {
				key: `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
				label: dayLabel(d, now),
				summary: daySummary(all, d === today ? xpToday : null),
				items: all.filter((i) => matches(i, filter))
			};
		})
		.filter((d) => d.items.length > 0);
}

export interface WeekRuns {
	done: number;
	failed: number;
	canceled: number;
	/** O mais novo que falhou: "Cat Swap", "tempo esgotado". */
	lastFailed: { name: string; error: string } | null;
	/** O servidor só devolve os RUN_PAGE mais novos e todos estão na janela: pode haver mais. */
	partial: boolean;
}

/** As execuções da janela, contadas (o cartão Execuções). */
export function weekRuns(runs: Run[], since: number): WeekRuns {
	const inside = runs.filter((r) => Date.parse(r.startedAt) >= since);
	const failed = inside.filter((r) => r.state === 'failed');
	const last = failed.sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt))[0];
	return {
		done: inside.filter((r) => r.state === 'done').length,
		failed: failed.length,
		canceled: inside.filter((r) => r.state === 'canceled').length,
		lastFailed: last ? { name: runName(last.className), error: last.error ?? '' } : null,
		partial: runs.length >= RUN_PAGE && inside.length === runs.length
	};
}

export interface GainRow {
	skill: string;
	name: string;
	/** "71 → 76", ou só "76" quando o nível não mudou. */
	levels: string;
	up: boolean;
	/** "+412k". */
	xp: string;
}

/** O cartão Na semana: o avanço de cada skill, a que mais ganhou primeiro (como o servidor manda). */
export function weekGains(skills: SkillGain[]): GainRow[] {
	return skills
		.filter((s) => s.xp > 0)
		.map((s) => ({
			skill: s.skill,
			name: skillName(s.skill),
			levels: s.to_level > s.from_level ? `${s.from_level} → ${s.to_level}` : String(s.to_level),
			up: s.to_level > s.from_level,
			xp: `+${compact(s.xp)}`
		}));
}
