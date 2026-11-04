import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildBands, sharedMilestones, visibleWindow, flattenSteps } from './compare.ts';
import { createFixtureAdapter } from './adapter.ts';
import { describeBank } from './bank.ts';
import { pickGuide, pickPair, planQuery } from './navigation.ts';
import type { StepVM } from './types.ts';

const s = (id: string, milestone?: string): StepVM => ({
	key: `g::${id}`,
	guideId: 'g',
	localId: id,
	text: id,
	milestone
});

test('sharedMilestones keeps common order and drops crossings', () => {
	const l = [s('a', 'A'), s('b', 'B'), s('c', 'C')];
	const r = [s('x', 'C'), s('y', 'A'), s('z', 'B')];
	assert.deepEqual(sharedMilestones(l, r), ['A', 'B']);
});

test('buildBands connects only shared milestones and keeps lanes independent', () => {
	const l = [s('l0'), s('l1', 'A'), s('l2'), s('l3', 'only-left'), s('l4', 'B')];
	const r = [s('r0'), s('r1'), s('r2', 'A'), s('r3', 'B'), s('r4')];
	const bands = buildBands(l, r);
	assert.deepEqual(
		bands.map((b) => b.milestone),
		[null, 'A', 'B']
	);
	assert.equal(bands[0].left.length, 1);
	assert.equal(bands[0].right.length, 2);
	assert.equal(bands[1].left.length, 2); // l2 + l3 (marco não compartilhado vira passo comum)
	assert.equal(bands[1].right.length, 0);
	assert.equal(bands[2].right.length, 1);
	const total = bands.reduce((n, b) => n + b.left.length + b.right.length, 0);
	// 4 âncoras (A e B em cada faixa); o resto cai em alguma banda.
	assert.equal(total, l.length + r.length - 4);
});

test('both guides compare without step-key collisions', async () => {
	const a = createFixtureAdapter();
	const [oz, bs] = await Promise.all([a.loadPlan('oziris-main'), a.loadPlan('bruhsailer-main')]);
	const ozSteps = flattenSteps(oz.sections);
	const bsSteps = flattenSteps(bs.sections);
	const ozLocal = new Set(ozSteps.map((x) => x.localId));
	assert.ok(
		bsSteps.some((x) => ozLocal.has(x.localId)),
		'fixtures must reuse local ids'
	);
	const keys = [...ozSteps, ...bsSteps].map((x) => x.key);
	assert.equal(new Set(keys).size, keys.length);
	const bands = buildBands(ozSteps, bsSteps);
	assert.ok(bands.filter((b) => b.milestone).length >= 3);
});

test('restored bank is not trustworthy and says so', () => {
	const v = describeBank({ freshness: 'restored', capturedAt: 0 }, 3 * 3_600_000);
	assert.equal(v.trustworthy, false);
	assert.equal(v.tone, 'warning');
	assert.match(v.label, /restaurado · há 3 h/);
	assert.match(v.detail, /desatualizado/);
	assert.equal(describeBank({ freshness: 'live', capturedAt: 0 }, 1000).trustworthy, true);
	assert.equal(describeBank({ freshness: 'unavailable', capturedAt: null }, 0).tone, 'error');
});

test('unavailable bank hides quantities in the next leg', async () => {
	const a = createFixtureAdapter({ bank: { 'oziris-main': 'unavailable' } });
	const leg = await a.nextBankLeg('oziris-main');
	assert.ok(leg);
	assert.ok(leg.withdraw.every((i) => i.inBank === null));
	assert.equal(leg.bank.capturedAt, null);
});

test('scenarios: error rejects, empty has no sections', async () => {
	await assert.rejects(createFixtureAdapter({ scenario: 'error' }).loadPlan('oziris-main'));
	const p = await createFixtureAdapter({ scenario: 'empty' }).loadPlan('oziris-main');
	assert.equal(p.sections.length, 0);
});

test('visibleWindow clamps and pads', () => {
	const w = visibleWindow({ count: 100, rowHeight: 10, scrollTop: 200, viewport: 50, overscan: 2 });
	assert.deepEqual(w, { start: 18, end: 27, padTop: 180, padBottom: 730 });
	assert.deepEqual(visibleWindow({ count: 5, rowHeight: 10, scrollTop: 0, viewport: 500 }), {
		start: 0,
		end: 5,
		padTop: 0,
		padBottom: 0
	});
});

test('navigation helpers are guide-aware', () => {
	const known = ['oziris-main', 'bruhsailer-main'];
	assert.equal(pickGuide('nope', known), 'oziris-main');
	assert.equal(pickGuide('bruhsailer-main', known), 'bruhsailer-main');
	assert.deepEqual(pickPair('oziris-main', 'oziris-main', known), [
		'oziris-main',
		'bruhsailer-main'
	]);
	assert.equal(planQuery({ guide: 'x', section: null }), '?guide=x');
	assert.equal(planQuery({}), '');
});
