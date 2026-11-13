/*
 * Mundo da frota (página 8 do canvas, V7Mundo1440; fatia V10 de docs/ui/decisoes-canvas.md 7.5):
 * as metas fixadas por qualquer conta, quem persegue cada uma e quão perto está, o que falta para
 * uma conta (o plano da meta) e onde cada rota chega. Só o que GET /api/fleet/goals e o plano medem:
 * sem dado, a linha some; unknown nunca vira unmet. Puro, testado com node --test.
 */
import type {
	AccountView,
	FleetGoal,
	FleetStanding,
	GoalNode,
	GoalPlan,
	Shortfall,
	StepPlace
} from './types.ts';
import { KIND_LABEL, objectiveLabel, parseKey, pct } from './view.ts';

/** Etiqueta do tipo na lista de metas (o artboard escreve "skill", não "level"). */
export function kindTag(kind: string): string {
	return kind === 'skill' ? 'skill' : (KIND_LABEL[kind] ?? kind);
}

/** Nome da meta: "Fishing 99" numa meta de nível (como no artboard); o rótulo do objetivo no resto. */
export function goalTitle(g: Pick<FleetGoal, 'objective_key' | 'name' | 'ref'>): string {
	const k = parseKey(g.objective_key);
	if (k?.kind === 'skill' && k.n !== undefined) return `${g.name} ${k.n}`;
	return objectiveLabel(g.objective_key, { [g.ref]: g.name });
}

/** "1 conta", "3 contas". */
export function plural(n: number, one: string, many: string): string {
	return `${n} ${n === 1 ? one : many}`;
}

/** "a", "a e b", "a, b e c". */
export function listJoin(parts: string[]): string {
	if (parts.length <= 1) return parts.join('');
	return `${parts.slice(0, -1).join(', ')} e ${parts[parts.length - 1]}`;
}

/**
 * Quão perto uma conta está, com a mesma medida do `closest` do servidor: o nível numa meta de
 * nível, a fração de requisitos feitos no resto. Nulo sem medida (meta de item sem requisitos,
 * nível sem leitura, ou nenhum requisito conhecido: `unknown` nunca vira 0%).
 */
export interface Closeness {
	/** Largura da barra, 0 a 100. */
	pct: number;
	/** Nível da conta numa meta de nível; ausente nas outras. */
	level?: number;
	/** O alvo da meta de nível. */
	target?: number;
}

export function closeness(g: Pick<FleetGoal, 'objective_key'>, s: FleetStanding): Closeness | null {
	if (s.verdict === 'met') return { pct: 100 };
	const k = parseKey(g.objective_key);
	if (k?.kind === 'skill' && k.n) {
		if (s.level === undefined) return null;
		return { pct: Math.min(100, pct(s.level, k.n)), level: s.level, target: k.n };
	}
	const r = s.requirements;
	if (r.total === 0 || r.unknown === r.total) return null;
	return { pct: pct(r.met, r.total) };
}

/** A medida na coluna estreita da linha da conta: "61%" ou "76/99". */
export function closenessLabel(c: Closeness): string {
	return c.level !== undefined ? `${c.level}/${c.target}` : `${c.pct}%`;
}

const has = (n: number) => (n === 1 ? 'tem' : 'têm');

/**
 * A linha de uma meta na lista: "2 contas · a mais perto em 61%", "1 conta · nível 76",
 * "3 de 5 contas têm" (quando todas as que perseguem já têm).
 */
export function goalLine(g: FleetGoal, fleetSize: number): string {
	const pursuers = g.accounts.filter((s) => s.goal);
	const haveIt = `${g.met} de ${plural(fleetSize, 'conta', 'contas')} ${has(g.met)}`;
	if (!pursuers.length) return g.met ? haveIt : '';
	if (pursuers.every((s) => s.verdict === 'met')) return haveIt;
	const who = plural(pursuers.length, 'conta', 'contas');
	const near = g.closest ? g.accounts.find((s) => s.account === g.closest) : undefined;
	const c = near ? closeness(g, near) : null;
	if (c) {
		const v = c.level !== undefined ? `nível ${c.level}` : `${c.pct}%`;
		if (pursuers.length === 1) return `${who} · ${v}`;
		return `${who} · a mais perto ${c.level !== undefined ? 'no' : 'em'} ${v}`;
	}
	return g.met ? `${who} · ${g.met} ${has(g.met)}` : who;
}

function shortLabel(x: Shortfall): string {
	const base = x.skill === 'qp' ? `${x.target} QP` : objectiveLabel(x.key, {});
	return x.level === null ? base : `${base} (tem ${x.level})`;
}

/**
 * O que falta para uma conta, do resumo do plano: "falta 175 QP e 5 quests",
 * "falta 70 Agility (tem 62)". Os desconhecidos ficam fora do "falta" (unknown não é unmet).
 */
