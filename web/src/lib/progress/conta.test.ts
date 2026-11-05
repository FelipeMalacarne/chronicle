import { test } from 'node:test';
import assert from 'node:assert/strict';
import { combatLevel, equipmentLayout, shortQty, toNext, XP_TABLE } from './conta.ts';

test('XP table matches the game', () => {
	assert.equal(XP_TABLE[2], 83);
	assert.equal(XP_TABLE[50], 101_333);
	assert.equal(XP_TABLE[99], 13_034_431);
});

test('toNext is the fraction from the current level to the next', () => {
	assert.equal(toNext(1, 0), 0);
	assert.ok(Math.abs(toNext(1, 41.5) - 0.5) < 1e-9);
	assert.equal(toNext(99, 200_000_000), 1);
});

test('combatLevel follows the game formula and needs every combat skill', () => {
	const s = (o: Record<string, number>) =>
		Object.entries(o).map(([skill, level]) => ({ skill, level }));
	assert.equal(
		combatLevel(
			s({ attack: 1, strength: 1, defence: 1, hitpoints: 10, prayer: 1, ranged: 1, magic: 1 })
		),
		3
	);
	assert.equal(
		combatLevel(
			s({ attack: 99, strength: 99, defence: 99, hitpoints: 99, prayer: 99, ranged: 99, magic: 99 })
		),
		126
	);
	assert.equal(combatLevel(s({ attack: 1 })), null);
});

test('shortQty', () => {
	assert.equal(shortQty(8200), '8200');
	assert.equal(shortQty(82_000), '82k');
	assert.equal(shortQty(12_500), '12.5k');
	assert.equal(shortQty(12_000_000), '12m');
});

test('equipmentLayout puts each item in its one slot, 2h in the weapon slot', () => {
	const { cells, rest } = equipmentLayout([
		{ id: 1153, name: 'Iron full helm', qty: 1, slot: 'head' },
		{ id: 1319, name: 'Rune 2h sword', qty: 1, slot: '2h' },
		{ id: 884, name: 'Iron arrow', qty: 212, slot: 'ammo' },
		{ id: 9999, name: 'Sem slot', qty: 1 },
		{ id: 1155, name: 'Bronze full helm', qty: 1, slot: 'head' }
	]);
	const at = (slot: string) => cells.find((c) => c.slot === slot)!;
	assert.equal(cells.length, 11);
	assert.equal(at('head').item?.id, 1153);
	assert.deepEqual([at('head').row, at('head').col], [1, 2]);
	assert.equal(at('weapon').item?.name, 'Rune 2h sword');
	assert.equal(at('shield').item, undefined);
	assert.equal(at('shield').blockedBy2h, true);
	assert.equal(at('ammo').item?.qty, 212);
	assert.equal(at('ring').item, undefined);
	assert.deepEqual(
		rest.map((r) => r.id),
		[9999, 1155]
	);
});
