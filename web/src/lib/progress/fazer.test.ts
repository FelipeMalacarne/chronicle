import { test } from 'node:test';
import assert from 'node:assert/strict';
import { firingText, runFinal, runnable, stoppable } from './fazer.ts';
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