export function missingLine(s: FleetStanding): string {
	if (s.verdict === 'met') return 'já tem';
	const parts = s.short.map(shortLabel);
	if (s.requirements.quests_left) parts.push(plural(s.requirements.quests_left, 'quest', 'quests'));
	if (parts.length) return `falta ${listJoin(parts)}`;
	const r = s.requirements;
	if (r.total && r.met + r.presumed === r.total) return 'requisitos feitos';
	if (r.unknown)
		return r.unknown === r.total
			? 'não sabemos os requisitos'
			: `não sabemos ${plural(r.unknown, 'requisito', 'requisitos')}`;
	return s.verdict === 'unknown' ? 'não sabemos' : '';
}

/** Uma conta na seção "Na frota". */
export interface FleetRow {
	account: string;
	rsn: string;
	standing: FleetStanding;
	/** A conta fixou a meta (a persegue). */
	pursues: boolean;
	measure: Closeness | null;
	line: string;
}

/**
 * As contas diante de uma meta: as que perseguem primeiro; em cada grupo, as que ainda não têm,
 * da mais perto à mais longe (sem medida no fim), e depois as que já têm; empate na ordem da frota.
 */
export function fleetRows(g: FleetGoal, accounts: AccountView[]): FleetRow[] {
	const order = new Map(accounts.map((a, i) => [a.id, i]));
	const rows = g.accounts.map((s) => ({
		account: s.account,
		rsn: accounts.find((a) => a.id === s.account)?.rsn ?? s.account,
		standing: s,
		pursues: !!s.goal,
		measure: closeness(g, s),
		line: missingLine(s)
	}));
	const rank = (r: FleetRow) =>
		r.standing.verdict === 'met' ? -1 : r.measure ? r.measure.pct : -0.5;
	return rows.sort(
		(a, b) =>
			Number(b.pursues) - Number(a.pursues) ||
			Number(a.standing.verdict === 'met') - Number(b.standing.verdict === 'met') ||
			rank(b) - rank(a) ||
			(order.get(a.account) ?? 0) - (order.get(b.account) ?? 0)
	);
}

/**
 * A conta de "O que falta": a pedida (se persegue a meta), senão a mais perto, senão a primeira
 * que persegue e ainda não tem, senão a primeira que persegue. Só quem persegue tem plano.
 */
export function needsAccount(g: FleetGoal, wanted?: string | null): FleetStanding | undefined {
	const pursuers = g.accounts.filter((s) => s.goal);
	return (
		pursuers.find((s) => s.account === wanted) ??
		pursuers.find((s) => s.account === g.closest) ??
		pursuers.find((s) => s.verdict !== 'met') ??
		pursuers[0]
	);
}

/** Bolinha de um requisito: feito, em andamento (algum requisito dele feito), não feito, não sabemos. */
export type NeedMark = 'met' | 'prog' | 'unmet' | 'unk';

export interface NeedRow {
	key: string;
	ref?: string;
	label: string;
	/** Requisito de um requisito (recuado). */
	sub: boolean;
	mark: NeedMark;
	/** O texto da direita: "3 de 10", "precisa de X", "não sabemos", o nível. */
	hint: string;
}

const trainOnly = (key: string) => key.startsWith('skill:') || key.startsWith('qp');

/**
 * A árvore curta de "O que falta" (V7Mundo1440): cada requisito direto ainda não feito e, abaixo,
 * os requisitos dele, até `limit` linhas. Numa meta sem requisitos no catálogo (nível), a linha do
 * próprio nível. `where` diz onde a rota ativa faz um requisito disponível.
 */
export function needRows(plan: GoalPlan, where: (p: StepPlace) => string, limit = 8): NeedRow[] {
	const byKey = new Map(plan.nodes.map((n) => [n.key, n]));
	const levels = new Map(plan.bottlenecks.map((b) => [b.skill, b.level]));
	const levelOf = (key: string): number | null => {
		const k = parseKey(key);
		if (!k) return null;
		return levels.get(k.kind === 'qp' ? 'qp' : k.id) ?? null;
	};
	const label = (key: string) => byKey.get(key)?.label ?? objectiveLabel(key, {});
	const done = (n: GoalNode) => n.group === 'done' || n.group === 'presumed';
	const kids = (n: GoalNode) =>
		(n.requires ?? []).map((k) => byKey.get(k)).filter((c): c is GoalNode => !!c);

	function row(n: GoalNode, sub: boolean): NeedRow {
		const children = sub ? [] : kids(n);
		const doneKids = children.filter((c) => c.group === 'done').length;
		let mark: NeedMark =
			n.group === 'done' || n.group === 'presumed'
				? 'met'
				: n.group === 'unknown'
					? 'unk'
					: 'unmet';
		if (mark !== 'met' && doneKids > 0) mark = 'prog';
		let hint: string;
		if (children.length && !done(n)) hint = `${doneKids} de ${children.length}`;
		else if (n.group === 'presumed') hint = 'pela marca';
		else if (n.group === 'unknown') hint = 'não sabemos';
		else if (n.group === 'done') hint = '';
		else if (trainOnly(n.key)) hint = levelOf(n.key)?.toString() ?? '';
		else if (n.missing?.length)
			hint =
				n.missing.length === 1
					? `precisa de ${label(n.missing[0])}`
					: `precisa de ${plural(n.missing.length, 'requisito', 'requisitos')}`;
		else hint = n.step ? where(n.step) : 'disponível';
		return { key: n.key, ref: n.ref, label: n.label, sub, mark, hint };
	}

	const out: NeedRow[] = [];
	if (!plan.requirements) {
		for (const b of plan.bottlenecks) {
			const n = byKey.get(b.key);
			if (n) out.push(row(n, false));
		}
		return out.slice(0, limit);
	}
	for (const top of plan.nodes.filter((n) => n.depth === 1 && !done(n))) {
		if (out.length >= limit) break;
		out.push(row(top, false));
		for (const c of kids(top)) {
			if (out.length >= limit) break;
			out.push(row(c, true));
		}
	}
	return out;
}

