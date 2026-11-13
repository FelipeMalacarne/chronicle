import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
	closeness,
	closenessLabel,
	fleetRows,
	goalLine,
	goalTitle,
	kindTag,
	listJoin,
	missingLine,
	needRows,
	needsAccount,
	routeBlocks
} from './mundo.ts';
import type {
	AccountView,
	FleetGoal,
	FleetStanding,
	GoalNode,
	GoalPlan,
	StepPlace
} from './types.ts';

const acc = (id: string, rsn: string, guide = 'bruhsailer-main'): AccountView => ({
	id,
	rsn,
	mode: 'ironman',
	guide_id: guide,
	layer: 'enhanced'
});
const accounts = [acc('a', 'Iron A'), acc('b', 'Iron B'), acc('c', 'Iron C', 'oziris-main')];

const place = (ref: string, guide = 'bruhsailer-main', done = false): StepPlace => ({
	guide_id: guide,
	step_id: `s-${ref}`,
	ref,
	title: `passo ${ref}`,
	section_id: 'sec',
	section_pos: 1,
	...(done ? { done } : {})
});

function standing(account: string, p: Partial<FleetStanding> = {}): FleetStanding {
	return {
		account,
		verdict: 'unmet',
		requirements: { met: 0, presumed: 0, unknown: 0, total: 0, quests_left: 0 },
		short: [],
		...p
	};
}

const pinned = {
	id: 'goal_1',
	label: '',
	priority: 100,
	status: 'active' as const,
	created_at: '2026-11-13T00:00:00Z'
};

function goal(p: Partial<FleetGoal> = {}): FleetGoal {
	return {
		objective_key: 'quest:recipe_for_disaster@finished',
		name: 'Recipe for Disaster',
		ref: 'quest:recipe_for_disaster',
		kind: 'quest',
		pinned: 0,
		met: 0,
		unknown: 0,
		accounts: [],
		routes: [],
		...p
	};
}

test('goalTitle: nível como no artboard, o resto pelo rótulo do objetivo', () => {
	assert.equal(
		goalTitle({ objective_key: 'skill:fishing>=99', name: 'Fishing', ref: 'skill:fishing' }),
		'Fishing 99'
	);
	assert.equal(
		goalTitle({
			objective_key: 'quest:dragon_slayer_i@finished',
			name: 'Dragon Slayer I',
			ref: 'quest:dragon_slayer_i'
		}),
		'Dragon Slayer I'
	);
	assert.equal(goalTitle({ objective_key: 'qp>=175', name: '', ref: '' }), '175 quest points');
	assert.equal(kindTag('skill'), 'skill');
	assert.equal(kindTag('quest'), 'quest');
});

test('listJoin', () => {
	assert.equal(listJoin([]), '');
	assert.equal(listJoin(['a']), 'a');
	assert.equal(listJoin(['a', 'b']), 'a e b');
	assert.equal(listJoin(['a', 'b', 'c']), 'a, b e c');
});

test('closeness: requisitos feitos, nível numa meta de nível, nada sem medida', () => {
	const g = goal();
	const s = standing('a', {
		requirements: { met: 6, presumed: 0, unknown: 1, total: 10, quests_left: 3 }
	});
	assert.deepEqual(closeness(g, s), { pct: 60 });
	assert.equal(closenessLabel({ pct: 60 }), '60%');
	assert.equal(closeness(g, standing('a')), null, 'sem requisitos: sem medida');
	assert.deepEqual(closeness(g, standing('a', { verdict: 'met' })), { pct: 100 });

	const lvl = goal({ objective_key: 'skill:fishing>=99', kind: 'skill' });
	const c = closeness(lvl, standing('a', { level: 76 }));
	assert.deepEqual(c, { pct: 77, level: 76, target: 99 });
	assert.equal(closenessLabel(c!), '76/99');
	assert.equal(closeness(lvl, standing('a')), null, 'nível sem leitura');
});

