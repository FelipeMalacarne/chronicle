import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
	carriedRunes,
	magicLevel,
	ownedRewards,
	parsePoints,
	readPoints,
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
		{ id: 'rune_pouch', name: 'Rune pouch', item_id: 12791, cost: z() },
		{ id: 'bones_to_peaches', name: 'Bones to Peaches', item_id: 6926, cost: z() }
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

const counter = (key: string, value: number, observed_at = '2026-11-16T03:00:00Z') => ({
	key,
	value,
	source: 'plugin',
	observed_at
});

test('ownedRewards counts Bones to Peaches only from the unlock the plugin read', () => {
	const st = (minigame: unknown[]) =>
		({ containers: [], skills: [], minigame }) as unknown as AccountState;
	assert.deepEqual(ownedRewards(table, st([])), []);
	assert.deepEqual(ownedRewards(table, st([counter('mta.bones_to_peaches', 0)])), []);
	assert.deepEqual(ownedRewards(table, st([counter('mta.bones_to_peaches', 1)])), [
		'bones_to_peaches'
	]);
});

test('readPoints needs every room and takes the newest reading', () => {
	const st = (minigame: unknown[]) => ({ minigame }) as unknown as AccountState;
	assert.equal(readPoints(st([counter('mta.telekinetic', 204)])), null);
	const all = [
		counter('mta.telekinetic', 204, '2026-11-16T02:00:00Z'),
		counter('mta.graveyard', 170),
		counter('mta.enchantment', 1342),
		counter('mta.alchemist', 140),
		counter('mta.bones_to_peaches', 1)
	];
	assert.deepEqual(readPoints(st(all)), {
		points: { telekinetic: 204, graveyard: 170, enchantment: 1342, alchemist: 140 },
		observedAt: Date.parse('2026-11-16T03:00:00Z')
	});
});