/** Um bloco de "Onde cada rota chega": o passo da rota e onde cada conta que a segue está. */
export interface RouteBlock {
	guide: string;
	ref: string;
	title: string;
	section: string;
	step: string;
	/** "inicia" quando a rota só começa o objetivo nesse passo. */
	role?: string;
	lines: string[];
}

function followerLine(rsn: string, s: FleetStanding): string {
	if (s.verdict === 'met') return `${rsn} já tem.`;
	const n = s.steps_left;
	if (n === 0) return `${rsn} já passou deste passo.`;
	if (s.cursor && n !== undefined)
		return `${rsn} está em ${s.cursor.ref}: ${n === 1 ? 'falta 1 passo' : `faltam ${n} passos`}.`;
	if (s.cursor) return `${rsn} está em ${s.cursor.ref}.`;
	return '';
}

/**
 * A ocorrência de uma rota serve à meta: a mesma entidade e, num nível ou nos quest points, um alvo
 * pelo menos igual (`routes` traz o primeiro passo que toca a entidade, que pode ser um nível menor).
 */
function servesGoal(occKey: string, goalKey: string): boolean {
	const have = parseKey(occKey);
	const want = parseKey(goalKey);
	if (!have || !want || have.ref !== want.ref || have.kind !== want.kind) return false;
	if (want.kind === 'skill' || want.kind === 'qp') return (have.n ?? 0) >= (want.n ?? 0);
	return true;
}

/**
 * Onde cada rota chega: as rotas que as contas seguem (na ordem da frota) e depois as outras que
 * fazem a meta. O passo é o primeiro da rota ativa que conclui o objetivo (`step` de uma conta que
 * a segue); sem conta nela, o primeiro passo da rota que toca a entidade (`routes`), se ele chega
 * ao alvo. Rota sem passo fica de fora.
 */
export function routeBlocks(
	g: FleetGoal,
	accounts: AccountView[],
	routeName: (guide: string) => string,
	maxLines = 4
): RouteBlock[] {
	const guides: string[] = [];
	for (const a of accounts) if (a.guide_id && !guides.includes(a.guide_id)) guides.push(a.guide_id);
	for (const r of g.routes) if (!guides.includes(r.guide)) guides.push(r.guide);
	const standing = new Map(g.accounts.map((s) => [s.account, s]));
	const out: RouteBlock[] = [];
	for (const guide of guides) {
		const followers = accounts
			.filter((a) => a.guide_id === guide && standing.has(a.id))
			.map((a) => ({ a, s: standing.get(a.id)! }))
			.sort((x, y) => Number(!!y.s.goal) - Number(!!x.s.goal));
		const own = followers.find((f) => f.s.step?.guide_id === guide)?.s.step;
		const occ = g.routes.find((r) => r.guide === guide && servesGoal(r.key, g.objective_key));
		let block: RouteBlock | null = null;
		if (own)
			block = {
				guide,
				ref: own.ref,
				title: own.title,
				section: own.section_id,
				step: own.step_id,
				lines: []
			};
		else if (occ)
			block = {
				guide,
				ref: occ.ref,
				title: occ.title ?? '',
				section: occ.section,
				step: occ.step,
				role: occ.role === 'starts' ? 'inicia' : undefined,
				lines: []
			};
		if (!block) continue;
		if (!followers.length) {
			block.lines.push(`Nenhuma conta segue a ${routeName(guide)} agora.`);
		} else {
			const lines = followers.map((f) => followerLine(f.a.rsn, f.s)).filter(Boolean);
			block.lines = lines.slice(0, maxLines);
			if (lines.length > maxLines) block.lines.push(`E mais ${lines.length - maxLines}.`);
		}
		out.push(block);
	}
	return out;
}
