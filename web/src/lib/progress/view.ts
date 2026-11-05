/**
 * View-model das telas v4 (Agora, Rotas, Rota) sobre o progresso real. Puro: sem fetch nem
 * runes, testado com `node --test`. Segue os artboards V4AgoraOziris, V4AgoraBruh, V4Rotas,
 * V4RotaOziris, V4RotaBruh e V4Vazios do canvas.
 */
import type {
	BoardObjective,
	BoardStep,
	ContentSection,
	ContentStep,
	Coverage,
	EventsView,
	ItemRequirement,
	Loadout,
	ReadinessItem,
	Role,
	SegmentReadiness,
	StepStatus,
	SyncRun,
	Verdict
} from './types.ts';

/* ------------------------------------------------------------------ chaves de objetivo */

export interface ParsedKey {
	/** Ref da entidade ("quest:x_marks_the_spot"); vazio para quest points. */
	ref: string;
	kind: string;
	id: string;
	op: '@' | '>=' | 'x';
	state?: string;
	n?: number;
}

/** Lê a chave canônica do doc 09, seção 3.2 (`quest:x@finished`, `skill:agility>=70`, `item:314x120`, `qp>=107`). */
export function parseKey(key: string): ParsedKey | null {
	if (key.startsWith('qp>=')) {
		const n = Number(key.slice(4));
		return Number.isInteger(n) ? { ref: '', kind: 'qp', id: '', op: '>=', n } : null;
	}
	const at = key.indexOf('@');
	if (at > 0) return withRef(key.slice(0, at), { op: '@', state: key.slice(at + 1) });
	const ge = key.indexOf('>=');
	if (ge > 0) return withRef(key.slice(0, ge), { op: '>=', n: Number(key.slice(ge + 2)) });
	if (key.startsWith('item:')) {
		const x = key.lastIndexOf('x');
		if (x > 5) return withRef(key.slice(0, x), { op: 'x', n: Number(key.slice(x + 1)) });
	}
	return null;
}

function withRef(ref: string, rest: Pick<ParsedKey, 'op' | 'state' | 'n'>): ParsedKey | null {
	const colon = ref.indexOf(':');
	if (colon <= 0) return null;
	return { ref, kind: ref.slice(0, colon), id: ref.slice(colon + 1), ...rest };
}

export const ROLE_LABEL: Record<Role, string> = {
	completes: 'conclui',
	starts: 'inicia',
	requires: 'requer',
	uses: 'usa'
};

