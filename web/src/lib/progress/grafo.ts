import type { Graph, GraphLink, GraphNode, StepPlace } from './types.ts';

/**
 * Peças puras do Mundo › Grafo (V6Grafo1440 em colunas, V6Grafo640 em árvore): o foco no meio,
 * os requisitos em colunas por profundidade à esquerda (a mais funda primeiro) e o que o foco
 * destrava à direita. Feitos colapsam numa linha "+N feitos"; os níveis que só um nó pede viram
 * uma linha só ("50 Agility · 50 Crafting…", um por skill no título). `unknown` nunca aparece
 * como não feito: tem a própria bolinha "?".
 */

/** Filtro de tipo do canvas: tudo · quest · level · item · unlock. */
export type KindFilter = 'tudo' | 'quest' | 'level' | 'item' | 'unlock';

export interface GraphOptions {
	hideDone: boolean;
	criticalOnly: boolean;
	kind: KindFilter;
	/** Nome curto da rota para "Oziris 1.1.150". */
	routeName: (guide: string) => string;
	/** O passo do cursor de cada rota, para "você está aqui". */
	cursorOf?: (guide: string) => string | undefined;
}

export interface NodeView {
	/** Chave do nó (a primeira, num grupo de níveis). */
	key: string;
	/** Entidade que o clique abre no painel; vazia para quest points. */
	ref: string;
	label: string;
	/** Classe da bolinha: v-met, v-prog, v-unmet (disponível), v-lock, v-unk. */
	vcls: string;
	/** A linha de baixo: "p/ Biohazard · Oziris 1.1.150", "p/ Legends’ · falta Biohazard". */
	sub: string;
	critical: boolean;
	/** Chaves do grupo de níveis (só num grupo). */
	keys?: string[];
}

export interface ColumnView {
	depth: number;
	title: string;
	nodes: NodeView[];
	/** Requisitos desta profundidade escondidos por filtro (fora os feitos). */
	shown: number;
	done: { count: number; names: string };
}

export interface GraphView {
	columns: ColumnView[];
	focus: NodeView;
	unlocks: NodeView[];
	/** Contagens da plaquinha: nós do grafo e feitos colapsados. */
	nodes: number;
	collapsed: number;
}

const isLevel = (n: GraphNode) => n.kind === 'skill' || n.kind === 'qp';
const isLevelKey = (key: string) => key.startsWith('skill:') || key.startsWith('qp>=');

/**
 * "50 Agility", "175 quest points", "Legends’ Quest"; as subquests de Recipe for Disaster ficam
 * curtas como no canvas: "RFD · Sir Amik Varze", "RFD · Lumbridge Guide".
 */
export function nodeLabel(n: Pick<GraphNode, 'key' | 'label'>): string {
	const m = /^(?:skill:[a-z_]+|qp)>=(\d+)$/.exec(n.key);
	if (!m) {
		const rfd = /^Recipe for Disaster\/(?:Freeing (?:the )?)?(.+)$/.exec(n.label);
		return rfd ? `RFD · ${rfd[1]}` : n.label;
	}
	return n.key.startsWith('qp') ? `${m[1]} quest points` : `${m[1]} ${n.label}`;
}

export function verdictClass(n: Pick<GraphNode, 'group' | 'verdict' | 'kind'>): string {
	// Um nível que não atende se treina agora: disponível, como no canvas ("70 Cooking").
	if (n.group === 'blocked' && n.verdict === 'unmet' && (n.kind === 'skill' || n.kind === 'qp'))
		return 'v-unmet';
	switch (n.group) {
		case 'done':
		case 'presumed':
			return 'v-met';
		case 'available':
			return 'v-unmet';
		case 'blocked':
			return n.verdict === 'unknown' ? 'v-unk' : 'v-lock';
	}
	return 'v-unk';
}

function matches(n: GraphNode, kind: KindFilter): boolean {
	switch (kind) {
		case 'tudo':
			return true;
		case 'level':
			return isLevel(n);
		default:
			return n.kind === kind;
	}
}

function namesOf(labels: string[], shown = 3): string {
	if (labels.length <= shown) return labels.join(' · ');
	return `${labels.slice(0, shown).join(' · ')}…`;
}

/** O estado de um nó na linha de baixo, sem o "p/ …". */
export function statusText(
	n: GraphNode,
	byKey: Map<string, GraphNode>,
	opts: Pick<GraphOptions, 'routeName' | 'cursorOf'>
): string {
	const parts: string[] = [];
	if (isLevel(n) && n.have !== undefined && n.group !== 'done') parts.push(`tem ${n.have}`);
	if (n.group === 'done') parts.push('feito');
	else if (n.group === 'presumed') parts.push('marcado à mão');
	else if (n.step) parts.push(where(n.step, opts));
	else if (n.group === 'available') parts.push('disponível');
	else if (n.group === 'unknown') parts.push('não sabemos');
	else if (n.missing?.length) {
		const quests = n.missing.filter((k) => !isLevelKey(k));
		const levels = n.missing.length - quests.length;
		if (quests.length && quests.length <= 2 && !levels)
			parts.push(
				`falta ${quests.map((k) => nodeLabel(byKey.get(k) ?? { key: k, label: k })).join(', ')}`
			);
		else {
			const bits: string[] = [];
			if (levels) bits.push(`${levels} ${levels === 1 ? 'nível' : 'níveis'}`);
			if (quests.length) bits.push(`${quests.length} ${quests.length === 1 ? 'quest' : 'quests'}`);
			parts.push(bits.join(' · '));
		}
	}
	return parts.join(' · ');
}