test('goalLine: quem persegue e a mais perto, ou quantas têm', () => {
	const two = goal({
		closest: 'a',
		accounts: [
			standing('a', {
				goal: pinned,
				requirements: { met: 61, presumed: 0, unknown: 0, total: 100, quests_left: 0 }
			}),
			standing('b', {
				goal: pinned,
				requirements: { met: 38, presumed: 0, unknown: 0, total: 100, quests_left: 0 }
			}),
			standing('c')
		]
	});
	assert.equal(goalLine(two, 3), '2 contas · a mais perto em 61%');

	const one = goal({
		objective_key: 'skill:fishing>=99',
		kind: 'skill',
		closest: 'a',
		accounts: [standing('a', { goal: pinned, level: 76 }), standing('b')]
	});
	assert.equal(goalLine(one, 3), '1 conta · nível 76');

	const all = goal({
		met: 3,
		accounts: [
			standing('a', { goal: pinned, verdict: 'met' }),
			standing('b', { verdict: 'met' }),
			standing('c', { verdict: 'met' })
		]
	});
	assert.equal(goalLine(all, 5), '3 de 5 contas têm');

	const item = goal({ accounts: [standing('a', { goal: pinned, verdict: 'unknown' })] });
	assert.equal(goalLine(item, 2), '1 conta', 'meta de item sem medida: só quem persegue');
});

test('missingLine: níveis e quests que faltam; desconhecido nunca vira falta', () => {
	const s = standing('a', {
		short: [
			{ key: 'qp>=175', skill: 'qp', level: null, target: 175 },
			{ key: 'skill:agility>=70', skill: 'agility', level: 62, target: 70 }
		],
		requirements: { met: 2, presumed: 0, unknown: 1, total: 9, quests_left: 5 }
	});
	assert.equal(missingLine(s), 'falta 175 QP, 70 Agility (tem 62) e 5 quests');
	assert.equal(missingLine(standing('a', { verdict: 'met' })), 'já tem');
	assert.equal(
		missingLine(
			standing('a', {
				verdict: 'unknown',
				requirements: { met: 0, presumed: 0, unknown: 4, total: 4, quests_left: 0 }
			})
		),
		'não sabemos os requisitos'
	);
	assert.equal(
		missingLine(
			standing('a', {
				requirements: { met: 2, presumed: 0, unknown: 1, total: 3, quests_left: 0 }
			})
		),
		'não sabemos 1 requisito'
	);
	assert.equal(missingLine(standing('a', { verdict: 'unknown' })), 'não sabemos');
	assert.equal(
		missingLine(
			standing('a', {
				requirements: { met: 3, presumed: 1, unknown: 0, total: 4, quests_left: 0 }
			})
		),
		'requisitos feitos'
	);
	assert.equal(missingLine(standing('a')), '', 'sem requisitos (item) nem leitura: a linha some');
});

test('fleetRows: quem persegue primeiro, da mais perto à mais longe, quem já tem depois', () => {
	const req = (met: number) => ({ met, presumed: 0, unknown: 0, total: 10, quests_left: 0 });
	const g = goal({
		accounts: [
			standing('a', { goal: pinned, requirements: req(3) }),
			standing('b', { goal: pinned, requirements: req(6) }),
			standing('c', { requirements: req(9) }),
			standing('d', { goal: pinned, verdict: 'met' }),
			standing('e', { goal: pinned })
		]
	});
	const rows = fleetRows(g, [...accounts, acc('d', 'Iron D'), acc('e', 'Iron E')]);
	assert.deepEqual(
		rows.map((r) => r.account),
		['b', 'a', 'e', 'd', 'c']
	);
	assert.equal(rows[0].rsn, 'Iron B');
	assert.equal(rows[0].pursues, true);
	assert.equal(rows[4].pursues, false);
	assert.equal(rows[2].measure, null);
});

