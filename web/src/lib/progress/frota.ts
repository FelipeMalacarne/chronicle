/*
 * Frota (página 8 do canvas, V7Frota1440 e V7FrotaCompacta1440): uma raia por conta, com o que ela
 * faz agora, onde roda, a rota e o que vem depois; para muitas contas, a lista compacta agrupada.
 * Só o que o servidor já sabe: ritmo de XP, ganhos da sessão e a fila de rotinas do artboard ainda
 * não existem na API. Puro, testado com node --test.
 */
import type { ClientStatus, WorkerCapacity } from '../api/microbot.ts';
import { roomHint, usageLine } from './capacidade.ts';
import { duration } from './relogios.ts';
import type { AccountState, AccountView, ProgressSummary, TimerView } from './types.ts';
import { routeLabel } from './view.ts';
import type { WorkerLine } from './workers.ts';

export type LaneTone = 'ok' | 'up' | 'bad' | 'off';

export interface FleetInput {
	account: AccountView;
	state: AccountState | null;
	summary: ProgressSummary | null;
	timers: TimerView[];
	/** A linha de Workers da conta; null sem sessão guardada ou com o módulo desligado. */
	worker: WorkerLine | null;
	/** O que o agent da conta diz do cliente; null sem agent. */
	client: ClientStatus | null;
	/** Os scripts ligados no cliente, pelo nome. */
	scripts: string[];
}

export interface LaneNext {
	label: string;
	when: string;
	ready: boolean;
}

export interface Lane {
	id: string;
	rsn: string;
	meta: string;
	/** A skill mais alta: o rosto da conta enquanto não se sabe o que ela treina. */
	face?: { skill: string; level: number };
	tone: LaneTone;
	activity: string;
	detail?: string;
	where: string;
	/** O título da etiqueta de onde roda: como roda lá e o uso medido do pod. */
	whereTitle?: string;
	route?: { label: string; ref?: string; done: number; total: number; pct: number };
	next: LaneNext[];
	/** Tem um agent: dá para abrir o cliente desta conta. */
	client: boolean;
	worker: WorkerLine | null;
}

export interface Need {
	account: string;
	rsn: string;
	text: string;
	sub?: string;
	hard: boolean;
	/** O comando que resolve, para copiar (a sessão nova só vem pelo launcher, na CLI). */
	command?: string;
}

/** A atividade de uma conta cujo último login o jogo recusou. */
export const REFUSED = 'Login recusado';

/** O comando que manda a sessão do jogo de novo, no computador com o launcher. */
export function connectCommand(character: string): string {
	return `chronicle accounts connect --character "${character}"`;
}

/**
 * O comando que resolve a sessão de uma conta com worker, quando é ela que falta: o personagem da
 * sessão recusada, ou o nome no jogo quando ainda não há sessão.
 */
export function sessionCommand(l: Lane): string | undefined {
	const w = l.worker;
	if (!w) return undefined;
	if (w.refused && l.tone === 'bad') return connectCommand(w.refused.character);
	if (!w.sessionWhen) return connectCommand(l.rsn);
	return undefined;
}

const MODE: Record<string, string> = {
	ironman: 'ironman',
	hcim: 'hardcore',
	uim: 'ultimate',
	gim: 'group'
};

/** Uma leitura do plugin mais nova que isto quer dizer que a conta está no jogo agora. */
export const PLAYING_MS = 5 * 60_000;

function pluginAge(summary: ProgressSummary | null, now: number): number | undefined {
	const s = summary?.sources.find((x) => x.source === 'plugin');
	return s ? now - Date.parse(s.last_observed_at) : undefined;
}

function ago(ms: number): string {
	return `há ${duration(Math.max(0, ms) / 1000)}`;
}

export function nextUp(timers: TimerView[], now: number, limit = 2): LaneNext[] {
	const sec = now / 1000;
	const out: (LaneNext & { at: number })[] = [];
	for (const t of timers) {
		const label = t.label ?? t.key;
		const due = t.ready_at !== undefined && t.ready_at <= sec;
		if (t.state === 'COMPLETED' || (t.state === 'IN_PROGRESS' && due))
			out.push({ label, when: 'pronto', ready: true, at: 0 });
		else if (t.state === 'IN_PROGRESS' && t.ready_at !== undefined)
			out.push({ label, when: duration(t.ready_at - sec), ready: false, at: t.ready_at });
	}
	return out
		.sort((a, b) => a.at - b.at || a.label.localeCompare(b.label))
		.slice(0, limit)
		.map(({ label, when, ready }) => ({ label, when, ready }));
}

