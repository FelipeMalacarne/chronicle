import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	aheadView,
	alsoIn,
	crossCell,
	crossObjectives,
	distanceTo,
	firstReaching,
	blockedBadge,
	bankColumns,
	checkboxLine,
	chipsOf,
	eventLines,
	entityVerdict,
	freshnessLabel,
	iconRef,
	mainKey,
	isParagraphStep,
	metaOf,
	objectiveLabel,
	paragraphOf,
	pendingBefore,
	parseKey,
	plainNotes,
	routeLabel,
	routesLine,
	sectionLabel,
	segmentsOf,
	sourceBand,
	stateLabel,
	statusView,
	unknownHint,
	verdictView
} from './view.ts';
import type { Ahead, AheadItem, ContentSection, ContentStep, SegmentReadiness } from './types.ts';

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

test('parseKey reads item families and alternatives', () => {
	assert.deepEqual(parseKey('item:family/graceful_hoodx1'), {
		ref: 'item:family/graceful_hood',
		kind: 'item',
		id: 'family/graceful_hood',
		op: 'x',
		n: 1,
		family: 'graceful_hood'
	});
	assert.deepEqual(parseKey('item:any(1355x2|1357x1|1359x1)'), {
		ref: 'item:any(1355x2|1357x1|1359x1)',
		kind: 'item',
		id: 'any(1355x2|1357x1|1359x1)',
		op: 'x',
		alternatives: [
			{ id: 1355, n: 2 },
			{ id: 1357, n: 1 },
			{ id: 1359, n: 1 }
		]
	});
	assert.equal(parseKey('item:any(1355x2)'), null);
	assert.equal(parseKey('item:any(1355x2|1357)'), null);
	assert.equal(parseKey('item:any(1355x2|1357x1'), null);
});

