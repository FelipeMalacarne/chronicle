import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	firingText,
	runDuration,
	runFinal,
	runOutcome,
	runWhen,
	runnable,
	stoppable
} from './fazer.ts';
import type { Run } from '../api/microbot.ts';
import type { PluginRow } from './plugins.ts';

const row = (className: string, over: Partial<PluginRow> = {}): PluginRow => ({
	className,
	name: className.split('.').pop() ?? className,
	source: 'meus',
	active: true,
	protected: false,
	tone: 'ok',
	...over
});

const rows = [
	row('a.fm.tickfisher.TickFisherPlugin'),
	row('a.kittentracker.KittenPlugin', { source: 'hub' }),
	row('a.fm.catswap.CatSwapPlugin', { active: false }),
	row('a.QoLPlugin', { source: 'cliente' }),
	row('a.fm.chroniclelink.ChronicleLinkPlugin', { protected: true })
];

test('runnable: ours and the Hub, Cat Swap first', () => {
	assert.deepEqual(
		runnable(rows).map((r) => r.name),
		['CatSwapPlugin', 'TickFisherPlugin', 'KittenPlugin']
	);
});

test('stoppable: active ones, the kitten left running', () => {
	assert.deepEqual(
		stoppable(rows, 'a.fm.catswap.CatSwapPlugin').map((s) => [s.row.name, s.checked]),
		[
			['TickFisherPlugin', true],
			['KittenPlugin', false]
		]
	);
	// The run plugin itself is never in the list.
	assert.equal(
		stoppable(rows, 'a.fm.tickfisher.TickFisherPlugin').some(
			(s) => s.row.name === 'TickFisherPlugin'
		),
		false
	);
});

test('runFinal', () => {
	assert.equal(runFinal(null), false);
	assert.equal(runFinal({ state: 'running' } as never), false);
	assert.equal(runFinal({ state: 'canceled' } as never), true);
});

test('firingText: what the trigger did with the last event', () => {
	const at = '2026-11-10T22:00:00Z';
	assert.equal(firingText(undefined), null);
	assert.equal(
		firingText({ at, event: 'kitten_grown', runId: 'run-3' }),
		'o filhote cresceu: disparou'
	);
	assert.equal(
		firingText({ at, event: 'kitten_grown', skipped: 'trigger off' }),
		'o filhote cresceu com o gatilho desligado'
	);
	assert.equal(
		firingText({ at, event: 'kitten_grown', skipped: 'run: another run is going' }),
		'o filhote cresceu durante outro run'
	);
});

const aRun = (over: Partial<Run> = {}): Run => ({
	id: 'run-1',
	className: 'a.fm.catswap.CatSwapPlugin',
	stop: [],
	state: 'done',
	startedAt: '2026-11-10T14:00:00Z',
	events: [],
	...over
});

test('runDuration: minutes, hours, unfinished', () => {
	assert.equal(runDuration(aRun()), null);
	assert.equal(runDuration(aRun({ finishedAt: '2026-11-10T14:00:20Z' })), 'menos de 1 min');
	assert.equal(runDuration(aRun({ finishedAt: '2026-11-10T14:04:10Z' })), '4 min');
	assert.equal(runDuration(aRun({ finishedAt: '2026-11-10T15:05:00Z' })), '1 h 05 min');
});

test('runOutcome: what Cat Swap left', () => {
	assert.equal(runOutcome(aRun()), '');
	assert.equal(
		runOutcome(aRun({ result: { outcome: 'done', deathRunesGained: 100, kittenFollows: true } })),
		'+100 death runes · filhote seguindo'
	);
});

test('runWhen: today, yesterday, older', () => {
	const now = new Date(2026, 10, 10, 18, 0);
	assert.match(runWhen(new Date(2026, 10, 10, 14, 2).toISOString(), now), /^hoje 14:02$/);
	assert.match(runWhen(new Date(2026, 10, 9, 23, 10).toISOString(), now), /^ontem 23:10$/);
	assert.match(runWhen(new Date(2026, 10, 8, 9, 15).toISOString(), now), /^08\/11 09:15$/);
});
