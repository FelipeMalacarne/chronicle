/**
 * Algoritmos puros sobre o grafo de entidades: estado derivado, recorte local (ego graph),
 * plano ordenado de uma meta e layout em camadas para desenhar o recorte.
 */
import type { EdgeVM, EntityGraph, EntityState, EntityVM } from './types.ts';

export interface Adjacency {
	/** requisitos diretos de cada nó */
	reqs: Map<string, string[]>;
	/** o que cada nó destrava diretamente */
	unlocks: Map<string, string[]>;
}

export function adjacency(edges: readonly EdgeVM[]): Adjacency {
	const reqs = new Map<string, string[]>();
	const unlocks = new Map<string, string[]>();
	for (const e of edges) {
		(reqs.get(e.to) ?? reqs.set(e.to, []).get(e.to)!).push(e.from);
		(unlocks.get(e.from) ?? unlocks.set(e.from, []).get(e.from)!).push(e.to);
	}
	return { reqs, unlocks };
}

/** done/in_progress vêm da conta; o resto é `available` se todos os requisitos estão feitos. */
export function deriveStates(g: EntityGraph): Record<string, EntityState> {
	const { reqs } = adjacency(g.edges);
	const out: Record<string, EntityState> = {};
	for (const e of Object.values(g.entities)) {
		if (e.observed === 'done') out[e.id] = 'done';
		else if (e.observed === 'in_progress') out[e.id] = 'in_progress';
		else {
			const r = reqs.get(e.id) ?? [];
			out[e.id] = r.every((x) => g.entities[x]?.observed === 'done') ? 'available' : 'blocked';
		}
	}
	return out;
}

/** Fecho transitivo de requisitos (sem o próprio nó). */
export function closure(id: string, adj: Adjacency): Set<string> {
	const seen = new Set<string>();
	const stack = [...(adj.reqs.get(id) ?? [])];
	while (stack.length) {
		const n = stack.pop()!;
		if (seen.has(n)) continue;
		seen.add(n);
		stack.push(...(adj.reqs.get(n) ?? []));
	}
	return seen;
}

export interface GoalPlan {
	/** Requisitos pendentes em ordem executável (pré-requisitos antes). */
	order: string[];
	total: number;
	done: number;
	/** Requisito pendente mais "fundo": o que mais segura a meta. */
	bottleneck: string | null;
}

/**
 * Plano de uma meta: ordem topológica dos requisitos pendentes. Empates seguem `priority`
 * (posição na rota ativa), que é a heurística de eficiência do guia; sem posição vai ao fim.
 */
export function goalPlan(
	target: string,
	g: EntityGraph,
	priority: (id: string) => number = () => Number.POSITIVE_INFINITY
): GoalPlan {
	const adj = adjacency(g.edges);
	const all = closure(target, adj);
	all.add(target);
	const pending = [...all].filter((id) => g.entities[id]?.observed !== 'done');
	const pendingSet = new Set(pending);
	const indeg = new Map(pending.map((id) => [id, 0]));
	for (const id of pending)
		for (const r of adj.reqs.get(id) ?? [])
			if (pendingSet.has(r)) indeg.set(id, indeg.get(id)! + 1);

	// profundidade = maior cadeia de requisitos pendentes abaixo do nó
	const depth = new Map<string, number>();
	const depthOf = (id: string): number => {
		if (depth.has(id)) return depth.get(id)!;
		depth.set(id, 0);
		const d = Math.max(
			0,
			...(adj.reqs.get(id) ?? []).filter((r) => pendingSet.has(r)).map((r) => depthOf(r) + 1)
		);
		depth.set(id, d);
		return d;
	};

	const ready = pending.filter((id) => indeg.get(id) === 0);
	const order: string[] = [];
	const byPriority = (a: string, b: string) => priority(a) - priority(b) || a.localeCompare(b);
	while (ready.length) {
		ready.sort(byPriority);
		const n = ready.shift()!;
		order.push(n);
		for (const u of adj.unlocks.get(n) ?? []) {
			if (!indeg.has(u)) continue;
			indeg.set(u, indeg.get(u)! - 1);
			if (indeg.get(u) === 0) ready.push(u);
		}
	}
	const reqOnly = order.filter((id) => id !== target);
	let bottleneck: string | null = null;
	let best = -1;
	for (const id of reqOnly) {
		const e = g.entities[id];
		// skill distante pesa pela distância de levels; quest pela cadeia abaixo dela
		const gap = e?.kind === 'skill' && e.level && e.current ? (e.level - e.current) / 5 : 0;
		const score = depthOf(id) + gap;
		if (adj.reqs.get(id)?.some((r) => pendingSet.has(r)) || gap > 0) {
			if (score > best) {
				best = score;
				bottleneck = id;
			}
		}
	}
	const total = all.size - 1;
	return { order: reqOnly, total, done: total - reqOnly.length, bottleneck };
}

export interface EgoOptions {
	up?: number;
	down?: number;
	/** Requisitos feitos viram um contador por nó, em vez de nós. */
	hideDone?: boolean;
}

export interface EgoGraph {
	focus: string;
	nodes: string[];
	edges: EdgeVM[];
	/** Para cada nó visível: quantos requisitos feitos foram escondidos. */
	hiddenDone: Record<string, number>;
	/** Nós na borda do recorte que têm mais vizinhos fora dele. */
	frontier: Set<string>;
}