/** Plugins que ficam sempre ligados e não dizem o que a conta está fazendo. */
const UTILITY = /break handler|auto ?login|chronicle link|antiban|quest helper/i;

/** "Tick Fisher", "Tick Fisher +1": o primeiro script que joga e quantos mais. */
export function scriptsLine(scripts: string[]): string {
	const playing = scripts.filter((s) => !UTILITY.test(s));
	if (!playing.length) return '';
	return playing.length > 1 ? `${playing[0]} +${playing.length - 1}` : playing[0];
}

export function lane(input: FleetInput, now = Date.now()): Lane {
	const { account, state, summary, worker, client } = input;
	const total = state?.total_level;
	const top = [...(state?.skills ?? [])].sort((a, b) => b.level - a.level || b.xp - a.xp)[0];
	const active = summary?.routes.find((r) => r.active);
	const out: Lane = {
		id: account.id,
		rsn: account.rsn,
		meta: [MODE[account.mode] ?? account.mode, total ? `${total} total` : '']
			.filter(Boolean)
			.join(' · '),
		face: top && top.level > 1 ? { skill: top.skill, level: top.level } : undefined,
		tone: 'off',
		activity: 'Parada',
		where: worker?.runtime ? worker.host : '',
		whereTitle:
			[worker?.runtime, usageLine(worker?.usage)].filter(Boolean).join(' · ') || undefined,
		route: active
			? {
					label: routeLabel(active.guide, active.title),
					ref: active.cursor_ref,
					done: active.counts.done,
					total: active.counts.total,
					pct: active.counts.total
						? Math.round((100 * active.counts.done) / active.counts.total)
						: 0
				}
			: undefined,
		next: nextUp(input.timers, now),
		client: !!worker?.cockpit,
		worker
	};
	const age = pluginAge(summary, now);
	const fresh = age !== undefined && age < PLAYING_MS;
	const game = client?.state;
	if (worker?.tone === 'warn') {
		out.tone = 'bad';
		out.activity = worker.state;
		out.detail = worker.detail;
	} else if (worker?.refused && !game?.loggedIn && !fresh) {
		// The server's only sign of an expired session: the game refused the last login with it.
		out.tone = 'bad';
		out.activity = REFUSED;
		out.detail = [
			worker.refused.when
				? `o jogo recusou o login ${worker.refused.when}`
				: 'o jogo recusou o último login',
			worker.refused.reason
		]
			.filter(Boolean)
			.join(' · ');
	} else if (worker?.tone === 'busy') {
		out.tone = 'up';
		out.activity = worker.state;
		out.detail = worker.detail;
	} else if (game?.loggedIn || (client?.running && fresh)) {
		out.tone = 'ok';
		out.activity = scriptsLine(input.scripts) || 'No jogo';
		const world = client?.login?.currentWorld;
		out.detail = [world ? `mundo ${world}` : '', game?.scriptsPaused ? 'scripts pausados' : '']
			.filter(Boolean)
			.join(' · ');
	} else if (client?.running) {
		out.tone = 'up';
		out.activity = 'Na tela de login';
		out.detail = client.login?.loginError;
	} else if (fresh) {
		// Sem agent, mas o plugin acabou de mandar: o dono está jogando num cliente que o Chronicle não controla.
		out.tone = 'ok';
		out.activity = 'No jogo';
		out.detail = `à mão · plugin ${ago(age)}`;
	} else {
		out.activity = worker ? 'Parada' : 'Só acompanhada';
		out.detail =
			worker?.blocked && !worker.sessionWhen
				? 'sem sessão do jogo'
				: age !== undefined
					? `plugin ${ago(age)}`
					: 'nenhuma leitura do plugin ainda';
	}
	return out;
}

