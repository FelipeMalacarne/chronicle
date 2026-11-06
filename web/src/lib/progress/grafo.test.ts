import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildGraphView, buildTree, nodeLabel, unlockView, type GraphOptions } from './grafo.ts';
import type { Graph, GraphNode } from './types.ts';

const node = (n: Partial<GraphNode> & Pick<GraphNode, 'key' | 'label' | 'depth'>): GraphNode => ({
	verdict: 'unmet',
	group: 'blocked',
	kind: n.key.startsWith('skill:') ? 'skill' : n.key.startsWith('qp') ? 'qp' : 'quest',
	ref: n.key.startsWith('qp') ? undefined : n.key.split(/[@>]/)[0],
	...n
});

/* Legends' Quest pede Heroes' (disponível), Underground Pass (falta Biohazard), Family Crest
 * (feita) e três níveis; Underground Pass pede Biohazard, que pede Plague City. */
const graph: Graph = {
	focus: 'quest:legends_quest@finished',
	label: 'Legends’ Quest',
	kind: 'quest',
	depth: 3,
	max_depth: 3,
	hidden: 0,
	requirements: true,
	met: 1,
	total: 8,
	presumed: 0,
	critical: [
		{ key: 'quest:legends_quest@finished', label: 'Legends’ Quest' },
		{ key: 'quest:underground_pass@finished', label: 'Underground Pass' },
		{ key: 'quest:biohazard@finished', label: 'Biohazard' },
		{ key: 'quest:plague_city@finished', label: 'Plague City' }
	],
	nodes: [
		node({
			key: 'quest:plague_city@finished',
			label: 'Plague City',
			depth: 3,
			group: 'available',
			for: ['quest:biohazard@finished'],
			critical: true,
			step: {
				guide_id: 'oziris-main',
				step_id: '1.1.150',
				ref: '1.1.150',
				title: 'Plague City',
				section_id: '1.1',
				section_pos: 150
			}
		}),
		node({
			key: 'quest:biohazard@finished',
			label: 'Biohazard',
			depth: 2,
			requires: ['quest:plague_city@finished'],
			missing: ['quest:plague_city@finished'],
			for: ['quest:underground_pass@finished'],
			critical: true
		}),
		node({
			key: 'quest:heroes_quest@finished',
			label: 'Heroes’ Quest',
			depth: 1,
			group: 'available',
			for: ['quest:legends_quest@finished']
		}),
		node({
			key: 'quest:underground_pass@finished',
			label: 'Underground Pass',
			depth: 1,
			requires: ['quest:biohazard@finished'],
			missing: ['quest:biohazard@finished'],
			for: ['quest:legends_quest@finished'],
			critical: true
		}),
		node({
			key: 'quest:family_crest@finished',
			label: 'Family Crest',
			depth: 1,
			verdict: 'met',
			group: 'done',
			for: ['quest:legends_quest@finished']
		}),
		node({
			key: 'skill:agility>=50',
			label: 'Agility',
			depth: 1,
			have: 31,
			for: ['quest:legends_quest@finished']
		}),
		node({
			key: 'skill:crafting>=50',
			label: 'Crafting',
			depth: 1,
			have: 35,
			for: ['quest:legends_quest@finished']
		}),
		node({
			key: 'skill:herblore>=45',
			label: 'Herblore',
			depth: 1,
			verdict: 'unknown',
			group: 'unknown',
			for: ['quest:legends_quest@finished', 'quest:biohazard@finished']
		}),
		node({
			key: 'quest:legends_quest@finished',
			label: 'Legends’ Quest',
			depth: 0,
			critical: true,
			requires: [
				'quest:heroes_quest@finished',
				'quest:underground_pass@finished',
				'quest:family_crest@finished',
				'skill:agility>=50',
				'skill:crafting>=50',
				'skill:herblore>=45'
			],
			missing: [
				'quest:heroes_quest@finished',
				'quest:underground_pass@finished',
				'skill:agility>=50',
				'skill:crafting>=50',
				'skill:herblore>=45'
			]
		})
	],
	unlocks: [
		{
			key: 'quest:rfd_sir_amik@finished',
			ref: 'quest:rfd_sir_amik',
			label: 'RFD · Sir Amik Varze',
			kind: 'quest',
			verdict: 'unmet',
			missing: 1
		},
		{
			key: 'quest:x@finished',
			ref: 'quest:x',
			label: 'X',
			kind: 'quest',
			verdict: 'unmet',
			missing: 0
		},
		{
			key: 'quest:y@finished',
			ref: 'quest:y',
			label: 'Y',
			kind: 'quest',
			verdict: 'met',
			missing: 0
		}
	],
	unlocks_total: 3,
	bottlenecks: [{ key: 'skill:agility>=50', skill: 'agility', level: 31, target: 50, met: false }],
	routes: []
};

const opts: GraphOptions = {
	hideDone: true,
	criticalOnly: false,
	kind: 'tudo',
	routeName: (g) => (g.startsWith('oziris') ? 'Oziris' : 'BRUHsailer'),
	cursorOf: () => undefined
};