test('item families and alternatives have their own labels and icons', () => {
	const names = {
		'item:family/graceful_hood': 'Graceful hood',
		'item:1355': 'Mithril axe',
		'item:1357': 'Adamant axe',
		'item:1359': 'Rune axe'
	};
	assert.equal(
		objectiveLabel('item:family/graceful_hoodx1', names),
		'Graceful hood (qualquer variante)'
	);
	assert.equal(
		objectiveLabel('item:family/prayer_potionx4', {}),
		'4 Prayer potion (qualquer variante)'
	);
	assert.equal(
		objectiveLabel('item:any(1355x2|1357x1|1359x1)', names),
		'2 Mithril axe ou Adamant axe ou Rune axe'
	);
	assert.equal(objectiveLabel('item:any(1355x1|9999x3)', names), 'Mithril axe ou 3 item 9999');
	assert.equal(stateLabel('item:family/graceful_hoodx1'), '×1');
	assert.equal(stateLabel('item:any(1355x2|1357x1)'), '×2 ou ×1');
	assert.equal(iconRef('item:any(1355x2|1357x1)'), 'item:1355');
	assert.equal(iconRef('item:family/graceful_hood'), undefined);
	assert.equal(iconRef('item:314'), 'item:314');
	assert.equal(iconRef('skill:agility'), 'skill:agility');
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
	assert.equal(statusView('pending', false, true).st, 'st-lock');
	assert.equal(statusView('pending', true, true).st, 'st-cur');
	assert.equal(statusView('pinned_open', false, true).st, '');
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

test('world helpers: state labels, main key, entity verdict', () => {
	assert.equal(stateLabel('quest:a@started'), 'iniciada');
	assert.equal(stateLabel('quest:a@finished'), 'concluída');
	assert.equal(stateLabel('skill:agility>=70'), '≥ 70');
	assert.equal(stateLabel('item:314x120'), '×120');
	assert.equal(mainKey(['quest:a@started', 'quest:a@finished']), 'quest:a@finished');
	assert.equal(
		mainKey(['skill:agility>=10', 'skill:agility>=70', 'skill:agility>=35']),
		'skill:agility>=70'
	);
	assert.equal(entityVerdict([{ verdict: 'met' }, { verdict: 'unknown' }]), 'prog');
	assert.equal(entityVerdict([{ verdict: 'unknown' }]), 'unknown');
	assert.equal(entityVerdict([{ verdict: 'unmet' }, { verdict: 'unknown' }]), 'unmet');
	assert.equal(
		routesLine(
			[
				{ guide: 'oziris-main', ref: '1.5.17' },
				{ guide: 'bruhsailer-main', ref: 'CH2.72' }
			],
			(g) => routeLabel(g, g)
		),
		'Oziris 1.5.17 · BRUHsailer CH2.72'
	);
});

test('source band: failed beats stale, rename alert passes through', () => {
	const now = Date.parse('2026-11-04T20:30:00');
	const run = (extra: object) => ({
		id: 'r',
		source: 'hiscores',
		trigger: 'scheduled' as const,
		steps_changed: 0,
		created_at: '2026-11-04T20:02:00',
		...extra
	});
	assert.equal(sourceBand(run({ status: 'ok' }) as never, now - 3600_000, now), null);
	const failed = sourceBand(
		run({
			status: 'rate_limited',
			http_status: 503,
			next_allowed_at: '2026-11-04T20:12:00'
		}) as never,
		Date.parse('2026-11-04T19:47:00'),
		now
	);
	assert.equal(failed?.kind, 'failed');
	assert.match(failed?.text ?? '', /desde 20:02 \(HTTP 503\).*19:47.*20:12/);
	const renamed = sourceBand(
		run({
			status: 'error',
			http_status: 404,
			error: 'conta não encontrada no hiscores 3 vezes seguidas; renomeou ou ainda não ranqueia?'
		}) as never,
		undefined,
		now
	);
	assert.match(renamed?.text ?? '', /renomeou/);
	assert.equal(sourceBand(undefined, now - 3 * 86400_000, now)?.kind, 'stale');
});

test('pending-before counts what the batch would mark', () => {
	const s = (id: string, status: 'pending' | 'auto_done' | 'skipped' | 'implied', extra = {}) => ({
		id,
		status,
		main: true,
		visible: true,
		...extra
	});
	const steps = [
		s('a', 'pending'),
		s('b', 'skipped', { mark: { kind: 'skipped' as const } }),
		s('c', 'auto_done'),
		s('d', 'pending', { visible: false }),
		s('t', 'pending', { main: false }),
		s('e', 'pending'),
		s('i', 'implied', { mark: { kind: 'implied' as const, implied_by_step_id: 'x' } }),
		s('x', 'pending')
	];
	assert.deepEqual(pendingBefore(steps, 'x'), ['a', 'e', 'i']);
	assert.deepEqual(pendingBefore(steps, 'a'), []);
	assert.deepEqual(pendingBefore(steps, 'missing'), []);
});

test('compare: cross objectives, cells, distance and reaching', () => {
	const st = (
		id: string,
		status: 'pending' | 'auto_done' | 'manual_done' | 'skipped',
		objectives: { key: string; verdict: 'met' | 'unmet' | 'unknown' }[],
		extra = {}
	) => ({
		id,
		ref: id.toUpperCase(),
		title: id,
		section_id: id.slice(0, 1),
		status,
		main: true,
		visible: true,
		objectives: objectives.map((o) => ({ ...o, role: 'completes' as const })),
		...extra
	});
	const oz = {
		guide: 'oz',
		steps: [
			st('a1', 'auto_done', [{ key: 'skill:woodcutting>=35', verdict: 'met' }]),
			st('a2', 'manual_done', [{ key: 'quest:monks_friend@finished', verdict: 'unknown' }]),
			st('a3', 'skipped', []),
			st('b1', 'pending', [{ key: 'skill:cooking>=40', verdict: 'unmet' }]),
			st('b2', 'pending', [{ key: 'quest:only_here@finished', verdict: 'unknown' }]),
			st('b3', 'pending', [{ key: 'skill:cooking>=70', verdict: 'unmet' }], { visible: false }),
			st('c1', 'pending', [{ key: 'skill:cooking>=70', verdict: 'unmet' }])
		]
	};
	const br = {
		guide: 'br',
		steps: [
			st('x1', 'pending', [
				{ key: 'quest:monks_friend@finished', verdict: 'unknown' },
				{ key: 'skill:woodcutting>=35', verdict: 'met' }
			]),
			st('x2', 'pending', [{ key: 'skill:cooking>=40', verdict: 'unmet' }])
		]
	};
	const rows = crossObjectives([oz, br]);
	assert.deepEqual(
		rows.map((r) => r.key),
		['skill:woodcutting>=35', 'quest:monks_friend@finished', 'skill:cooking>=40']
	);
	assert.equal(crossCell(rows[0], 'oz'), 'feito');
	assert.equal(crossCell(rows[0], 'br'), 'adianta');
	assert.equal(crossCell(rows[1], 'oz'), 'feito à mão');
	assert.equal(crossCell(rows[1], 'br'), 'não sabemos');
	assert.equal(crossCell(rows[2], 'br'), 'pendente');
	assert.equal(crossCell(rows[2], 'zz'), null);
	assert.deepEqual(distanceTo(oz.steps, 'c1'), { steps: 3, sections: ['b', 'c'] });
	assert.equal(distanceTo(oz.steps, 'nope'), null);
	assert.equal(firstReaching(oz.steps, 'skill:cooking>=50')?.id, 'c1');
	assert.equal(firstReaching(oz.steps, 'skill:cooking>=30')?.id, 'b1');
	assert.equal(firstReaching(br.steps, 'skill:cooking>=50'), null);
});

test('preview of the next steps when the guide lists no setup', () => {
	const it = (name: string, extra: Partial<AheadItem>): AheadItem => ({
		name,
		variant: 'exact',
		item_id: 1,
		slot: 'inventory',
		required: 1,
		qty_stated: false,
		verdict: 'unmet',
		carried: 0,
		...extra
	});
	const bank = { bank: { state: 'live' as const, observed: true, qty: 6 } };
	const a: Ahead = {
		from_step_id: '1.1.219',
		until_step_id: '1.1.282',
		steps: 63,
		groups: [
			{
				step_id: '1.1.223',
				text: 'Start biohazard',
				offset: 4,
				items: [
					it('GP', { verdict: 'met', carried: 8200, also_at: ['1.1.236'] }),
					it('Clay', {
						required: 6,
						qty_stated: true,
						owned: { verdict: 'met', held: 6, containers: bank }
					}),
					it('Iron ore', {
						required: 2,
						qty_stated: true,
						owned: { verdict: 'unmet', held: 1, containers: bank }
					}),
					it('Ghost skull', { owned: { verdict: 'unmet', held: 0, containers: {} } }),
					it('Planks', { owned: { verdict: 'met', stale: true, held: 3, containers: bank } }),
					it('cat', { variant: 'unresolved', item_id: undefined, verdict: 'unknown' })
				]
			},
			{
				step_id: '1.1.271',
				text: 'Teleport to Camelot',
				offset: 52,
				items: [it('Hammer', { verdict: 'unknown' })]
			}
		],
		obtained: [
			{
				name: 'Fishing rod',
				item_id: 307,
				qty: 2,
				step_id: '1.1.230',
				use_step_id: '1.1.236',
				use_offset: 17
			},
			{
				name: 'Excalibur',
				item_id: 35,
				qty: 1,
				step_id: '1.1.248',
				use_step_id: '1.1.271',
				use_offset: 52
			}
		]
	};
	const v = aheadView(a, 30);
	assert.equal(v.until, '30 passos à frente');
	assert.deepEqual(
		v.groups.map((g) => [g.stepId, g.when]),
		[['1.1.223', 'em 4 passos']]
	);
	assert.deepEqual(
		v.groups[0].slots.map((s) => [s.name, s.cls, s.state, s.qty]),
		[
			['GP', 'slot-you', 'com você', ''],
			['Clay', '', 'no banco', '6'],
			['Iron ore', 'slot-miss', 'tem 1 de 2', '2'],
			['Ghost skull', 'slot-miss', 'falta', ''],
			['Planks', 'slot-stale', 'tinha', ''],
			['cat', 'slot-unk', 'sem id', '']
		]
	);
	assert.equal(v.groups[0].slots[0].tip, 'também em 1.1.236');
	assert.deepEqual(v.counts, { total: 6, you: 1, have: 2, miss: 2, unk: 1 });
	assert.deepEqual(
		v.obtained.map((o) => o.name),
		['Fishing rod ×2']
	);
	const all = aheadView(a, 0);
	assert.equal(all.until, '1.1.282, fim do trecho');
	assert.equal(all.groups.length, 2);
	assert.equal(all.groups[1].slots[0].state, 'não sabemos');
	assert.equal(all.obtained.length, 2);
	// A horizon past the end of the leg is the whole leg.
	assert.equal(aheadView(a, 100).until, '1.1.282, fim do trecho');
});

test('blocked badge names the first missing requirement', () => {
	const names = { 'quest:twilights_promise': "Twilight's Promise" };
	assert.equal(blockedBadge(undefined, names), '');
	assert.equal(blockedBadge(['skill:slayer>=48'], names), 'falta 48 Slayer');
	assert.equal(
		blockedBadge(['quest:twilights_promise@finished', 'skill:slayer>=48'], names),
		"falta Twilight's Promise +1"
	);
});