function titleCase(id: string): string {
	return id.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Nome de exibição do alvo: "35 Woodcutting", "Monk's Friend", "120 Feather", "107 quest points". */
export function objectiveLabel(key: string, names: Record<string, string>): string {
	const k = parseKey(key);
	if (!k) return key;
	if (k.kind === 'qp') return `${k.n} quest points`;
	const name = names[k.ref] ?? titleCase(k.id);
	switch (k.kind) {
		case 'skill':
		case 'boss':
			return `${k.n} ${name}`;
		case 'item':
			return (k.n ?? 1) > 1 ? `${k.n} ${name}` : name;
		default:
			return name;
	}
}

export interface VerdictView {
	/** Classe da bolinha: v-met, v-unmet, v-unk. */
	cls: string;
	glyph: string;
	/** Chip pontilhado quando não sabemos (unknown nunca é unmet, decisão 6 do v4). */
	unknown: boolean;
	/** Texto curto ao lado do nome. */
	hint: string;
}

const UNKNOWN_HINT: Record<string, string> = {
	quest: 'sem faceta de quest',
	item: 'sem container',
	skill: 'sem leitura',
	diary: 'sem faceta de diary',
	boss: 'sem hiscores',
	unlock: 'sem fonte',
	ca: 'sem fonte',
	qp: 'sem fonte'
};

export function verdictView(o: Pick<BoardObjective, 'key' | 'verdict' | 'regressed'>): VerdictView {
	const kind = parseKey(o.key)?.kind ?? '';
	switch (o.verdict) {
		case 'met':
			return { cls: 'v-met', glyph: '✓', unknown: false, hint: o.regressed ? 'não vale mais' : '' };
		case 'unmet':
			return { cls: 'v-unmet', glyph: '', unknown: false, hint: 'não atende' };
		default:
			return { cls: 'v-unk', glyph: '?', unknown: true, hint: UNKNOWN_HINT[kind] ?? 'não sabemos' };
	}
}

export interface Chip {
	key: string;
	ref: string;
	kind: string;
	role: Role;
	roleLabel: string;
	label: string;
	verdict: Verdict;
	view: VerdictView;
}

/** Chips dos objetivos de um passo, com o veredito do board (ausente = não sabemos). */
export function chipsOf(
	step: Pick<ContentStep, 'objectives' | 'names'>,
	board: Pick<BoardStep, 'objectives'> | undefined
): Chip[] {
	const verdicts = new Map((board?.objectives ?? []).map((o) => [`${o.role}|${o.key}`, o]));
	return (step.objectives ?? []).map((o) => {
		const b = verdicts.get(`${o.role}|${o.key}`);
		const verdict = b?.verdict ?? 'unknown';
		const k = parseKey(o.key);
		return {
			key: o.key,
			ref: k?.ref ?? '',
			kind: k?.kind ?? '',
			role: o.role,
			roleLabel: ROLE_LABEL[o.role] ?? o.role,
			label: objectiveLabel(o.key, step.names),
			verdict,
			view: verdictView({ key: o.key, verdict, regressed: b?.regressed })
		};
	});
}

/** Frase de apoio quando algum chip é "não sabemos": de onde virá a fonte. */
export function unknownHint(chips: Chip[]): string {
	const kinds = new Set(chips.filter((c) => c.verdict === 'unknown').map((c) => c.kind));
	const parts: string[] = [];
	if (kinds.has('quest'))
		parts.push('quests ficam "não sabemos" até o plugin 1.1 enviar a faceta de quest');
	if (kinds.has('item')) parts.push('itens dependem dos containers do plugin');
	if (kinds.has('skill')) parts.push('levels vêm do hiscores ou do plugin');
	if (kinds.has('diary')) parts.push('diaries esperam a faceta do plugin 1.1');
	if (parts.length === 0) return '';
	const text = parts.join('; ');
	return text.charAt(0).toUpperCase() + text.slice(1) + '.';
}

/* ------------------------------------------------------------------ estado do passo */

export interface StatusView {
	/** Classe do círculo: st-auto, st-man, st-imp, st-skip, st-cur, ''. */
	st: string;
	label: string;
	done: boolean;
}

export function statusView(status: StepStatus | undefined, current = false): StatusView {
	if (current) return { st: 'st-cur', label: 'agora', done: false };
	switch (status) {
		case 'auto_done':
			return { st: 'st-auto', label: 'detectado', done: true };
		case 'manual_done':
			return { st: 'st-man', label: 'marcado', done: true };
		case 'implied':
			return { st: 'st-imp', label: 'presumido', done: true };
		case 'skipped':
			return { st: 'st-skip', label: 'pulado', done: false };
		case 'pinned_open':
			return { st: '', label: 'reaberto', done: false };
		default:
			return { st: '', label: '', done: false };
	}
}

/* ------------------------------------------------------------------ passo-parágrafo (BRUHsailer) */

export interface Paragraph {
	/** Texto aberto: o TL;DR do autor, ou o primeiro bullet quando não há TL;DR. */
	head: string;
	tldr: boolean;
	/** O que fica recolhido em "Passo a passo do autor". */
	rest: string[];
	bullets: number;
	chars: number;
}

const TLDR = /TL;DR:?\s*/i;

/** Divide o texto do autor em bullets e acha o TL;DR (decisão do dono, 2026-11-04). */
export function paragraphOf(text: string): Paragraph {
	const bullets = text
		.split(/\n+/)
		.map((l) => l.replace(/^\s*[-•]\s*/, '').trim())
		.filter(Boolean);
	const i = bullets.findIndex((b) => TLDR.test(b));
	if (i >= 0) {
		const head = bullets[i].replace(TLDR, '').trim();
		const rest = bullets.filter((_, j) => j !== i);
		return { head, tldr: true, rest, bullets: rest.length, chars: rest.join(' ').length };
	}
	const [head = '', ...rest] = bullets;
	return { head, tldr: false, rest, bullets: rest.length, chars: rest.join(' ').length };
}

/** Passo de parágrafo: texto longo em bullets, como no BRUHsailer. */
export function isParagraphStep(step: Pick<ContentStep, 'text'>): boolean {
	return step.text.length > 280 || /\n\s*-\s/.test(step.text);
}

const META: [RegExp, string][] = [
	[/^GP after step:\s*(.+)$/i, 'GP após'],
	[/^Total time:\s*(.+)$/i, 'tempo acumulado'],
	[/^Items needed:\s*(.+)$/i, 'itens']
];

/** Metadados do BRUHsailer nas notas ("GP after step", "Total time", "Items needed") como chips. */
export function metaOf(step: Pick<ContentStep, 'notes'>): { label: string; value: string }[] {
	const out: { label: string; value: string }[] = [];
	for (const n of step.notes ?? []) {
		for (const [re, label] of META) {
			const m = re.exec(n.text.trim());
			if (m && m[1].trim().toLowerCase() !== 'none') out.push({ label, value: m[1].trim() });
		}
	}
	return out;
}

/** Notas que não são metadados (dicas do Oziris). */
export function plainNotes(step: Pick<ContentStep, 'notes'>): string[] {
	return (step.notes ?? [])
		.map((n) => n.text.trim())
		.filter((t) => t && !META.some(([re]) => re.test(t)) && !/^Skills \/ quests:/i.test(t));
}

/* ------------------------------------------------------------------ trechos (segmentos) */

export interface SegmentView {
	id: string;
	kind: 'bank_leg' | 'sequence';
	loadout?: Loadout;
	bankActions: { type: string; position: string; text: string }[];
	steps: ContentStep[];
	/** "1.1.75 – 1.1.113" com as referências do autor. */
	range: string;
}

/** Agrupa os passos da seção por trecho de banco (o agrupador natural do doc 09). */
export function segmentsOf(
	section: Pick<ContentSection, 'segments' | 'steps' | 'id'>
): SegmentView[] {
	const byId = new Map(section.steps.map((s) => [s.id, s]));
	const segs = section.segments?.length
		? section.segments
		: [
				{
					id: section.id,
					kind: 'sequence' as const,
					steps: section.steps.map((s) => ({ id: s.id }))
				}
			];
	return segs.map((seg) => {
		const steps = seg.steps.map((s) => byId.get(s.id)).filter((s): s is ContentStep => !!s);
		const first = steps[0]?.ref ?? '';
		const last = steps[steps.length - 1]?.ref ?? '';
		return {
			id: seg.id,
			kind: seg.kind,
			loadout: 'loadout' in seg ? seg.loadout : undefined,
			bankActions: ('bank_actions' in seg ? seg.bank_actions : undefined) ?? [],
			steps,
			range: first === last ? first : `${first} – ${last}`
		};
	});
}

export function loadoutLabel(l: Loadout | undefined): string {
	if (!l || l.status === 'unknown') return '';
	return l.status === 'known' ? 'loadout conhecido' : 'loadout parcial';
}

export function loadoutItems(l: Loadout | undefined): ItemRequirement[] {
	return [...(l?.inventory ?? []), ...(l?.equipment ?? [])];
}

/** "1k" → 1000 já vem resolvido; aqui só o texto curto de quantidade do slot. */
export function qtyLabel(n: number | undefined): string {
	if (n === undefined) return '';
	if (n >= 1_000_000) return `${+(n / 1_000_000).toFixed(1)}m`;
	if (n >= 10_000) return `${+(n / 1000).toFixed(1)}k`;
	return String(n);
}

/* ------------------------------------------------------------------ banco: com você / no banco / o guia manda */

export interface FreshnessLabel {
	label: string;
	cls: '' | 'fr-rest' | 'fr-un';
	live: boolean;
}

export function freshnessLabel(
	seg: Pick<SegmentReadiness, 'freshness'> | null,
	container: string,
	now: number
): FreshnessLabel {
	const f = seg?.freshness?.[container];
	const name =
		container === 'bank' ? 'banco' : container === 'inventory' ? 'inventário' : 'equipamento';
	if (!f || !f.observed || f.state === 'unavailable')
		return { label: `${name} · não verificável`, cls: 'fr-un', live: false };
	const at = f.observed_at ? Date.parse(f.observed_at) : NaN;
	if (f.state === 'restored') {
		return {
			label: `${name} · espelho${Number.isNaN(at) ? '' : ` de ${shortDate(at)}`}`,
			cls: 'fr-rest',
			live: false
		};
	}
	return {
		label: `${name} · ao vivo${Number.isNaN(at) ? '' : ` · ${ago(at, now)}`}`,
		cls: '',
		live: true
	};
}

function shortDate(at: number): string {
	const d = new Date(at);
	const p = (n: number) => String(n).padStart(2, '0');
	return `${p(d.getDate())}/${p(d.getMonth() + 1)} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function ago(at: number, now: number): string {
	const s = Math.max(0, Math.round((now - at) / 1000));
	if (s < 60) return `há ${Math.max(1, s)} s`;
	const m = Math.round(s / 60);
	if (m < 60) return `há ${m} min`;
	const h = Math.round(m / 60);
	return h < 48 ? `há ${h} h` : `há ${Math.round(h / 24)} d`;
}

export interface BankSlot {
	name: string;
	itemId?: number;
	qty: string;
	cls: '' | 'slot-miss' | 'slot-unk' | 'slot-stale';
	suffix: string;
	ok: boolean;
}

export interface BankColumns {
	carried: number;
	total: number;
	slots: BankSlot[];
	inBank: number;
	missing: number;
	unverifiable: number;
	unresolved: string[];
}

/** As três colunas do card de banco a partir da readiness do trecho (decisão 7 do v4). */
export function bankColumns(seg: SegmentReadiness): BankColumns {
	const out: BankColumns = {
		carried: 0,
		total: seg.items.length,
		slots: [],
		inBank: 0,
		missing: 0,
		unverifiable: 0,
		unresolved: []
	};
	for (const it of seg.items) {
		if (it.carried >= Math.max(1, it.required)) out.carried++;
		out.slots.push(slotOf(it));
	}
	for (const s of out.slots) {
		if (s.cls === 'slot-miss') out.missing++;
		else if (s.cls === 'slot-unk') out.unverifiable++;
		else out.inBank++;
	}
	out.unresolved = seg.items
		.filter((it) => it.variant === 'unresolved')
		.map((it) => it.source_text ?? it.name);
	return out;
}

function slotOf(it: ReadinessItem): BankSlot {
	const bank = it.owned?.containers?.bank;
	const base = {
		name: it.name,
		itemId: it.matched_id ?? it.item_id ?? it.any_of?.[0],
		qty: it.qty_stated || it.required > 1 ? qtyLabel(it.required) : ''
	};
	if (it.variant === 'unresolved') return { ...base, cls: 'slot-unk', suffix: 'sem id', ok: false };
	switch (it.verdict) {
		case 'met':
			return it.owned?.stale
				? {
						...base,
						qty: qtyLabel(bank?.qty) || base.qty,
						cls: 'slot-stale',
						suffix: 'tinha',
						ok: false
					}
				: { ...base, qty: qtyLabel(bank?.qty) || base.qty, cls: '', suffix: '', ok: true };
		case 'unmet':
			return { ...base, qty: String(bank?.qty ?? 0), cls: 'slot-miss', suffix: 'falta', ok: false };
		default:
			return { ...base, cls: 'slot-unk', suffix: 'não sabemos', ok: false };
	}
}

/* ------------------------------------------------------------------ cobertura e seções (Rotas) */

export function pct(part: number, whole: number): number {
	return whole > 0 ? Math.round((part / whole) * 100) : 0;
}

export function coverageLine(c: Coverage): string {
	return `${c.met} / ${c.objectives} objetivos detectáveis`;
}

export function checkboxLine(c: Coverage): string {
	const skipped = c.checkbox_skipped === 1 ? '1 pulado' : `${c.checkbox_skipped} pulados`;
	return `${c.checkbox} passos sem objetivo detectável contam só por marca manual (${c.checkbox_done} feitos, ${skipped}).`;
}

/** Rótulo curto da seção: o id do Oziris ("1.1"), "cap. 1.1" no BRUHsailer, "S" na track. */
export function sectionLabel(id: string): string {
	const m = /^bruh-(.+)$/.exec(id);
	return m ? `cap. ${m[1]}` : id;
}

/* ------------------------------------------------------------------ "Nesta sessão" */

export interface EventLine {
	st: string;
	text: string;
	right: string;
	at: number;
}

const SOURCE_SHORT: Record<string, string> = {
	hiscores: 'HS',
	plugin: 'RL',
	wom: 'WOM',
	wikisync: 'WS'
};

/** Linhas do painel "Nesta sessão": passos e objetivos, mais novos primeiro. */
export function eventLines(
	ev: EventsView,
	refOf: (stepId: string) => string,
	names: Record<string, string>,
	limit = 8
): EventLine[] {
	const time = (at: number) => {
		const d = new Date(at);
		return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
	};
	const lines: EventLine[] = [];
	for (const e of ev.steps) {
		const at = Date.parse(e.at);
		const ref = refOf(e.step_id);
		const src = e.source ? (SOURCE_SHORT[e.source] ?? e.source) : '';
		switch (e.to) {
			case 'auto_done':
				lines.push({
					st: 'st-auto',
					text: `${ref} detectado`,
					right: [src, time(at)].filter(Boolean).join(' · '),
					at
				});
				break;
			case 'implied':
				lines.push({ st: 'st-imp', text: `${ref} presumido`, right: 'cascata', at });
				break;
			case 'manual_done':
				lines.push({ st: 'st-man', text: `${ref} marcado`, right: `você · ${time(at)}`, at });
				break;
			case 'skipped':
				lines.push({ st: 'st-skip', text: `${ref} pulado`, right: `você · ${time(at)}`, at });
				break;
			case 'pending':
				if (e.cause === 'user')
					lines.push({ st: '', text: `${ref} desfeito`, right: `você · ${time(at)}`, at });
				break;
		}
	}
	for (const e of ev.objectives) {
		if (e.to !== 'met') continue;
		const at = Date.parse(e.at);
		const src = e.source ? (SOURCE_SHORT[e.source] ?? e.source) : '';
		lines.push({
			st: 'badge',
			text: objectiveLabel(e.key, names),
			right: [src, time(at)].filter(Boolean).join(' · '),
			at
		});
	}
	return lines.sort((a, b) => b.at - a.at).slice(0, limit);
}

/* ------------------------------------------------------------------ "também na rota X" */

export interface AlsoIn {
	guide: string;
	label: string;
	entries: { label: string; where: string }[];
}

/**
 * A referência cruzada discreta do passo (decisão 1 do v4): para cada outra rota, onde ela
 * conclui as mesmas entidades, ou "não está na rota".
 */
export function alsoIn(
	step: Pick<ContentStep, 'also'>,
	chips: Chip[],
	others: { guide: string; label: string }[]
): AlsoIn[] {
	const completing = chips.filter((c) => c.role === 'completes' || c.role === 'starts');
	if (completing.length === 0) return [];
	return others.map((g) => ({
		guide: g.guide,
		label: g.label,
		entries: completing.map((c) => {
			const occ = step.also?.[c.key]?.find((o) => o.guide === g.guide);
			if (!occ) return { label: c.label, where: 'não está na rota' };
			return {
				label: c.label,
				where: occ.position ? `${occ.ref} (pos. ${occ.position} de ${occ.total})` : occ.ref
			};
		})
	}));
}

/** Nome curto da rota para as linhas cruzadas: "Oziris", "BRUHsailer". */
export function routeLabel(guide: string, title: string): string {
	if (guide.startsWith('oziris')) return 'Oziris';
	if (guide.startsWith('bruhsailer')) return 'BRUHsailer';
	return title;
}

/* ------------------------------------------------------------------ Mundo e metas (V4Mundo) */

/** Rótulo do alvo dentro da entidade: "iniciada", "concluída", "≥ 70", "×120". */
export function stateLabel(key: string): string {
	const k = parseKey(key);
	if (!k) return key;
	if (k.op === '@') {
		return (
			{ started: 'iniciada', finished: 'concluída', complete: 'completa', done: 'feito' }[
				k.state ?? ''
			] ??
			k.state ??
			''
		);
	}
	return k.op === 'x' ? `×${k.n}` : `≥ ${k.n}`;
}

const STATE_RANK: Record<string, number> = { started: 1, finished: 2, complete: 2, done: 2 };

/** O objetivo que "Virar meta" fixa: o estado final ou o alvo mais alto da entidade. */
export function mainKey(keys: string[]): string | undefined {
	let best: string | undefined;
	let bestRank = -1;
	for (const key of keys) {
		const k = parseKey(key);
		if (!k) continue;
		const rank = k.op === '@' ? (STATE_RANK[k.state ?? ''] ?? 0) * 1e9 : (k.n ?? 0);
		if (rank > bestRank) {
			best = key;
			bestRank = rank;
		}
	}
	return best;
}

export const KIND_LABEL: Record<string, string> = {
	quest: 'quest',
	item: 'item',
	skill: 'level',
	diary: 'diary',
	unlock: 'unlock',
	location: 'local',
	boss: 'boss',
	ca: 'ca',
	qp: 'qp'
};

/** Veredito agregado da entidade para a bolinha da lista: met se todos, progresso se algum. */
export function entityVerdict(
	objectives: { verdict: Verdict }[]
): 'met' | 'prog' | 'unmet' | 'unknown' {
	if (objectives.length === 0) return 'unknown';
	const met = objectives.filter((o) => o.verdict === 'met').length;
	if (met === objectives.length) return 'met';
	if (met > 0) return 'prog';
	return objectives.some((o) => o.verdict === 'unmet') ? 'unmet' : 'unknown';
}

/** "Oziris 1.5.17 · BRUHsailer CH2.72": onde cada rota conclui a meta. */
export function routesLine(
	routes: { guide: string; ref: string }[],
	label: (guide: string) => string
): string {
	return routes.map((r) => `${label(r.guide)} ${r.ref}`).join(' · ');
}

/* ------------------------------------------------------------------ fontes (V4Vazios) */

export const STALE_SOURCE_MS = 24 * 3600_000;

export interface SourceBand {
	kind: 'failed' | 'stale';
	text: string;
}

/**
 * Faixa do topo do Agora: a última tentativa do hiscores falhou ("fonte falhou"), ou o último
 * dado bom tem mais de 24 h ("fonte velha"). Nenhuma das duas muda veredito.
 */
export function sourceBand(
	last: SyncRun | undefined,
	lastObserved: number | undefined,
	now: number
): SourceBand | null {
	const hhmm = (at: number) => {
		const d = new Date(at);
		return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
	};
	const good = lastObserved ? ` Os níveis são do último snapshot bom, ${hhmm(lastObserved)}.` : '';
	if (last && (last.status === 'error' || last.status === 'rate_limited')) {
		const at = Date.parse(last.finished_at ?? last.created_at);
		if (last.http_status === 404 && last.error?.includes('renomeou')) {
			return { kind: 'failed', text: `Hiscores: ${last.error}` };
		}
		const why = last.http_status ? `HTTP ${last.http_status}` : 'sem resposta';
		const retry = last.next_allowed_at
			? ` Nova tentativa às ${hhmm(Date.parse(last.next_allowed_at))}.`
			: '';
		return {
			kind: 'failed',
			text: `Hiscores indisponível desde ${hhmm(at)} (${why}).${good}${retry}`
		};
	}
	if (lastObserved && now - lastObserved > STALE_SOURCE_MS) {
		return {
			kind: 'stale',
			text: `Dados do hiscores ${ago(lastObserved, now)}: os requisitos de nível podem estar errados.`
		};
	}
	return null;
}

/* ------------------------------------------------------------------ marcar tudo antes */

/**
 * Os passos que "marcar tudo antes deste" marcaria: da rota principal, visíveis, antes do
 * passo, pendentes sem marca ou presumidos (o mesmo critério de progress.PlanMarkBefore).
 */
export function pendingBefore(
	steps: Pick<BoardStep, 'id' | 'status' | 'mark' | 'main' | 'visible'>[],
	stepId: string
): string[] {
	const out: string[] = [];
	for (const s of steps) {
		if (s.id === stepId) return out;
		if (!s.main || !s.visible) continue;
		if (s.mark ? s.mark.kind === 'implied' : s.status === 'pending') out.push(s.id);
	}
	return [];
}
