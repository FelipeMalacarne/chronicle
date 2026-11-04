import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	alsoIn,
	bankColumns,
	checkboxLine,
	chipsOf,
	eventLines,
	freshnessLabel,
	isParagraphStep,
	metaOf,
	objectiveLabel,
	paragraphOf,
	parseKey,
	plainNotes,
	routeLabel,
	sectionLabel,
	segmentsOf,
	statusView,
	unknownHint,
	verdictView
} from './view.ts';
import type { ContentSection, ContentStep, SegmentReadiness } from './types.ts';

const step = (id: string, extra: Partial<ContentStep> = {}): ContentStep => ({
	id,
	text: id,
	section: 's',
	guide: 'g',
	manual: true,
	ref: id,
	total: 3,
	names: {},
	...extra
});

test('parseKey reads every target of the canonical grammar', () => {
	assert.deepEqual(parseKey('quest:x_marks_the_spot@started'), {
		ref: 'quest:x_marks_the_spot',
		kind: 'quest',
		id: 'x_marks_the_spot',
		op: '@',
		state: 'started'
	});
	assert.equal(parseKey('skill:agility>=70')?.n, 70);
	assert.equal(parseKey('diary:ardougne:easy@complete')?.id, 'ardougne:easy');
	assert.deepEqual(parseKey('item:960x120'), {
		ref: 'item:960',
		kind: 'item',
		id: '960',
		op: 'x',
		n: 120
	});
	assert.equal(parseKey('qp>=107')?.kind, 'qp');
	assert.equal(parseKey('nonsense'), null);
});

test('objective labels follow the chips of the canvas', () => {
	const names = {
		'skill:woodcutting': 'Woodcutting',
		'quest:monks_friend': "Monk's Friend",
		'item:314': 'Feather'
	};
	assert.equal(objectiveLabel('skill:woodcutting>=35', names), '35 Woodcutting');
	assert.equal(objectiveLabel('quest:monks_friend@finished', names), "Monk's Friend");
	assert.equal(objectiveLabel('item:314x120', names), '120 Feather');
	assert.equal(objectiveLabel('item:314x1', names), 'Feather');
	assert.equal(objectiveLabel('qp>=107', names), '107 quest points');
	assert.equal(objectiveLabel('quest:clock_tower@finished', {}), 'Clock Tower');
});

test('unknown is never unmet', () => {
	assert.deepEqual(verdictView({ key: 'quest:a@finished', verdict: 'unknown' }), {
		cls: 'v-unk',
		glyph: '?',
		unknown: true,
		hint: 'sem faceta de quest'
	});
	assert.equal(verdictView({ key: 'skill:a>=1', verdict: 'unmet' }).cls, 'v-unmet');
	assert.equal(
		verdictView({ key: 'item:1x1', verdict: 'met', regressed: true }).hint,
		'não vale mais'
	);
});

test('chips take the verdict of the board and default to unknown', () => {
	const st = step('a', {
		objectives: [
			{ key: 'skill:agility>=10', role: 'completes' },
			{ key: 'quest:cooks_assistant@started', role: 'starts' }
		],
		names: { 'skill:agility': 'Agility' }
	});
	const chips = chipsOf(st, {
		objectives: [{ key: 'skill:agility>=10', role: 'completes', verdict: 'met' }]
	});
	assert.deepEqual(
		chips.map((c) => [c.roleLabel, c.label, c.verdict]),
		[
			['conclui', '10 Agility', 'met'],
			['inicia', 'Cooks Assistant', 'unknown']
		]
	);
	assert.match(unknownHint(chips), /plugin 1\.1/);
	assert.equal(unknownHint(chips.slice(0, 1)), '');
});

test('status view maps marks and evidence to the step circle', () => {
	assert.equal(statusView('auto_done').st, 'st-auto');
	assert.equal(statusView('implied').label, 'presumido');
	assert.equal(statusView('implied').done, true);
	assert.equal(statusView('skipped').done, false);
	assert.equal(statusView('pending', true).st, 'st-cur');
});

