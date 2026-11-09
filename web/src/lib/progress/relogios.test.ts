import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clockRows, duration } from './relogios.ts';

test('duration reads like the cockpit', () => {
	assert.equal(duration(0), '<1 min');
	assert.equal(duration(61), '2 min');
	assert.equal(duration(23 * 60), '23 min');
	assert.equal(duration(2 * 3600 + 5 * 60), '2 h 05');
	assert.equal(duration(27 * 3600), '1 d 3 h');
});

test('clockRows: ready first, then by time; unseen apart', () => {
	const now = 1_000_000;
	const { rows, unseen } = clockRows(
		[
			{ key: 'TREE', label: 'Trees', state: 'IN_PROGRESS', readyAt: now + 5 * 3600 },
			{ key: 'HOPS', label: 'Hops', state: 'UNKNOWN' },
			{ key: 'HERB', label: 'Herbs', state: 'IN_PROGRESS', readyAt: now + 23 * 60 },
			{ key: 'ALLOTMENT', label: 'Allotments', state: 'EMPTY' },
			{ key: 'BIRD_HOUSE', label: 'Bird Houses', state: 'COMPLETED', readyAt: now - 600 },
			// Past its time but not flipped yet: ready.
			{ key: 'FLOWER', label: 'Flowers', state: 'IN_PROGRESS', readyAt: now - 1 }
		],
		now
	);
	assert.deepEqual(
		rows.map((r) => [r.key, r.tone, r.when]),
		[
			['BIRD_HOUSE', 'ready', 'pronto'],
			['FLOWER', 'ready', 'pronto'],
			['HERB', 'growing', 'em 23 min'],
			['TREE', 'growing', 'em 5 h 00'],
			['ALLOTMENT', 'idle', 'vazio']
		]
	);
	assert.deepEqual(unseen, ['Hops']);
});