function where(p: StepPlace, opts: Pick<GraphOptions, 'routeName' | 'cursorOf'>): string {
	if (opts.cursorOf?.(p.guide_id) === p.step_id) return 'você está aqui';
	return `${opts.routeName(p.guide_id)} ${p.ref}`;
}

function forText(n: GraphNode, byKey: Map<string, GraphNode>): string {
	const parents = n.for ?? [];
	if (!parents.length) return '';
	const first = byKey.get(parents[0]);
	const name = first ? nodeLabel(first) : parents[0];
	return `p/ ${name}${parents.length > 1 ? ` +${parents.length - 1}` : ''}`;
}

function view(
	n: GraphNode,
	byKey: Map<string, GraphNode>,
	opts: GraphOptions,
	withParent: boolean
): NodeView {
	const sub = [withParent ? forText(n, byKey) : '', statusText(n, byKey, opts)]
		.filter(Boolean)
		.join(' · ');
	return {
		key: n.key,
		ref: n.ref ?? '',
		label: nodeLabel(n),
		vcls: verdictClass(n),
		sub,
		critical: !!n.critical
	};
}

/**
 * Agrupa os níveis abertos que só um nó pede (o mesmo "p/"): o canvas mostra "50 Agility · 50
 * Crafting · 45 Herblore…" numa linha, com "p/ Legends’ · 3 níveis".
 */
function groupLevels(
	nodes: GraphNode[],
	byKey: Map<string, GraphNode>,
	opts: GraphOptions,
	withParent: boolean
): NodeView[] {
	const out: NodeView[] = [];
	const groups = new Map<string, GraphNode[]>();
	for (const n of nodes) {
		if (isLevel(n) && n.group !== 'done' && (n.for?.length ?? 0) === 1) {
			const k = n.for![0];
			if (!groups.has(k)) groups.set(k, []);
			groups.get(k)!.push(n);
		}
	}
	const emitted = new Set<string>();
	for (const n of nodes) {
		const parent = n.for?.[0] ?? '';
		const group = isLevel(n) && (n.for?.length ?? 0) === 1 ? groups.get(parent) : undefined;
		if (!group || group.length < 2) {
			out.push(view(n, byKey, opts, withParent));
			continue;
		}
		if (emitted.has(parent)) continue;
		emitted.add(parent);
		const labels = group.map(nodeLabel);
		const haves = group.map((g) => (g.have === undefined ? '?' : String(g.have)));
		const p = byKey.get(parent);
		// Nas colunas, "p/ Legends’ · 9 níveis"; na árvore, o pai é a linha de cima e a linha de
		// baixo traz os níveis atuais ("31 · 35 · 20 · 41").
		const sub = withParent
			? [p ? `p/ ${nodeLabel(p)}` : '', `${group.length} níveis`].filter(Boolean).join(' · ')
			: namesOf(haves, 4);
		out.push({
			key: group[0].key,
			ref: group[0].ref ?? '',
			label: namesOf(labels),
			vcls: group.every((g) => g.verdict === 'unknown') ? 'v-unk' : 'v-unmet',
			sub,
			critical: group.some((g) => !!g.critical),
			keys: group.map((g) => g.key)
		});
	}
	return out;
}

const BOTTLENECK_SKILL = (skill: string, target: number, label?: string) =>
	skill === 'qp' ? `${target} quest points` : `${target} ${label ?? skill}`;