test('paragraph keeps the TL;DR open, else the first bullet', () => {
	const tldr = paragraphOf(
		'- TL;DR: do Clock Tower and Monk’s Friend\n- Run to the clocktower\n- Talk to Kojo'
	);
	assert.equal(tldr.tldr, true);
	assert.equal(tldr.head, 'do Clock Tower and Monk’s Friend');
	assert.deepEqual(tldr.rest, ['Run to the clocktower', 'Talk to Kojo']);
	const plain = paragraphOf('- Choose a swag look\n- Enable 2FA\n- Finish tutorial island');
	assert.equal(plain.tldr, false);
	assert.equal(plain.head, 'Choose a swag look');
	assert.equal(plain.bullets, 2);
	assert.equal(isParagraphStep({ text: '- a\n- b' }), true);
	assert.equal(isParagraphStep({ text: 'Run to Draynor bank' }), false);
});

test('BRUHsailer notes become meta chips; tips stay notes', () => {
	const st = step('a', {
		notes: [
			{ kind: 'tip', text: 'GP after step: 25 gp' },
			{ kind: 'tip', text: 'Items needed: none' },
			{ kind: 'tip', text: 'Skills / quests: yes' },
			{ kind: 'tip', text: 'Total time: 35 minutes' },
			{ kind: 'tip', text: 'Bring food' }
		]
	});
	assert.deepEqual(metaOf(st), [
		{ label: 'GP após', value: '25 gp' },
		{ label: 'tempo acumulado', value: '35 minutes' }
	]);
	assert.deepEqual(plainNotes(st), ['Bring food']);
});

test('segments group the section steps and show the author range', () => {
	const section: ContentSection = {
		id: '1.1',
		slug: '1-1',
		title: 'x',
		guide: 'g',
		names: {},
		steps: [step('a', { ref: '1.1.1' }), step('b', { ref: '1.1.2' }), step('c', { ref: '1.1.3' })],
		segments: [
			{
				id: 'seg-1',
				kind: 'bank_leg',
				loadout: { status: 'partial', inventory: [{ name: 'Hammer', item_id: 2347 }] },
				steps: [{ id: 'a' }, { id: 'b' }]
			},
			{ id: 'seg-2', kind: 'sequence', steps: [{ id: 'c' }] }
		]
	};
	const segs = segmentsOf(section);
	assert.deepEqual(
		segs.map((s) => [s.id, s.range, s.steps.length]),
		[
			['seg-1', '1.1.1 – 1.1.2', 2],
			['seg-2', '1.1.3', 1]
		]
	);
	assert.equal(segmentsOf({ ...section, segments: undefined }).length, 1);
});

test('bank columns: with you, in the bank, the guide says', () => {
	const seg: SegmentReadiness = {
		guide_id: 'g',
		segment_id: 's',
		section_id: '1.1',
		kind: 'bank_leg',
		status: 'not_ready',
		loadout_status: 'partial',
		bank_actions: [{ type: 'open_bank', position: 'start', text: 'bank' }],
		freshness: {
			bank: {
				state: 'live',
				observed: true,
				complete: true,
				live: true,
				observed_at: '2026-11-04T12:00:00Z'
			}
		},
		items: [
			{
				name: 'Hammer',
				variant: 'exact',
				item_id: 2347,
				slot: 'inventory',
				required: 1,
				qty_stated: false,
				verdict: 'met',
				carried: 1,
				owned: { verdict: 'met', containers: { bank: { observed: true, qty: 1 } } }
			},
			{
				name: 'Saw',
				variant: 'exact',
				item_id: 8794,
				slot: 'inventory',
				required: 1,
				qty_stated: false,
				verdict: 'unmet',
				carried: 0,
				owned: { verdict: 'unmet', containers: { bank: { observed: true, qty: 0 } } }
			},
			{
				name: 'arrow shafts',
				variant: 'unresolved',
				slot: 'inventory',
				required: 1000,
				qty_stated: true,
				verdict: 'unknown',
				carried: 0,
				source_text: '1k arrow shafts'
			},
			{
				name: 'Planks',
				variant: 'exact',
				item_id: 960,
				slot: 'inventory',
				required: 120,
				qty_stated: true,
				verdict: 'met',
				carried: 0,
				owned: { verdict: 'met', stale: true, containers: { bank: { observed: true, qty: 120 } } }
			}
		]
	};
	const cols = bankColumns(seg);
	assert.equal(cols.carried, 1);
	assert.deepEqual(
		cols.slots.map((s) => [s.name, s.cls, s.suffix, s.qty]),
		[
			['Hammer', '', '', '1'],
			['Saw', 'slot-miss', 'falta', '0'],
			['arrow shafts', 'slot-unk', 'sem id', '1000'],
			['Planks', 'slot-stale', 'tinha', '120']
		]
	);
	assert.deepEqual([cols.inBank, cols.missing, cols.unverifiable], [2, 1, 1]);
	assert.deepEqual(cols.unresolved, ['1k arrow shafts']);
	const now = Date.parse('2026-11-04T12:00:30Z');
	assert.equal(freshnessLabel(seg, 'bank', now).label, 'banco · ao vivo · há 30 s');
	assert.equal(freshnessLabel(seg, 'inventory', now).cls, 'fr-un');
});

