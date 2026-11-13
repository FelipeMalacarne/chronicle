import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
	compact,
	dayCard,
	fleetSummary,
	gained,
	gpToday,
	laneRhythm,
	nextLevel,
	nowStats,
	runningOut,
	sessionItems,
	sessionXp,
	signed,
	trainingBar,
	untilBreak,
	upToday,
	xpPerHour
} from './ritmo.ts';
import type { AccountRhythm } from './types.ts';

const rhythm = (over: Partial<AccountRhythm>): AccountRhythm => ({
	account: 'acc_a',
	training: null,
	session: null,
	today: { since: '2026-11-13T03:00:00Z', xp: 0, levels: 0, skills: [] },
	gp: null,
	...over
});

test('compact numbers read like the artboard', () => {
	assert.equal(compact(412), '412');
	assert.equal(compact(1300), '1,3k');
	assert.equal(compact(38_250), '38,2k');
	assert.equal(compact(412_000), '412k');
	assert.equal(compact(6_150_000), '6,1M');
	assert.equal(compact(-1100), '−1,1k');
	assert.equal(signed(84_000), '+84k');
	assert.equal(signed(-1100), '−1,1k');
	assert.equal(signed(0), '0');
});

test('the skill being trained', () => {
	const r = rhythm({
		training: {
			skill: 'fishing',
			level: 76,
			xp: 1_340_000,
			xp_per_hour: 38_200,
			next_level_in_s: 1320,
			last_gain_at: '2026-11-13T14:00:00Z'
		}
	});
	assert.equal(xpPerHour(r), '38,2k xp/h');
	assert.equal(nextLevel(r), '77 em 22 min');
	assert.equal(
		nextLevel(rhythm({ training: { ...r.training!, next_level_in_s: 6600 } })),
		'77 em 1 h 50'
	);
	// Without a rate or at 99 the line goes away.
	assert.equal(xpPerHour(rhythm({ training: { ...r.training!, xp_per_hour: 0 } })), null);
	assert.equal(
		nextLevel(rhythm({ training: { ...r.training!, next_level_in_s: undefined } })),
		null
	);
	assert.equal(xpPerHour(rhythm({})), null);
});

test('the session: xp, items gained and the one running out', () => {
	const r = rhythm({
		session: {
			started_at: '2026-11-13T13:00:00Z',
			xp: 26_700,
			gained: [
				{ item: 383, name: 'Raw shark', qty: 412 },
				{ item: 371, name: 'Raw swordfish', qty: 388 },
				{ item: 359, name: 'Raw tuna', qty: 301 },
				{ item: 1511, name: 'Logs', qty: 5 }
			],
			running_out: [{ item: 314, name: 'Feather', used: 400, left: 650, in_s: 11_100 }]
		}
	});
	assert.equal(sessionXp(r), '26,7k xp');
	assert.deepEqual(
		gained(r).map((g) => g.qty),
		[412, 388, 301]
	);
	assert.equal(runningOut(r)?.text, 'Feather acabam em 3 h 05');
	// Unknown (null) and nothing ([]) both show nothing.
	const unknown = rhythm({ session: { ...r.session!, gained: null, running_out: null } });
	assert.deepEqual(gained(unknown), []);
	assert.equal(runningOut(unknown), null);
	assert.equal(sessionXp(rhythm({})), null);
});

test('GP today only when the start of the day was seen', () => {
	assert.equal(gpToday(rhythm({ gp: { value: 1_900_000, today: 22_000 } })), '+22k');
	assert.equal(gpToday(rhythm({ gp: { value: 1_900_000 } })), null);
	assert.equal(gpToday(rhythm({})), null);
});

test('until the break', () => {
	assert.deepEqual(untilBreak({ active: false, in_s: 1860 }), {
		label: 'Até a pausa',
		text: '31 min'
	});
	assert.deepEqual(untilBreak({ active: true, left_s: 360 }), {
		label: 'Em pausa',
		text: 'volta em 6 min'
	});
	// The client does not say: nothing.
	assert.equal(untilBreak({ active: false }), null);
	assert.equal(untilBreak({ active: true }), null);
	assert.equal(untilBreak(undefined), null);
});

