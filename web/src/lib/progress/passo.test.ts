import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	actionsOf,
	byteToIndex,
	clauses,
	missingOf,
	sentences,
	type PassoObjective
} from './passo.ts';

const bytes = (s: string) => new TextEncoder().encode(s).length;

test('byteToIndex maps UTF-8 byte offsets to string positions', () => {
	const t = 'start Daddy’s Home.';
	assert.equal(byteToIndex(t, bytes('start Daddy’s Home')), 'start Daddy’s Home'.length);
	assert.equal(byteToIndex(t, 0), 0);
	assert.equal(byteToIndex('a😀b', bytes('a😀')), 3);
});

test('sentences split lines and sentences, not abbreviations', () => {
	const t =
		'- Teleport to Varrock (optional: e.g. clan wars first). Start Gertrude’s cat, go to Varrock square.\n- Bank. 3 trips to the mine';
	assert.deepEqual(
		sentences(t).map((s) => s.text),
		[
			'Teleport to Varrock (optional: e.g. clan wars first).',
			'Start Gertrude’s cat, go to Varrock square.',
			'Bank.',
			'3 trips to the mine'
		]
	);
	for (const s of sentences(t)) assert.equal(t.slice(s.start, s.end), s.text);
});

test('actionsOf puts each objective in the sentence that quotes it', () => {
	const t =
		'Use your chronicle to teleport. Start Gertrude’s cat, pick up doogle leaves. Start Daddy’s Home.';
	const span = (q: string): [number, number] => {
		const i = t.indexOf(q);
		return [bytes(t.slice(0, i)), bytes(t.slice(0, i + q.length))];
	};
	const objs: PassoObjective[] = [
		{
			key: 'quest:gertrudes_cat@started',
			role: 'starts',
			label: 'Gertrude’s Cat',
			verdict: 'met',
			span: span('Start Gertrude’s cat')
		},
		{
			key: 'item:1573x1',
			role: 'completes',
			label: 'Doogle leaves',
			verdict: 'unmet',
			span: span('pick up doogle leaves')
		},
		{
			key: 'quest:daddys_home@started',
			role: 'starts',
			label: 'Daddy’s Home',
			verdict: 'unknown',
			span: span('Start Daddy’s Home')
		},
		{ key: 'skill:agility>=1', role: 'requires', label: '1 Agility', verdict: 'met' }
	];
	const a = actionsOf(t, objs);
	assert.deepEqual(
		a.map((x) => x.state),
		['plain', 'open', 'unknown']
	);
	assert.deepEqual(
		a[1].pieces.map((p) => [p.text, p.objective?.key ?? '']),
		[
			['Start Gertrude’s cat', 'quest:gertrudes_cat@started'],
			[', ', ''],
			['pick up doogle leaves', 'item:1573x1'],
			['.', '']
		]
	);
	assert.equal(a[2].pieces[0].text, 'Start Daddy’s Home');
	// from skips the TL;DR the card already shows
	assert.equal(actionsOf(t, objs, t.indexOf('Start Daddy')).length, 1);
});

test('missingOf lists what the step does and the account lacks', () => {
	const o = (key: string, role: string, label: string, verdict: 'met' | 'unmet' | 'unknown') => ({
		key,
		role,
		label,
		verdict
	});
	assert.deepEqual(
		missingOf([
			o('quest:shield_of_arrav@started', 'starts', 'Shield of Arrav', 'unmet'),
			o('quest:holy_grail@finished', 'completes', 'Holy Grail', 'unmet'),
			o('item:1573x1', 'completes', 'Doogle leaves', 'unmet'),
			o('quest:x@finished', 'completes', 'X', 'unknown'),
			o('skill:agility>=50', 'requires', '50 Agility', 'unmet'),
			o('quest:y@finished', 'completes', 'Y', 'met')
		]),
		['começar Shield of Arrav', 'terminar Holy Grail', 'Doogle leaves']
	);
});

test('clauses split a long sentence at commas outside parentheses', () => {
	const t =
		'Talk to Aris Maye (formerly Gypsy Aris, in the square) to start Demon Slayer, Romeo to start Romeo and Juliet, then north, Noah to start and complete Children of the Sun, Reldo to progress The Knight’s Sword. Bank.';
	assert.deepEqual(
		clauses(t).map((c) => c.text),
		[
			'Talk to Aris Maye (formerly Gypsy Aris, in the square) to start Demon Slayer,',
			'Romeo to start Romeo and Juliet, then north,',
			'Noah to start and complete Children of the Sun,',
			'Reldo to progress The Knight’s Sword.',
			'Bank.'
		]
	);
	for (const c of clauses(t)) assert.equal(t.slice(c.start, c.end), c.text);
	// a short sentence keeps its commas
	assert.deepEqual(
		clauses('Buy 5 ale, 39 mind bombs and 12 stouts.').map((c) => c.text),
		['Buy 5 ale, 39 mind bombs and 12 stouts.']
	);
});