test('nodeLabel names levels and quest points by their target, and shortens RFD', () => {
	assert.equal(nodeLabel({ key: 'skill:agility>=50', label: 'Agility' }), '50 Agility');
	assert.equal(
		nodeLabel({ key: 'quest:x@finished', label: 'Recipe for Disaster/Freeing Sir Amik Varze' }),
		'RFD · Sir Amik Varze'
	);
	assert.equal(
		nodeLabel({
			key: 'quest:x@finished',
			label: 'Recipe for Disaster/Freeing the Lumbridge Guide'
		}),
		'RFD · Lumbridge Guide'
	);
	assert.equal(
		nodeLabel({ key: 'quest:x@finished', label: "Recipe for Disaster/Another Cook's Quest" }),
		"RFD · Another Cook's Quest"
	);
	assert.equal(nodeLabel({ key: 'qp>=175', label: 'quest points' }), '175 quest points');
	assert.equal(nodeLabel({ key: 'quest:biohazard@finished', label: 'Biohazard' }), 'Biohazard');
});

test('columns go from the deepest to depth 1 and collapse what is done', () => {
	const v = buildGraphView(graph, opts);
	assert.deepEqual(
		v.columns.map((c) => c.title),
		['Requer · nível 3', 'Requer · nível 2', 'Requer · nível 1']
	);
	const [d3, d2, d1] = v.columns;
	assert.equal(d3.nodes[0].sub, 'p/ Biohazard · Oziris 1.1.150');
	assert.equal(d3.nodes[0].vcls, 'v-unmet');
	assert.equal(d2.nodes[0].sub, 'p/ Underground Pass · falta Plague City');
	assert.equal(d2.nodes[0].vcls, 'v-lock');
	assert.deepEqual(
		d1.nodes.map((n) => [n.label, n.sub, n.vcls, n.critical]),
		[
			['Heroes’ Quest', 'p/ Legends’ Quest · disponível', 'v-unmet', false],
			['Underground Pass', 'p/ Legends’ Quest · falta Biohazard', 'v-lock', true],
			['50 Agility · 50 Crafting', 'p/ Legends’ Quest · 2 níveis', 'v-unmet', false],
			// Pedido por dois nós: fica sozinho, e "não sabemos" nunca vira não feito.
			['45 Herblore', 'p/ Legends’ Quest +1 · não sabemos', 'v-unk', false]
		]
	);
	assert.deepEqual(d1.done, { count: 1, names: 'Family Crest' });
	assert.equal(v.collapsed, 1);
	assert.equal(v.nodes, 9);
	assert.equal(v.focus.label, 'Legends’ Quest');
	assert.equal(v.focus.vcls, 'v-prog');
	assert.equal(v.focus.sub, '1 de 8 · gargalo 50 Agility · caminho crítico: Plague City');
});

test('filters: done listed, only the critical path, one kind', () => {
	const all = buildGraphView(graph, { ...opts, hideDone: false });
	assert.ok(all.columns[2].nodes.some((n) => n.label === 'Family Crest' && n.vcls === 'v-met'));
	assert.equal(all.columns[2].done.count, 0);
	const crit = buildGraphView(graph, { ...opts, criticalOnly: true });
	assert.deepEqual(
		crit.columns.map((c) => c.nodes.map((n) => n.label)),
		[['Plague City'], ['Biohazard'], ['Underground Pass']]
	);
	const levels = buildGraphView(graph, { ...opts, kind: 'level' });
	assert.deepEqual(
		levels.columns[2].nodes.map((n) => n.label),
		['50 Agility · 50 Crafting', '45 Herblore']
	);
	assert.equal(levels.columns[0].nodes.length, 0);
});

test('"você está aqui" when the step is the route cursor', () => {
	const v = buildGraphView(graph, {
		...opts,
		cursorOf: (g) => (g === 'oziris-main' ? '1.1.150' : undefined)
	});
	assert.equal(v.columns[0].nodes[0].sub, 'p/ Biohazard · você está aqui');
});

test('unlocks: available, blocked or done', () => {
	assert.deepEqual(
		graph.unlocks.map((u) => [unlockView(u).vcls, unlockView(u).sub]),
		[
			['v-lock', 'quest · faltam 1'],
			['v-unmet', 'quest · disponível'],
			['v-met', 'quest · feito']
		]
	);
});

test('a level that is not met is trainable now, never blocked; depth stops at the deepest', () => {
	const v = buildGraphView(
		{
			...graph,
			max_depth: 1,
			nodes: graph.nodes.filter((n) => n.depth <= 1 && n.key !== 'skill:crafting>=50')
		},
		opts
	);
	assert.equal(v.columns.length, 1);
	const agility = v.columns[0].nodes.find((n) => n.label === '50 Agility');
	assert.equal(agility?.vcls, 'v-unmet');
	assert.equal(agility?.sub, 'p/ Legends’ Quest · tem 31');
});

test('the tree nests requirements under what needs them and collapses on request', () => {
	const rows = buildTree(graph, opts);
	assert.deepEqual(
		rows.map((r) => [r.depth, r.node.label, r.node.sub, r.parent]),
		[
			[0, 'Heroes’ Quest', 'disponível', false],
			[0, 'Underground Pass', 'crítico · falta Biohazard', true],
			[1, 'Biohazard', 'crítico · falta Plague City', true],
			[2, 'Plague City', 'Oziris 1.1.150', false],
			[0, '50 Agility · 50 Crafting', '31 · 35', false],
			[0, '45 Herblore', 'não sabemos', false],
			[0, '+1 feito · Family Crest', '', false]
		]
	);
	const closed = buildTree(graph, opts, new Set([rows[1].id]));
	assert.deepEqual(
		closed.map((r) => r.node.label),
		[
			'Heroes’ Quest',
			'Underground Pass',
			'50 Agility · 50 Crafting',
			'45 Herblore',
			'+1 feito · Family Crest'
		]
	);
	const shallow = buildTree({ ...graph, depth: 1 }, opts);
	assert.ok(shallow.every((r) => r.depth === 0 && !r.parent));
});
