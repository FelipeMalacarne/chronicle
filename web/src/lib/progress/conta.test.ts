import { test } from 'node:test';
import assert from 'node:assert/strict';
import { combatLevel, shortQty, toNext, XP_TABLE } from './conta.ts';

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