test('route labels and coverage lines', () => {
	assert.equal(sectionLabel('bruh-1.1'), 'cap. 1.1');
	assert.equal(sectionLabel('1.1'), '1.1');
	assert.equal(
		checkboxLine({ objectives: 10, met: 2, checkbox: 367, checkbox_done: 74, checkbox_skipped: 1 }),
		'367 passos sem objetivo detectável contam só por marca manual (74 feitos, 1 pulado).'
	);
});

test('session lines mix steps and objectives, newest first', () => {
	const lines = eventLines(
		{
			guide: 'g',
			account: 'a',
			steps: [
				{
					step_id: 'x',
					to: 'auto_done',
					from: 'pending',
					cause: 'engine',
					source: 'hiscores',
					at: '2026-11-04T20:14:00Z'
				},
				{
					step_id: 'y',
					to: 'implied',
					from: 'pending',
					cause: 'implied',
					at: '2026-11-04T20:14:00Z'
				},
				{
					step_id: 'z',
					to: 'manual_done',
					from: 'pending',
					cause: 'user',
					at: '2026-11-04T20:31:00Z'
				}
			],
			objectives: [
				{
					key: 'skill:firemaking>=50',
					to: 'met',
					from: 'unmet',
					source: 'hiscores',
					at: '2026-11-04T20:14:00Z'
				}
			]
		},
		(id) => `ref-${id}`,
		{ 'skill:firemaking': 'Firemaking' }
	);
	assert.equal(lines[0].text, 'ref-z marcado');
	assert.ok(lines.some((l) => l.text === 'ref-x detectado' && l.right.startsWith('HS')));
	assert.ok(lines.some((l) => l.text === '50 Firemaking'));
});

test('also-in lists where each other route completes the same entity', () => {
	const st = step('a', {
		objectives: [
			{ key: 'quest:monks_friend@finished', role: 'completes' },
			{ key: 'quest:clock_tower@finished', role: 'completes' },
			{ key: 'item:590x1', role: 'uses' }
		],
		names: { 'quest:monks_friend': "Monk's Friend", 'quest:clock_tower': 'Clock Tower' },
		also: {
			'quest:monks_friend@finished': [
				{
					guide: 'oziris-main',
					step: '1.1.40',
					ref: '1.1.40',
					section: '1.1',
					position: 40,
					total: 286,
					key: 'quest:monks_friend@finished',
					role: 'completes'
				}
			]
		}
	});
	const rows = alsoIn(st, chipsOf(st, undefined), [{ guide: 'oziris-main', label: 'Oziris' }]);
	assert.deepEqual(rows, [
		{
			guide: 'oziris-main',
			label: 'Oziris',
			entries: [
				{ label: "Monk's Friend", where: '1.1.40 (pos. 40 de 286)' },
				{ label: 'Clock Tower', where: 'não está na rota' }
			]
		}
	]);
	assert.equal(routeLabel('bruhsailer-main', 'x'), 'BRUHsailer');
});