test('the fleet summary', () => {
	const s = fleetSummary({
		since: '2026-11-13T03:00:00Z',
		accounts: [],
		totals: { xp_today: 412_000, levels_today: 7, gaining: 3, gp: 6_100_000, gp_today: 84_000 }
	});
	assert.deepEqual(s, { xp: '412k', xpSub: '7 níveis · 3 contas', gp: '6,1M', gpSub: '+84k hoje' });
	const one = fleetSummary({
		since: '2026-11-13T03:00:00Z',
		accounts: [],
		totals: { xp_today: 900, levels_today: 1, gaining: 1 }
	});
	assert.deepEqual(one, { xp: '900', xpSub: '1 nível · 1 conta', gp: null, gpSub: null });
	assert.equal(
		fleetSummary({
			since: '2026-11-13T03:00:00Z',
			accounts: [],
			totals: { xp_today: 5_000, levels_today: 0, gaining: 2 }
		}).xpSub,
		'nenhum nível · 2 contas'
	);
	// Nobody gained XP since midnight: a measured zero, said as such.
	const none = fleetSummary({
		since: '2026-11-13T03:00:00Z',
		accounts: [],
		totals: { xp_today: 0, levels_today: 0, gaining: 0, gp: 25_000, gp_today: 0 }
	});
	assert.deepEqual(none, { xp: '0', xpSub: 'nada ainda hoje', gp: '25k', gpSub: '0 hoje' });
});

const fishing = rhythm({
	training: {
		skill: 'fishing',
		level: 76,
		// 1 336 443 is level 76; 1 475 581 is 77: 64% of the way.
		xp: 1_425_500,
		xp_per_hour: 38_200,
		next_level_in_s: 1320,
		last_gain_at: '2026-11-13T14:00:00Z'
	},
	session: {
		started_at: '2026-11-13T13:00:00Z',
		xp: 26_700,
		gained: [
			{ item: 383, name: 'Raw shark', qty: 412 },
			{ item: 371, name: 'Raw swordfish', qty: 388 },
			{ item: 359, name: 'Raw tuna', qty: 301 },
			{ item: 1511, qty: 5 }
		],
		running_out: [{ item: 314, name: 'Feather', used: 1100, left: 2900, in_s: 10_800 }]
	},
	today: {
		since: '2026-11-13T03:00:00Z',
		xp: 114_300,
		levels: 4,
		skills: [
			{ skill: 'fishing', xp: 96_400, levels: 2, level: 76 },
			{ skill: 'farming', xp: 11_200, levels: 1, level: 39 },
			{ skill: 'hunter', xp: 5_600, levels: 1, level: 34 },
			{ skill: 'agility', xp: 1100, levels: 0, level: 52 }
		]
	},
	gp: { value: 1_900_000, today: 22_000 }
});

test('the training bar: the skill, the way to the next level and when it comes', () => {
	assert.deepEqual(trainingBar(fishing), {
		skill: 'fishing',
		name: 'Fishing',
		level: 76,
		pct: 64,
		next: '77 em 22 min'
	});
	// At 99 the bar is full and there is no next level.
	const maxed = rhythm({
		training: { ...fishing.training!, level: 99, xp: 13_100_000, next_level_in_s: undefined }
	});
	assert.deepEqual(trainingBar(maxed), {
		skill: 'fishing',
		name: 'Fishing',
		level: 99,
		pct: 100,
		next: null
	});
	assert.equal(trainingBar(rhythm({})), null);
	assert.equal(trainingBar(null), null);
});

