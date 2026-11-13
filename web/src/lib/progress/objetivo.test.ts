import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	actionsSummary,
	countLabel,
	goalGroups,
	goalName,
	goalView,
	nextAction
} from './objetivo.ts';
import { chipsOf } from './view.ts';
import type { BoardObjective, ContentStep, Role } from './types.ts';

const step = (objs: [string, Role][]) =>
	({
		objectives: objs.map(([key, role]) => ({ key, role })),
		names: {
			'item:561': 'Nature rune',
			'item:562': 'Chaos rune',
			'quest:a_ruff_situation': 'A Ruff Situation',
			'skill:magic': 'Magic'
		}
	}) as Pick<ContentStep, 'objectives' | 'names'>;

test('countLabel writes pt-BR numbers and shortens the big ones', () => {
	assert.equal(countLabel(240), '240');
	assert.equal(countLabel(1250), '1.250');
	assert.equal(countLabel(99_999), '99.999');
	assert.equal(countLabel(784_932), '784k');
	assert.equal(countLabel(1_250_000), '1,25M');
	assert.equal(countLabel(1_594_538), '1,59M');
	assert.equal(countLabel(2_000_000), '2M');
	assert.equal(countLabel(12_400_000), '12M');
});

test('goalName drops the count in front only where the number goes to the bar', () => {
	assert.equal(goalName({ key: 'item:561x1250', label: '1250 Nature rune' }), 'Nature rune');
	assert.equal(goalName({ key: 'skill:magic>=43', label: '43 Magic' }), 'Magic');
	assert.equal(goalName({ key: 'qp>=107', label: '107 quest points' }), 'Quest points');
	assert.equal(
		goalName({ key: 'quest:a_ruff_situation@started', label: 'A Ruff Situation' }),
		'A Ruff Situation'
	);
	assert.equal(
		goalName({ key: 'item:any(1355x2|1357x1)', label: '2 Mithril axe ou Adamant axe' }),
		'2 Mithril axe ou Adamant axe'
	);
});

test('goalView shows how much is missing when the board knows the number', () => {
	const [nature] = chipsOf(step([['item:561x1250', 'completes']]), {
		objectives: [{ key: 'item:561x1250', role: 'completes', verdict: 'unmet' }]
	});
	const g = goalView(nature, { value: 240, target: 1250 });
	assert.equal(g.name, 'Nature rune');
	assert.equal(g.text, '240 / 1.250');
	assert.equal(g.pct, 19);
	assert.equal(g.cls, '');
});

test('goalView never fills the bar of what is missing, even past the target', () => {
	const [nature] = chipsOf(step([['item:561x1250', 'completes']]), {
		objectives: [{ key: 'item:561x1250', role: 'completes', verdict: 'unmet' }]
	});
	assert.equal(goalView(nature, { value: 1300, target: 1250 }).pct, 99);
});

test('goalView: met is "feito", unknown stays "não sabemos" and never takes a number', () => {
	const chips = chipsOf(
		step([
			['item:562x500', 'completes'],
			['quest:a_ruff_situation@started', 'starts']
		]),
		{ objectives: [{ key: 'item:562x500', role: 'completes', verdict: 'met' }] }
	);
	const met = goalView(chips[0], { value: 500, target: 500 });
	assert.deepEqual([met.cls, met.pct, met.text, met.name], ['met', 100, 'feito', 'Chaos rune']);
	const unk = goalView(chips[1], { value: 0, target: 1 });
	assert.deepEqual([unk.cls, unk.pct, unk.text], ['unk', 0, 'não sabemos']);
});

test('goalView without a number: "falta" with the full label', () => {
	const [quest] = chipsOf(step([['quest:a_ruff_situation@started', 'starts']]), {
		objectives: [{ key: 'quest:a_ruff_situation@started', role: 'starts', verdict: 'unmet' }]
	});
	const g = goalView(quest);
	assert.deepEqual([g.name, g.text, g.pct, g.cls], ['A Ruff Situation', 'falta', 0, '']);
});

test('goalGroups orders the roles and matches each observed value to its role and key', () => {
	const objectives: BoardObjective[] = [
		{ key: 'quest:a_ruff_situation@started', role: 'starts', verdict: 'unmet' },
		{
			key: 'item:561x1250',
			role: 'completes',
			verdict: 'unmet',
			observed: { value: 240, target: 1250 }
		},
		{ key: 'skill:magic>=43', role: 'requires', verdict: 'unknown' }
	];
	const s = step([
		['quest:a_ruff_situation@started', 'starts'],
		['item:561x1250', 'completes'],
		['skill:magic>=43', 'requires']
	]);
	const groups = goalGroups(chipsOf(s, { objectives }), objectives);
	assert.deepEqual(
		groups.map((g) => [g.label, g.goals.map((x) => x.text)]),
		[
			['Conclui', ['240 / 1.250']],
			['Inicia', ['falta']],
			['Requer', ['não sabemos']]
		]
	);
});

test('nextAction is the first action still missing; actionsSummary counts them', () => {
	const acts = [
		{ state: 'plain' as const },
		{ state: 'done' as const },
		{ state: 'open' as const },
		{ state: 'open' as const },
		{ state: 'unknown' as const }
	];
	assert.equal(nextAction(acts), 2);
	assert.equal(nextAction([{ state: 'done' as const }]), -1);
	assert.equal(actionsSummary(acts), 'Passo a passo · 5 ações · 2 com o que falta');
	assert.equal(actionsSummary([{ state: 'plain' as const }]), 'Passo a passo · 1 ação');
});

test('goalView says an owner-marked objective was marked by the owner', () => {
	const st = step([['skill:magic>=50', 'completes']]);
	const [marked] = chipsOf(st, {
		objectives: [{ key: 'skill:magic>=50', role: 'completes', verdict: 'met', owner_marked: true }]
	});
	const [evidence] = chipsOf(st, {
		objectives: [{ key: 'skill:magic>=50', role: 'completes', verdict: 'met' }]
	});
	assert.equal(goalView(marked).ownerMarked, true);
	assert.match(goalView(marked).title, /marcado por você/);
	assert.equal(goalView(evidence).ownerMarked, false);
	assert.doesNotMatch(goalView(evidence).title, /marcado por você/);
});
