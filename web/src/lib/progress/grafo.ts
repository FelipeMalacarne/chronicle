/**
 * Mundo › Grafo (V6Grafo1440 e V6Grafo640): o que a tela faz com o grafo por foco do servidor.
 * Puro, testado em node: glifo de cada cartão, a linha de baixo ("p/ Legends' · disponível"), os
 * filtros e a árvore do estreito.
 */
import type { GoalGroup, Graph, GraphColumn, GraphNode, StepPlace } from './types.ts';

/** Glifo do cartão pelo grupo (legenda do artboard): feito, em progresso, disponível, bloqueado, ?. */
export const GROUP_GLYPH: Record<GoalGroup, string> = {
	done: 'v-met',
	presumed: 'v-prog',
	available: 'v-unmet',
	blocked: 'v-lock',
	unknown: 'v-unk'
};

export const GROUP_LABEL: Record<GoalGroup, string> = {
	done: 'feito',
	presumed: 'em progresso',
	available: 'disponível',
	blocked: 'bloqueado',
	unknown: 'não sabemos'
};

/** Os tipos do filtro do artboard: tudo, quest, level, item, unlock. */
export type KindFilter = 'tudo' | 'quest' | 'level' | 'item' | 'unlock';
export const KIND_FILTERS: KindFilter[] = ['tudo', 'quest', 'level', 'item', 'unlock'];

export function kindMatches(n: GraphNode, f: KindFilter): boolean {
	switch (f) {
		case 'tudo':
			return true;
		case 'level':
			return n.kind === 'levels' || n.kind === 'skill' || n.kind === 'qp';
		default:
			return n.kind === f;
	}
}

export interface Filters {
	hideDone: boolean;
	criticalOnly: boolean;
	kind: KindFilter;
}

/** O passo da rota em poucas letras: "Oziris 1.1.150". */
export function placeLabel(p: StepPlace | undefined, routeName: (guide: string) => string): string {
	return p ? `${routeName(p.guide_id)} ${p.ref}` : '';
}

/**
 * A linha de baixo do cartão (V6Grafo1440): para quem é e, depois, o passo onde a rota ativa faz,
 * "você está aqui", "disponível", "falta X" ou "N níveis".
 */
export function nodeLine(
	n: GraphNode,
	routeName: (guide: string) => string,
	cursorStep?: string
): string {
	const parts: string[] = [];
	if (n.for?.length) parts.push(`p/ ${n.for[0]}`);
	if (n.kind === 'levels') {
		const open = n.levels?.filter((l) => l.verdict !== 'met').length ?? 0;
		parts.push(
			open ? `${open} de ${n.levels?.length ?? 0} níveis` : `${n.levels?.length ?? 0} níveis`
		);
		return parts.join(' · ');
	}
	if (n.step && n.step.step_id === cursorStep) parts.push('você está aqui');
	else if (n.group === 'available') parts.push('disponível');
	else if (n.group === 'blocked' && n.missing?.length)
		parts.push(n.missing.length === 1 ? `falta ${n.missing[0]}` : `faltam ${n.missing.length}`);
	else if (n.group === 'unknown') parts.push('não sabemos');
	if (n.step && n.step.step_id !== cursorStep) parts.push(placeLabel(n.step, routeName));
	return parts.join(' · ');
}

/** A coluna depois dos filtros: os abertos e, à parte, os feitos (que a tela recolhe). */
export interface ShownColumn {
	depth: number;
	nodes: GraphNode[];
	done: GraphNode[];
}

export function filterColumn(c: GraphColumn, f: Filters): ShownColumn {
	const keep = (n: GraphNode) => kindMatches(n, f.kind) && (!f.criticalOnly || !!n.critical);
	return {
		depth: c.depth,
		nodes: c.nodes.filter(keep),
		done: f.criticalOnly ? [] : c.done.filter(keep)
	};
}

/** "+4 feitos" com até três nomes: "Family Crest · Shilo Village · …". */
export function doneSummary(done: GraphNode[]): { count: string; names: string } {
	const names = done.slice(0, 3).map((n) => n.label);
	return {
		count: `+${done.length} feito${done.length === 1 ? '' : 's'}`,
		names: names.join(' · ') + (done.length > 3 ? ' · …' : '')
	};
}

/** Uma linha da árvore do estreito (V6Grafo640): um nó ou os feitos de um pai recolhidos. */
export interface TreeRow {
	node?: GraphNode;
	/** Os feitos de um pai, numa linha só, quando "esconder feitos" está ligado. */
	done?: GraphNode[];
	indent: number;
	/** Tem filhos na árvore (chevron). */
	parent: boolean;
}

/**
 * A árvore "requer" a partir do foco: os filhos de um nó são os da coluna seguinte que dizem ser
 * "p/" ele. Um nó com vários pais aparece só sob o primeiro.
 */
export function treeRows(g: Graph, f: Filters): TreeRow[] {
	const cols = g.columns.map((c) => filterColumn(c, f));
	const childrenOf = (label: string, depth: number) => {
		const c = cols.find((x) => x.depth === depth);
		const own = (n: GraphNode) => (n.for?.[0] ?? '') === label;
		return { open: c?.nodes.filter(own) ?? [], done: c?.done.filter(own) ?? [] };
	};
	const rows: TreeRow[] = [];
	const walk = (label: string, depth: number, indent: number) => {
		const { open, done } = childrenOf(label, depth);
		for (const n of open) {
			const below = childrenOf(n.label, depth + 1);
			const row: TreeRow = { node: n, indent, parent: below.open.length + below.done.length > 0 };
			rows.push(row);
			if (n.kind !== 'levels') walk(n.label, depth + 1, indent + 1);
		}
		if (f.hideDone && done.length) rows.push({ done, indent, parent: false });
		else for (const n of done) rows.push({ node: n, indent, parent: false });
	};
	walk(g.focus.label, 1, 0);
	return rows;
}

/** A entidade que um cartão abre no painel: a própria, ou o primeiro nível aberto de um grupo. */
export function nodeRef(n: GraphNode): string | undefined {
	if (n.ref) return n.ref;
	if (n.kind === 'levels' && n.levels?.length) {
		const l = n.levels.find((x) => x.verdict !== 'met') ?? n.levels[0];
		return l.key.split('>=')[0];
	}
	return undefined;
}
