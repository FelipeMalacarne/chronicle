import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	bankMarks,
	gpLabel,
	headerFacts,
	highlight,
	legSummary,
	snippet,
	stepFacts,
	stepMatches,
	timeLabel
} from './rota.ts';
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

test('stepMatches looks at ref, text, location and resolved names', () => {
	const s = {
		ref: 'CH1.31',
		text: 'Barb fish to 50 agility',
		location: 'Otto',
		names: { 'quest:death_on_the_isle': 'Death on the Isle' }
	};
	assert.equal(stepMatches(s, 'ch1.31'), true);
	assert.equal(stepMatches(s, 'AGILITY'), true);
	assert.equal(stepMatches(s, 'otto'), true);
	assert.equal(stepMatches(s, 'death on'), true);
	assert.equal(stepMatches(s, 'corrupted'), false);
	assert.equal(stepMatches(s, '  '), true);
});

test('snippet centers on the first match and marks cut ends', () => {
	const text = `${'a '.repeat(100)}Corrupted Gauntlet${' b'.repeat(100)}`;
	const cut = snippet(text, 'corrupted', 60);
	assert.ok(cut.startsWith('…') && cut.endsWith('…'));
	assert.ok(cut.includes('Corrupted Gauntlet'));
	assert.equal(snippet('short\n text', 'x'), 'short text');
	assert.ok(snippet(text, 'nowhere', 60).startsWith('a a'));
});

test('bankMarks marks the first step of each bank leg after the first one', () => {
	const st = (id: string) => ({ id }) as ContentStep;
	const marks = bankMarks([
		{ kind: 'bank_leg', steps: [st('a'), st('b')], bankActions: [] },
		{
			kind: 'bank_leg',
			steps: [st('c')],
			loadout: { status: 'partial', inventory: [{ name: 'Knife' }, { name: 'Coins' }] },
			bankActions: [{ type: 'bank', position: 'start', text: 'bank your money' }]
		},
		{ kind: 'sequence', steps: [st('d')], bankActions: [] },
		{ kind: 'bank_leg', steps: [st('e')], bankActions: [] }
	]);
	assert.deepEqual([...marks.keys()], ['c', 'e']);
	assert.equal(marks.get('c')?.label, 'banco · 2 itens');
	assert.equal(
		marks.get('c')?.title,
		'O guia manda passar no banco antes deste passo · “bank your money” · lista parcial do que levar'
	);
	assert.equal(marks.get('e')?.label, 'banco');
});

test('stepFacts reads the GP after the step and the guide time of BRUHsailer', () => {
	const notes = (...t: string[]) => ({ notes: t.map((text) => ({ kind: 'tip', text })) });
	assert.equal(
		stepFacts(notes('GP after step: 777349 gp', 'Items needed: knife', 'Total time: 5d0h')),
		'777.349 gp · 5 d 0 h'
	);
	assert.equal(stepFacts(notes('GP after step: 1719 gp (after the bond)')), '1.719 gp');
	assert.equal(stepFacts(notes('Something else')), '');
	assert.equal(timeLabel('12d23h'), '12 d 23 h');
	assert.equal(gpLabel('-500 gp'), '-500 gp');
});

test('headerFacts names the GP and the guide time, and keeps only the GP at 640', () => {
	const notes = (...t: string[]) => ({ notes: t.map((text) => ({ kind: 'tip', text })) });
	const both = notes('GP after step: 777349 gp', 'Total time: 5d0h');
	assert.equal(headerFacts(both), 'GP depois 777.349 · tempo do guia 5 d 0 h');
	assert.equal(headerFacts(both, true), 'GP depois 777.349');
	// A step with the time but no GP never calls the time "GP".
	assert.equal(headerFacts(notes('Total time: 5d0h')), 'tempo do guia 5 d 0 h');
	assert.equal(headerFacts(notes('Total time: 5d0h'), true), '');
	assert.equal(headerFacts(notes('Something else')), '');
});
