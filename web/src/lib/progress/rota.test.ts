import { test } from 'node:test';
import assert from 'node:assert/strict';
import { highlight, legSummary } from './rota.ts';
import type { ContentStep } from './types.ts';

const step = (location: string, quests: string[] = []) =>
	({
		location,
		objectives: quests.map((q) => ({ key: `quest:${q}@finished`, role: 'completes' as const })),
		names: Object.fromEntries(quests.map((q) => [`quest:${q}`, q.replace(/_/g, ' ')]))
	}) as Pick<ContentStep, 'location' | 'objectives' | 'names'>;

test('legSummary names the leg by its first and last place and counts the quests', () => {
	const s = legSummary(
		[
			step('Lumbridge', ['a', 'b']),
			step('Varrock', ['c']),
			step('Varrock', ['d', 'e', 'f']),
			step('Al Kharid')
		],
		'1.1.178 – 1.1.282'
	);
	assert.equal(s.title, 'Lumbridge → Al Kharid, 6 quests');
	assert.equal(s.quests, 'a, b, c, d, e e mais 1');
	assert.equal(s.places, 'por Lumbridge, Varrock, Al Kharid');
	assert.equal(s.full, '6 quests: a, b, c, d, e, f · 3 locais, de Lumbridge a Al Kharid');
});

test('legSummary with one place and one quest, or nothing at all', () => {
	assert.equal(
		legSummary([step('Varrock', ['shield_of_arrav'])], 'x').title,
		'Varrock, shield of arrav'
	);
	assert.equal(legSummary([step('')], '1.1.1 – 1.1.9').title, 'Trecho 1.1.1 – 1.1.9');
});

test('highlight marks every match, case-insensitive', () => {
	assert.deepEqual(highlight('Finish The Grand tree', 'grand'), [
		{ t: 'Finish The ', m: false },
		{ t: 'Grand', m: true },
		{ t: ' tree', m: false }
	]);
	assert.deepEqual(highlight('abc', ''), [{ t: 'abc', m: false }]);
});