test('the lane: rate, the items gained and the one running out, only while playing', () => {
	const l = laneRhythm(fishing, true)!;
	assert.equal(l.rate, '38,2k xp/h');
	assert.equal(l.bar?.next, '77 em 22 min');
	assert.deepEqual(
		l.gained.map((g) => [g.item, g.qty]),
		[
			[383, '412'],
			[371, '388'],
			[359, '301']
		]
	);
	assert.equal(l.gained[0].title, 'Raw shark: +412 nesta sessão');
	assert.equal(l.out?.qty, '−1,1k');
	assert.equal(
		l.out?.title,
		'Feather: 1.100 gastos nesta sessão, sobram 2.900; acabam em 3 h 00 neste ritmo'
	);
	// A stopped account: its session is the last one, not now's.
	assert.equal(laneRhythm(rhythm({ session: fishing.session }), false), null);
	// Still gaining XP (a hand-played client the plugin is late on): the rhythm shows.
	assert.equal(laneRhythm(fishing, false)?.rate, '38,2k xp/h');
	// Playing with nothing measured: no line at all.
	assert.equal(laneRhythm(rhythm({}), true), null);
	assert.equal(laneRhythm(undefined, true), null);
	// Unknown items (null) show nothing, not an empty "gained".
	const unknown = laneRhythm(
		rhythm({
			training: fishing.training,
			session: { ...fishing.session!, gained: null, running_out: null }
		}),
		true
	)!;
	assert.deepEqual(unknown.gained, []);
	assert.equal(unknown.out, null);
});

test('doing now: the stats that have data, and the items of the session', () => {
	assert.deepEqual(nowStats(fishing, { active: false, in_s: 1860 }), [
		{ k: 'XP por hora', v: '38,2k' },
		{ k: 'Nesta sessão', v: '26,7k xp' },
		{ k: 'Até a pausa', v: '31 min' }
	]);
	// In a break; and without the client, no pause cell.
	assert.deepEqual(nowStats(fishing, { active: true, left_s: 360 })[2], {
		k: 'Em pausa',
		v: 'volta em 6 min'
	});
	assert.deepEqual(
		nowStats(fishing, null).map((s) => s.k),
		['XP por hora', 'Nesta sessão']
	);
	assert.deepEqual(nowStats(rhythm({}), undefined), []);
	const items = sessionItems(fishing);
	assert.deepEqual(
		items.gained.map((g) => `${g.qty} ${g.name}`),
		['412 Raw shark', '388 Raw swordfish', '301 Raw tuna']
	);
	assert.equal(items.out?.text, 'Feather acabam em 3 h 00');
	assert.deepEqual(sessionItems(null), { gained: [], out: null });
});

test('today: the skills that gained XP, the levels and the GP', () => {
	const d = dayCard(fishing);
	assert.deepEqual(d.rows, [
		{ skill: 'fishing', name: 'Fishing', up: true, levels: '74 → 76', xp: '+96,4k' },
		{ skill: 'farming', name: 'Farming', up: true, levels: '38 → 39', xp: '+11,2k' },
		{ skill: 'hunter', name: 'Hunter', up: true, levels: '33 → 34', xp: '+5,6k' },
		{ skill: 'agility', name: 'Agility', up: false, levels: '52', xp: '+1,1k' }
	]);
	assert.equal(d.more, null);
	assert.deepEqual(d.gp, { value: '1,9M', today: '+22k' });
	// More skills than fit: the rest folds into one line.
	const two = dayCard(fishing, 2);
	assert.equal(two.rows.length, 2);
	assert.deepEqual(two.more, { n: 2, xp: '+6,7k' });
	// The bank never seen: no GP line; a day not seen at its start: GP without today's change.
	assert.equal(dayCard(rhythm({})).gp, null);
	assert.deepEqual(dayCard(rhythm({ gp: { value: 25_000 } })).gp, { value: '25k', today: null });
	assert.deepEqual(dayCard(rhythm({})).rows, []);
});

test('what went up today, for the character sheet', () => {
	assert.deepEqual(upToday(fishing), {
		levels: { fishing: 2, farming: 1, hunter: 1 },
		text: 'Fishing +2, Farming +1, Hunter +1'
	});
	assert.deepEqual(upToday(rhythm({})), { levels: {}, text: null });
	assert.deepEqual(upToday(null), { levels: {}, text: null });
});
