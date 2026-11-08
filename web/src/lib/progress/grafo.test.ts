import { test } from 'node:test';
import assert from 'node:assert/strict';
import { doneSummary, filterColumn, nodeLine, nodeRef, treeRows } from './grafo.ts';
import type { Graph, GraphNode } from './types.ts';

const route = (g: string) => (g.startsWith('oziris') ? 'Oziris' : 'BRUHsailer');
const step = (id: string, ref: string) => ({
	guide_id: 'oziris-main',
	step_id: id,
	ref,
	title: '',
	section_id: '1.1',
	section_pos: 1
});
const node = (o: Partial<GraphNode> & { key: string; label: string }): GraphNode => ({
	kind: 'quest',
	depth: 1,
	verdict: 'unmet',
	group: 'blocked',
	...o
});

test('nodeLine says who needs it, then the state or the route step', () => {
	assert.equal(
		nodeLine(
			node({
				key: 'a',
				label: 'Plague City',
				for: ['Biohazard'],
				group: 'available',
				step: step('1.1.150', '1.1.150')
			}),
			route
		),
		'p/ Biohazard · disponível · Oziris 1.1.150'
	);
	assert.equal(
		nodeLine(
			node({ key: 'b', label: 'Underground Pass', for: ["Legends'"], missing: ['Biohazard'] }),
			route
		),
		"p/ Legends' · falta Biohazard"
	);
	assert.equal(
		nodeLine(
			node({ key: 'c', label: 'Grand Tree', group: 'unknown', step: step('x9', '1.3.2') }),
			route,
			'x9'
		),
		'você está aqui'
	);
	assert.equal(
		nodeLine(
			node({
				key: 'levels:q',
				label: '50 Agility · 45 Herblore',
				kind: 'levels',
				for: ["Legends'"],
				levels: [
					{ key: 'skill:agility>=50', skill: 'Agility', target: 50, verdict: 'unmet' },
					{ key: 'skill:herblore>=45', skill: 'Herblore', target: 45, verdict: 'met' }
				]
			}),
			route
		),
		"p/ Legends' · 1 de 2 níveis"
	);
});

test('filters keep the kind, the critical path and set the done ones apart', () => {
	const col = {
		depth: 1,
		nodes: [
			node({ key: 'q', label: 'Q', critical: true }),
			node({ key: 'levels:q', label: 'L', kind: 'levels' }),
			node({ key: 'qp>=10', label: '10 quest points', kind: 'qp' })
		],
		done: [node({ key: 'd', label: 'D', group: 'done', verdict: 'met' })]
	};
	const lv = filterColumn(col, { hideDone: true, criticalOnly: false, kind: 'level' });
	assert.deepEqual(
		lv.nodes.map((n) => n.key),
		['levels:q', 'qp>=10']
	);
	const crit = filterColumn(col, { hideDone: false, criticalOnly: true, kind: 'tudo' });
	assert.deepEqual(
		crit.nodes.map((n) => n.key),
		['q']
	);
	assert.equal(crit.done.length, 0);
});

test('doneSummary names three and elides the rest', () => {
	const d = ['A', 'B', 'C', 'D'].map((l) => node({ key: l, label: l, group: 'done' }));
	assert.deepEqual(doneSummary(d), { count: '+4 feitos', names: 'A · B · C · …' });
	assert.deepEqual(doneSummary(d.slice(0, 1)), { count: '+1 feito', names: 'A' });
});

test('treeRows nests each node under the one it is for and collapses the done ones', () => {
	const g: Graph = {
		focus: node({ key: 'f', label: 'Legends', depth: 0 }),
		depth: 2,
		max_depth: 2,
		critical: [],
		nodes: 4,
		done_nodes: 1,
		routes: [],
		unlocks: [],
		columns: [
			{
				depth: 1,
				nodes: [node({ key: 'h', label: 'Heroes', for: ['Legends'] })],
				done: [node({ key: 'w', label: 'Waterfall', for: ['Legends'], group: 'done' })]
			},
			{
				depth: 2,
				nodes: [node({ key: 's', label: 'Shield', depth: 2, for: ['Heroes'] })],
				done: []
			}
		]
	};
	const rows = treeRows(g, { hideDone: true, criticalOnly: false, kind: 'tudo' });
	assert.deepEqual(
		rows.map((r) => `${r.indent}:${r.node?.label ?? `+${r.done?.length}`}:${r.parent}`),
		['0:Heroes:true', '1:Shield:false', '0:+1:false']
	);
});

test('nodeRef opens the entity, or the first open level of a group', () => {
	assert.equal(nodeRef(node({ key: 'q', label: 'Q', ref: 'quest:x' })), 'quest:x');
	assert.equal(
		nodeRef(
			node({
				key: 'levels:q',
				label: 'L',
				kind: 'levels',
				levels: [
					{ key: 'skill:herblore>=45', skill: 'Herblore', target: 45, verdict: 'met' },
					{ key: 'skill:agility>=50', skill: 'Agility', target: 50, verdict: 'unmet' }
				]
			})
		),
		'skill:agility'
	);
});