test('needsAccount: a pedida, a mais perto, a primeira que ainda não tem', () => {
	const g = goal({
		closest: 'b',
		accounts: [
			standing('a', { goal: pinned, verdict: 'met' }),
			standing('b', { goal: pinned }),
			standing('c')
		]
	});
	assert.equal(needsAccount(g, 'a')?.account, 'a');
	assert.equal(needsAccount(g, 'c')?.account, 'b', 'quem não persegue não tem plano');
	assert.equal(needsAccount(g)?.account, 'b');
	assert.equal(needsAccount(goal({ accounts: [standing('c')] })), undefined);
	const noClosest = goal({
		accounts: [standing('a', { goal: pinned, verdict: 'met' }), standing('b', { goal: pinned })]
	});
	assert.equal(needsAccount(noClosest)?.account, 'b');
});

function node(key: string, p: Partial<GoalNode> = {}): GoalNode {
	return { key, label: key, depth: 1, verdict: 'unmet', group: 'available', ...p };
}

test('needRows: requisitos diretos em aberto e os deles, recuados, até o limite', () => {
	const plan: GoalPlan = {
		goal: 'quest:rfd@finished',
		requirements: true,
		met: 3,
		total: 7,
		presumed: 0,
		bottlenecks: [{ key: 'qp>=175', skill: 'qp', level: null, target: 175, met: false }],
		routes: [],
		nodes: [
			node('quest:cook@finished', { label: "Cook's Assistant", verdict: 'met', group: 'done' }),
			node('quest:a@finished', { label: 'Another Cook', depth: 2, verdict: 'met', group: 'done' }),
			node('quest:b@finished', {
				label: 'Goblin generals',
				depth: 2,
				verdict: 'unknown',
				group: 'unknown'
			}),
			node('quest:c@finished', {
				label: 'Evil Dave',
				depth: 2,
				group: 'blocked',
				missing: ['quest:shadow@finished']
			}),
			node('quest:shadow@finished', {
				label: 'Shadow of the Storm',
				depth: 3,
				group: 'available',
				step: place('CH2.10')
			}),
			node('quest:rfd_sub@finished', {
				label: 'Freeing',
				group: 'blocked',
				requires: ['quest:a@finished', 'quest:b@finished', 'quest:c@finished'],
				missing: ['quest:c@finished']
			}),
			node('qp>=175', { label: '175 quest points', group: 'blocked' }),
			node('quest:rfd@finished', { label: 'Recipe for Disaster', depth: 0, group: 'blocked' })
		]
	};
	const where = (p: StepPlace) => `BRUHsailer ${p.ref}`;
	const rows = needRows(plan, where);
	assert.deepEqual(
		rows.map((r) => [r.label, r.sub, r.mark, r.hint]),
		[
			['Freeing', false, 'prog', '1 de 3'],
			['Another Cook', true, 'met', ''],
			['Goblin generals', true, 'unk', 'não sabemos'],
			['Evil Dave', true, 'unmet', 'precisa de Shadow of the Storm'],
			['175 quest points', false, 'unmet', '']
		],
		'o requisito feito (Cook) some; o QP sem leitura fica sem número'
	);
	assert.equal(needRows(plan, where, 2).length, 2);
});

