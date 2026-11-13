import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rowChip, rowChips, sessionLine, whoDid } from './agora.ts';
import type { EventsView } from './types.ts';
import type { Chip } from './view.ts';

const now = Date.parse('2026-11-05T20:00:00Z');
const at = (min: number) => new Date(now - min * 60_000).toISOString();
const ev: EventsView = {
	guide: 'oziris-main',
	account: 'acc',
	steps: [
		{ step_id: '1.1.217', to: 'auto_done', cause: 'engine', source: 'plugin', at: at(12) },
		{ step_id: '1.1.218', to: 'manual_done', cause: 'user', at: at(9) },
		{ step_id: '1.1.216', to: 'implied', cause: 'implied', at: at(9) },
		{ step_id: '1.1.100', to: 'auto_done', cause: 'engine', source: 'hiscores', at: at(60 * 30) },
		{ step_id: '1.1.218', from: 'manual_done', to: 'pending', cause: 'user', at: at(20) }
	],
	objectives: [
		{ key: 'skill:magic>=43', to: 'met', source: 'hiscores', at: at(5) },
		{ key: 'quest:the_grand_tree@finished', to: 'met', source: 'plugin', at: at(5) },
		{ key: 'skill:agility>=10', to: 'met', source: 'hiscores', at: at(60 * 30) }
	]
};

test('whoDid names the source, the owner or the cascade, with the age', () => {
	assert.equal(whoDid(ev, '1.1.217', now), 'plugin · 12 min');
	assert.equal(whoDid(ev, '1.1.218', now), 'você · 9 min');
	assert.equal(whoDid(ev, '1.1.216', now), 'presumido · 9 min');
	assert.equal(whoDid(ev, '1.1.999', now), '');
	assert.equal(whoDid(null, '1.1.217', now), '');
});

test('sessionLine counts closed steps and levels of the last hours only', () => {
	assert.equal(sessionLine(ev, now), '+3 passos, +1 level');
	assert.equal(sessionLine({ ...ev, steps: [], objectives: [] }, now), '');
});

test('rowChip picks completes over starts over uses and counts the rest', () => {
	const chip = (role: Chip['role'], key: string) => ({ role, key }) as Chip;
	const r = rowChip([
		chip('uses', 'item:1'),
		chip('starts', 'quest:b'),
		chip('completes', 'quest:a')
	]);
	assert.equal(r.chip?.key, 'quest:a');
	assert.equal(r.more, 2);
	assert.deepEqual(rowChip([]), { chip: null, more: 0 });
});

test('rowChips keeps the first n by role and counts the rest', () => {
	const chip = (role: Chip['role'], key: string) => ({ role, key }) as Chip;
	const all = [chip('uses', 'item:1'), chip('starts', 'quest:b'), chip('completes', 'quest:a')];
	const r = rowChips(all, 2);
	assert.deepEqual(
		r.shown.map((c) => c.key),
		['quest:a', 'quest:b']
	);
	assert.equal(r.more, 1);
	assert.deepEqual(rowChips(all, 5).more, 0);
});