/** Recorte local: `up` níveis de requisitos e `down` níveis do que o foco destrava. */
export function egoGraph(focus: string, g: EntityGraph, opts: EgoOptions = {}): EgoGraph {
	const { up = 2, down = 1, hideDone = true } = opts;
	const adj = adjacency(g.edges);
	const keep = new Set([focus]);
	const hiddenDone: Record<string, number> = {};
	const frontier = new Set<string>();
	let layer = [focus];
	for (let d = 0; d < up; d++) {
		const next: string[] = [];
		for (const n of layer) {
			for (const r of adj.reqs.get(n) ?? []) {
				if (hideDone && g.entities[r]?.observed === 'done' && r !== focus) {
					hiddenDone[n] = (hiddenDone[n] ?? 0) + 1;
					continue;
				}
				if (!keep.has(r)) {
					keep.add(r);
					next.push(r);
				}
			}
		}
		layer = next;
	}
	for (const n of layer) if ((adj.reqs.get(n) ?? []).length) frontier.add(n);
	layer = [focus];
	for (let d = 0; d < down; d++) {
		const next: string[] = [];
		for (const n of layer)
			for (const u of adj.unlocks.get(n) ?? [])
				if (!keep.has(u)) {
					keep.add(u);
					next.push(u);
				}
		layer = next;
	}
	for (const n of layer) if ((adj.unlocks.get(n) ?? []).length) frontier.add(n);
	const edges = g.edges.filter((e) => keep.has(e.from) && keep.has(e.to));
	return { focus, nodes: [...keep], edges, hiddenDone, frontier };
}

export interface LayoutNode {
	id: string;
	rank: number;
	order: number;
	x: number;
	y: number;
}

export interface Layout {
	nodes: Record<string, LayoutNode>;
	width: number;
	height: number;
	ranks: number;
}

/**
 * Layout em camadas (Sugiyama simplificado): rank = maior caminho a partir das fontes,
 * ordem dentro do rank por baricentro em varreduras alternadas, coordenadas em grade.
 * Requisitos ficam à esquerda; o que é destravado, à direita.
 */
export function layeredLayout(
	nodes: readonly string[],
	edges: readonly EdgeVM[],
	opts: { colWidth?: number; rowHeight?: number; sweeps?: number } = {}
): Layout {
	const { colWidth = 240, rowHeight = 72, sweeps = 4 } = opts;
	const set = new Set(nodes);
	const es = edges.filter((e) => set.has(e.from) && set.has(e.to));
	const preds = new Map<string, string[]>(nodes.map((n) => [n, []]));
	const succs = new Map<string, string[]>(nodes.map((n) => [n, []]));
	for (const e of es) {
		preds.get(e.to)!.push(e.from);
		succs.get(e.from)!.push(e.to);
	}
	const rank = new Map<string, number>();
	const visiting = new Set<string>();
	const rankOf = (n: string): number => {
		if (rank.has(n)) return rank.get(n)!;
		if (visiting.has(n)) return 0; // ciclo: corta
		visiting.add(n);
		const r = Math.max(-1, ...preds.get(n)!.map(rankOf)) + 1;
		visiting.delete(n);
		rank.set(n, r);
		return r;
	};
	nodes.forEach(rankOf);
	const ranks = Math.max(0, ...rank.values()) + 1;
	const layers: string[][] = Array.from({ length: ranks }, () => []);
	for (const n of nodes) layers[rank.get(n)!].push(n);
	for (const l of layers) l.sort();

	const pos = new Map<string, number>();
	const index = () => layers.forEach((l) => l.forEach((n, i) => pos.set(n, i)));
	index();
	const bary = (n: string, side: Map<string, string[]>) => {
		const ns = side.get(n)!;
		return ns.length ? ns.reduce((s, x) => s + pos.get(x)!, 0) / ns.length : pos.get(n)!;
	};
	for (let s = 0; s < sweeps; s++) {
		const down = s % 2 === 0;
		const seq = down ? layers.slice(1) : layers.slice(0, -1).reverse();
		for (const l of seq) {
			const b = new Map(l.map((n) => [n, bary(n, down ? preds : succs)]));
			l.sort((a, z) => b.get(a)! - b.get(z)! || a.localeCompare(z));
			l.forEach((n, i) => pos.set(n, i));
		}
	}
	const tallest = Math.max(1, ...layers.map((l) => l.length));
	const out: Record<string, LayoutNode> = {};
	layers.forEach((l, r) => {
		const offset = ((tallest - l.length) * rowHeight) / 2;
		l.forEach((n, i) => {
			out[n] = { id: n, rank: r, order: i, x: r * colWidth, y: offset + i * rowHeight };
		});
	});
	return { nodes: out, width: ranks * colWidth, height: tallest * rowHeight, ranks };
}

/** Contagem de cruzamentos entre camadas adjacentes (métrica de qualidade do layout). */
export function crossings(layout: Layout, edges: readonly EdgeVM[]): number {
	const es = edges
		.map((e) => [layout.nodes[e.from], layout.nodes[e.to]] as const)
		.filter(([a, b]) => a && b && b.rank === a.rank + 1);
	let c = 0;
	for (let i = 0; i < es.length; i++)
		for (let j = i + 1; j < es.length; j++) {
			const [a1, b1] = es[i];
			const [a2, b2] = es[j];
			if (a1.rank !== a2.rank) continue;
			if ((a1.order - a2.order) * (b1.order - b2.order) < 0) c++;
		}
	return c;
}

export function kindLabel(kind: EntityVM['kind']): string {
	return {
		quest: 'Quest',
		skill: 'Level',
		item: 'Item',
		diary: 'Diary',
		unlock: 'Desbloqueio',
		boss: 'Boss',
		goal: 'Meta'
	}[kind];
}
