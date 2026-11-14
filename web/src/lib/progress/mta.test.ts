import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
	carriedRunes,
	magicLevel,
	ownedRewards,
	parsePoints,
	runeRows,
	type MtaTable
} from './mta.ts';
import type { AccountState } from './types.ts';

const table = {
	source: '',
	rooms: [],
	enchant_levels: [],
	rewards: [
		{ id: 'beginner_wand', name: 'Beginner wand', item_id: 6908, cost: z() },
		{ id: 'rune_pouch', name: 'Rune pouch', item_id: 12791, cost: z() }
	]
} as unknown as MtaTable;
function z() {
	return { telekinetic: 0, graveyard: 0, enchantment: 0, alchemist: 0 };
}
const state = (containers: unknown[], skills: unknown[] = []) =>
	({ containers, skills }) as unknown as AccountState;
const cont = (container: string, items: { id: number; qty: number }[], s = 'live') => ({
	container,
	state: s,
	items
});

test('magicLevel reads the skill, undefined without it', () => {
	assert.equal(magicLevel(state([], [{ skill: 'magic', level: 87 }])), 87);
	assert.equal(magicLevel(state([])), undefined);
});

test('carriedRunes sums every container and is null when none was read', () => {
	assert.equal(carriedRunes(state([])), null);
	assert.equal(carriedRunes(state([cont('bank', [], 'unavailable')])), null);
	const st = state([
		cont('inventory', [{ id: 563, qty: 10 }]),
		cont('bank', [
			{ id: 563, qty: 5 },
			{ id: 561, qty: 7 },
			{ id: 995, qty: 99 }
		])
	]);
	assert.deepEqual(carriedRunes(st), { law: 15, nature: 7, cosmic: 0 });
});

test('ownedRewards matches item ids across containers', () => {
	const st = state([cont('equipment', [{ id: 6908, qty: 1 }])]);
	assert.deepEqual(ownedRewards(table, st), ['beginner_wand']);
});

test('parsePoints keeps digits and clamps to the cap', () => {
	assert.equal(parsePoints('1.200', 8000), 1200);
	assert.equal(parsePoints('99999', 8000), 8000);
	assert.equal(parsePoints('abc', 8000), 0);
	assert.equal(parsePoints('', 8000), 0);
});

test('runeRows: short is never negative and unknown stays null', () => {
	const res = {
		buy: [],
		needed: z(),
		missing: z(),
		runes: { law: 100, nature: 50, cosmic: 10 },
		enchant_level: 5
	};
	const rows = runeRows(res, { law: 30, nature: 80, cosmic: 0 });
	assert.deepEqual(
		rows.map((r) => r.short),
		[70, 0, 10]
	);
	assert.deepEqual(
		runeRows(res, null).map((r) => r.short),
		[null, null, null]
	);
});