export function lanes(inputs: FleetInput[], now = Date.now()): Lane[] {
	const rank: Record<LaneTone, number> = { bad: 0, ok: 1, up: 2, off: 3 };
	return inputs
		.map((i) => lane(i, now))
		.sort((a, b) => rank[a.tone] - rank[b.tone] || a.rsn.localeCompare(b.rsn));
}

/**
 * O que pede uma decisão do dono, do mais grave ao mais leve. `cap` dá o número de "falta lugar"
 * (quanto um worker pede e quanto sobra na quota).
 */
export function needs(all: Lane[], cap?: WorkerCapacity): Need[] {
	const out: Need[] = [];
	for (const l of all) {
		const base = { account: l.id, rsn: l.rsn };
		const command = sessionCommand(l);
		if (l.worker?.refused && l.tone === 'bad')
			out.push({
				...base,
				text: 'não entra: o jogo recusou o login',
				sub: 'Abra o launcher no zaros e envie a sessão de novo.',
				hard: true,
				command
			});
		if (l.tone === 'bad' && l.worker?.noRoom)
			out.push({ ...base, text: 'não subiu: falta lugar', sub: roomHint(cap), hard: false });
		else if (l.tone === 'bad' && l.activity !== REFUSED)
			out.push({ ...base, text: l.activity, sub: l.detail, hard: true });
		if (l.worker && !l.worker.sessionWhen)
			out.push({
				...base,
				text: 'não tem sessão do jogo',
				sub: 'Abra o launcher no zaros e envie a sessão.',
				hard: true,
				command
			});
		if (l.worker?.outdated)
			out.push({
				...base,
				text: 'roda numa versão antiga',
				sub: 'Reiniciar sai do jogo.',
				hard: false
			});
		for (const n of l.next)
			if (n.ready) out.push({ ...base, text: `tem ${n.label} prontos`, hard: false });
	}
	return out.sort((a, b) => Number(b.hard) - Number(a.hard));
}

export function fleetSummary(all: Lane[]): string {
	const n = (tone: LaneTone) => all.filter((l) => l.tone === tone).length;
	const parts = [`${all.length} ${all.length === 1 ? 'conta' : 'contas'}`];
	if (n('ok')) parts.push(`${n('ok')} no jogo`);
	if (n('up')) parts.push(`${n('up')} subindo`);
	if (n('bad')) parts.push(`${n('bad')} com problema`);
	return parts.join(' · ');
}

export type GroupKey = 'need' | 'playing' | 'stopped';

export interface CompactGroup {
	key: GroupKey;
	label: string;
	lanes: Lane[];
}

const GROUPS: { key: GroupKey; label: string }[] = [
	{ key: 'need', label: 'Precisa de você' },
	{ key: 'playing', label: 'Jogando' },
	{ key: 'stopped', label: 'Paradas' }
];

function fold(s: string): string {
	return s
		.normalize('NFD')
		.replace(/[\u0300-\u036f]/g, '')
		.toLowerCase()
		.trim();
}

/**
 * A lista compacta (V7FrotaCompacta1440): Precisa de você (com problema ou com um pedido grave),
 * Jogando (no jogo ou subindo) e Paradas, na ordem das raias, filtradas pelo nome. Grupos vazios
 * somem.
 */
export function compactGroups(all: Lane[], attention: Need[], query = ''): CompactGroup[] {
	const urgent = new Set(attention.filter((n) => n.hard).map((n) => n.account));
	const q = fold(query);
	const key = (l: Lane): GroupKey =>
		l.tone === 'bad' || urgent.has(l.id) ? 'need' : l.tone === 'off' ? 'stopped' : 'playing';
	const shown = q ? all.filter((l) => fold(l.rsn).includes(q)) : all;
	return GROUPS.map((g) => ({ ...g, lanes: shown.filter((l) => key(l) === g.key) })).filter(
		(g) => g.lanes.length
	);
}

export type Density = 'raias' | 'compacta';

/** A densidade lembrada neste navegador; qualquer outro valor volta às raias. */
export function parseDensity(saved: string | null | undefined): Density {
	return saved === 'compacta' ? 'compacta' : 'raias';
}