test('needRows: disponível diz onde a rota ativa o faz; meta de nível mostra o nível', () => {
	const plan: GoalPlan = {
		goal: 'quest:x@finished',
		requirements: true,
		met: 0,
		total: 2,
		presumed: 1,
		bottlenecks: [
			{ key: 'skill:agility>=70', skill: 'agility', level: 62, target: 70, met: false }
		],
		routes: [],
		nodes: [
			node('quest:y@finished', { label: 'Y', step: place('CH1.40') }),
			node('quest:z@finished', { label: 'Z', verdict: 'unknown', group: 'presumed' }),
			node('skill:agility>=70', { label: '70 Agility', group: 'blocked' })
		]
	};
	const rows = needRows(plan, (p) => `BRUHsailer ${p.ref}`);
	assert.deepEqual(
		rows.map((r) => [r.label, r.mark, r.hint]),
		[
			['Y', 'unmet', 'BRUHsailer CH1.40'],
			['70 Agility', 'unmet', '62']
		],
		'a presumida pela marca não falta'
	);

	const level: GoalPlan = {
		goal: 'skill:fishing>=99',
		requirements: false,
		met: 0,
		total: 0,
		presumed: 0,
		bottlenecks: [
			{ key: 'skill:fishing>=99', skill: 'fishing', level: 76, target: 99, met: false }
		],
		routes: [],
		nodes: [node('skill:fishing>=99', { label: '99 Fishing', depth: 0, group: 'blocked' })]
	};
	assert.deepEqual(
		needRows(level, () => '').map((r) => [r.label, r.mark, r.hint]),
		[['99 Fishing', 'unmet', '76']]
	);
	const item: GoalPlan = { ...level, bottlenecks: [], nodes: [] };
	assert.deepEqual(
		needRows(item, () => ''),
		[],
		'item sem requisitos: nada'
	);
});

test('routeBlocks: o passo de cada rota e onde cada conta está', () => {
	const done = place('CH2.72');
	const g = goal({
		routes: [
			{
				guide: 'oziris-main',
				step: 's-1.5.17',
				ref: '1.5.17',
				title: 'Finish RFD, buy B gloves',
				section: '1.5',
				total: 566,
				key: 'quest:recipe_for_disaster@finished',
				role: 'completes'
			},
			{
				guide: 'bruhsailer-main',
				step: 's-CH2.70',
				ref: 'CH2.70',
				title: 'start RFD',
				section: 'bruh-2',
				total: 226,
				key: 'quest:recipe_for_disaster@started',
				role: 'starts'
			}
		],
		accounts: [
			standing('a', { goal: pinned, step: done, steps_left: 136, cursor: place('CH1.34') }),
			standing('b', { step: done, steps_left: 1, cursor: place('CH2.71') }),
			standing('c', { verdict: 'met', step: place('1.5.17', 'oziris-main', true), steps_left: 0 })
		]
	});
	const name = (guide: string) => (guide.startsWith('oziris') ? 'Oziris' : 'BRUHsailer');
	const blocks = routeBlocks(g, accounts, name);
	assert.equal(blocks.length, 2);
	assert.equal(blocks[0].guide, 'bruhsailer-main');
	assert.equal(blocks[0].ref, 'CH2.72', 'o passo que conclui na rota ativa, não o que inicia');
	assert.equal(blocks[0].role, undefined);
	assert.deepEqual(blocks[0].lines, [
		'Iron A está em CH1.34: faltam 136 passos.',
		'Iron B está em CH2.71: falta 1 passo.'
	]);
	assert.deepEqual(blocks[1].lines, ['Iron C já tem.']);

	const nobody = routeBlocks(g, [accounts[0]], name);
	assert.equal(nobody[1].ref, '1.5.17');
	assert.deepEqual(nobody[1].lines, ['Nenhuma conta segue a Oziris agora.']);

	const starts = routeBlocks(
		goal({ routes: g.routes.slice(1), accounts: [] }),
		[accounts[2]],
		name
	);
	assert.equal(starts.length, 1, 'rota seguida sem passo some');
	assert.equal(starts[0].role, 'inicia');

	const lower = goal({
		objective_key: 'skill:fishing>=99',
		ref: 'skill:fishing',
		kind: 'skill',
		routes: [
			{ ...g.routes[0], key: 'skill:fishing>=40' },
			{ ...g.routes[1], key: 'skill:fishing>=99', role: 'completes' }
		],
		accounts: []
	});
	assert.deepEqual(
		routeBlocks(lower, [], name).map((b) => b.ref),
		['CH2.70'],
		'um passo que só chega a um nível menor não conta'
	);
});