/** As colunas, o foco e o destrava do canvas a partir da resposta da API. */
export function buildGraphView(g: Graph, opts: GraphOptions): GraphView {
	const byKey = new Map(g.nodes.map((n) => [n.key, n]));
	const focusNode = byKey.get(g.focus);
	const columns: ColumnView[] = [];
	let collapsed = 0;
	// Colunas até o requisito mais fundo: uma profundidade pedida maior não desenha colunas vazias.
	for (let depth = Math.min(g.depth, Math.max(g.max_depth, 1)); depth >= 1; depth--) {
		const all = g.nodes.filter((n) => n.depth === depth);
		const done = all.filter((n) => n.group === 'done');
		let open = opts.hideDone ? all.filter((n) => n.group !== 'done') : all;
		open = open.filter((n) => matches(n, opts.kind));
		if (opts.criticalOnly) open = open.filter((n) => n.critical);
		const nodes = groupLevels(open, byKey, opts, true);
		if (opts.hideDone) collapsed += done.length;
		columns.push({
			depth,
			title: `Requer · nível ${depth}`,
			nodes,
			shown: nodes.length,
			done: opts.hideDone
				? { count: done.length, names: namesOf(done.map(nodeLabel)) }
				: { count: 0, names: '' }
		});
	}
	// O foco: tipo, "6 de 20", o gargalo e o fim do caminho crítico.
	const subs: string[] = [];
	if (g.requirements) subs.push(`${g.met} de ${g.total}`);
	const top = g.bottlenecks.find((b) => !b.met);
	if (top) {
		const label = byKey.get(top.key)?.label;
		subs.push(`gargalo ${BOTTLENECK_SKILL(top.skill, top.target, label)}`);
	}
	if (g.critical.length > 1)
		subs.push(`caminho crítico: ${nodeLabel(g.critical[g.critical.length - 1])}`);
	const focusVerdict =
		focusNode?.group === 'done'
			? 'v-met'
			: g.met > 0
				? 'v-prog'
				: focusNode
					? verdictClass(focusNode)
					: 'v-unk';
	const focus: NodeView = {
		key: g.focus,
		ref: focusNode?.ref ?? '',
		label: nodeLabel(focusNode ?? { key: g.focus, label: g.label }),
		vcls: focusVerdict,
		sub: subs.join(' · '),
		critical: g.critical.length > 0
	};
	const unlocks = g.unlocks.map((u) => unlockView(u));
	return {
		columns,
		focus,
		unlocks,
		nodes: g.nodes.length + g.hidden,
		collapsed
	};
}

export function unlockView(u: GraphLink): NodeView {
	// Disponível: nenhum requisito direto em aberto; bloqueada: falta algum (não feito ou não sabemos).
	let vcls = u.missing ? 'v-lock' : 'v-unmet';
	let state = u.missing ? `faltam ${u.missing}` : 'disponível';
	if (u.verdict === 'met') {
		vcls = 'v-met';
		state = 'feito';
	}
	return {
		key: u.key,
		ref: u.ref,
		label: u.label,
		vcls,
		sub: `${u.kind} · ${state}`,
		critical: false
	};
}

export interface TreeRow {
	/** Chave estável da linha: o caminho de chaves desde a raiz. */
	id: string;
	depth: number;
	node: NodeView;
	/** Há filhos (abertos ou colapsados) abaixo desta linha. */
	parent: boolean;
	/** Linha de feitos colapsados ("+2 feitos · Family Crest · Shilo Village"). */
	done?: boolean;
}

/**
 * A árvore do estreito (V6Grafo640): a partir dos requisitos diretos do foco, cada nó com os
 * seus requisitos recuados embaixo, até a profundidade pedida. `closed` são as linhas recolhidas.
 */
export function buildTree(
	g: Graph,
	opts: GraphOptions,
	closed: Set<string> = new Set()
): TreeRow[] {
	const byKey = new Map(g.nodes.map((n) => [n.key, n]));
	const rows: TreeRow[] = [];
	// depth é o recuo da linha (0 para os requisitos diretos do foco), até g.depth colunas.
	const walk = (parentKey: string, depth: number, path: string) => {
		const parent = byKey.get(parentKey);
		if (!parent || depth >= g.depth) return;
		const kids = (parent.requires ?? [])
			.map((k) => byKey.get(k))
			.filter((n): n is GraphNode => !!n && !path.split('/').includes(n.key));
		const done = kids.filter((n) => n.group === 'done');
		let open = opts.hideDone ? kids.filter((n) => n.group !== 'done') : kids;
		open = open.filter((n) => matches(n, opts.kind));
		if (opts.criticalOnly) open = open.filter((n) => n.critical);
		for (const v of groupLevels(open, byKey, opts, false)) {
			const id = `${path}/${v.key}`;
			const n = byKey.get(v.key);
			const hasKids = !v.keys && depth + 1 < g.depth && !!n?.requires?.some((k) => byKey.get(k));
			const sub = v.critical && hasKids ? ['crítico', v.sub].filter(Boolean).join(' · ') : v.sub;
			rows.push({ id, depth, node: { ...v, sub }, parent: hasKids });
			if (hasKids && !closed.has(id)) walk(v.key, depth + 1, id);
		}
		if (opts.hideDone && done.length) {
			rows.push({
				id: `${path}/+done`,
				depth,
				parent: false,
				done: true,
				node: {
					key: `${path}/+done`,
					ref: '',
					label: `+${done.length} ${done.length === 1 ? 'feito' : 'feitos'} · ${namesOf(done.map(nodeLabel))}`,
					vcls: 'v-met',
					sub: '',
					critical: false
				}
			});
		}
	};
	walk(g.focus, 0, '');
	return rows;
}
