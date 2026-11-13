import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	areaName,
	bankSource,
	carriedSource,
	conquests,
	diaryRows,
	factSource,
	questCounts,
	questRows,
	summaryCells
} from './personagem.ts';
import type { AccountState, CATierView, ContainerView, QuestView } from './types.ts';

const NOW = Date.parse('2026-11-13T12:00:00Z');
const at = (msAgo: number) => new Date(NOW - msAgo).toISOString();
const src = { source: 'plugin', observed_at: at(12_000) };

const skills = ['attack', 'strength', 'defence', 'hitpoints', 'prayer', 'ranged', 'magic'].map(
	(skill) => ({ skill, level: 60, xp: 273_742, ...src })
);

const quest = (name: string, state: QuestView['state'], observed = 12_000): QuestView => ({
	name,
	state,
	source: 'plugin',
	observed_at: at(observed)
});

const state: AccountState = {
	account: { id: 'acc_x', rsn: 'X', mode: 'ironman' } as AccountState['account'],
	total_level: 1204,
	quest_points: 132,
	skills,
	quests: [
		quest('Cook’s Assistant', 'FINISHED'),
		quest('Holy Grail', 'IN_PROGRESS'),
		quest('Dragon Slayer I', 'IN_PROGRESS', 60_000),
		quest('Monkey Madness I', 'NOT_STARTED')
	],
	diaries: [
		{ area: 'karamja', tier: 'easy', complete: true, ...src },
		{ area: 'karamja', tier: 'medium', complete: true, ...src },
		{ area: 'karamja', tier: 'hard', complete: false, ...src },
		{ area: 'karamja', tier: 'elite', complete: false, ...src },
		{ area: 'ardougne', tier: 'easy', complete: true, ...src },
		{ area: 'varrock', tier: 'easy', complete: false, ...src }
	],
	combat_achievements: [],
	collection_log: { obtained: 21, total: 1734, ...src },
	collection_log_pages: [],
	containers: []
};

test('summaryCells shows only what was read, with the totals in small print', () => {
	assert.deepEqual(summaryCells(state), [
		{ k: 'Total', v: '1.204' },
		{ k: 'Combat', v: '76' },
		{ k: 'Quests', v: '1', of: 'de 4 · 132 QP' },
		{ k: 'Diaries', v: '3', of: 'de 6' },
		{ k: 'Collection log', v: '21', of: 'de 1.734' }
	]);
	// Without quests or skills, only the quest points.
	assert.deepEqual(
		summaryCells({ ...state, total_level: undefined, skills: [], quests: [], diaries: [] }).map(
			(c) => c.k
		),
		['Quest points', 'Collection log']
	);
});

test('quests by filter, newest reading first, and the search', () => {
	assert.deepEqual(questCounts(state.quests), { active: 2, done: 1, missing: 1 });
	assert.deepEqual(
		questRows(state.quests, 'active').map((q) => q.name),
		['Holy Grail', 'Dragon Slayer I']
	);
	assert.deepEqual(
		questRows(state.quests, 'active', 'drag').map((q) => q.name),
		['Dragon Slayer I']
	);
	assert.deepEqual(
		questRows(state.quests, 'missing').map((q) => q.name),
		['Monkey Madness I']
	);
});

test('diary rows: one per area, the started first, unread tiers dotted', () => {
	const rows = diaryRows(state.diaries);
	assert.deepEqual(
		rows.map((r) => [r.name, r.tiers.map((t) => t.dot).join(' '), r.next]),
		[
			['Ardougne', 'on unk unk unk', 'medium'],
			['Karamja', 'on on off off', 'hard'],
			['Varrock', 'off unk unk unk', 'easy']
		]
	);
	assert.equal(areaName('kourend_and_kebos'), 'Kourend & Kebos');
});

test('conquests: Easy to Elite always, the higher tiers once they have one', () => {
	const tier = (t: CATierView['tier'], done: number, total: number): CATierView => ({
		tier: t,
		done,
		total,
		...src
	});
	const ca = [
		tier('easy', 3, 41),
		tier('medium', 3, 64),
		tier('hard', 0, 89),
		tier('elite', 0, 166),
		tier('master', 0, 160),
		tier('grandmaster', 0, 135)
	];
	const c = conquests(ca, state.collection_log);
	assert.deepEqual(
		c.rows.map((r) => `${r.label} ${r.done}/${r.total} ${r.pct}%`),
		['Easy 3/41 7%', 'Medium 3/64 5%', 'Hard 0/89 0%', 'Elite 0/166 0%']
	);
	assert.equal(c.note, 'Collection log: 21 de 1.734. Master e Grandmaster ainda sem nada.');
	const some = conquests([...ca.slice(0, 4), tier('master', 1, 160)], undefined);
	assert.equal(some.rows.at(-1)?.label, 'Master');
	assert.equal(some.note, '');
	const off = { ...tier('easy', 3, 41), game_done: 4 };
	assert.match(conquests([off], undefined).rows[0].warn ?? '', /o jogo conta 4/);
});

test('sources: the lamp is lit only for a reading from now', () => {
	assert.deepEqual(factSource(skills, NOW), { lamp: 'ok', text: 'plugin · há 12 s' });
	assert.deepEqual(factSource([{ source: 'hiscores', observed_at: at(2 * 3600_000) }], NOW), {
		lamp: 'rest',
		text: 'hiscores · há 2 h'
	});
	assert.deepEqual(factSource([], NOW), { lamp: 'off', text: 'sem leitura' });

	const box = (state: ContainerView['state'], msAgo: number): ContainerView => ({
		container: 'inventory',
		state,
		observed_at: at(msAgo),
		complete: true,
		items: []
	});
	assert.deepEqual(carriedSource([box('live', 12_000), undefined], NOW), {
		lamp: 'ok',
		text: 'ao vivo · há 12 s'
	});
	assert.deepEqual(carriedSource([box('live', 3 * 3600_000)], NOW), {
		lamp: 'rest',
		text: 'lido há 3 h'
	});
	assert.deepEqual(carriedSource([box('restored', 60_000)], NOW), {
		lamp: 'rest',
		text: 'espelho · há 1 min'
	});
	assert.equal(carriedSource([box('unavailable', 0)], NOW).lamp, 'off');

	const bank = { ...box('live', 42 * 60_000), container: 'bank' };
	assert.deepEqual(bankSource(bank, NOW), {
		lamp: 'rest',
		text: 'lido há 42 min, na última vez que ela abriu o banco',
		stale: false
	});
	assert.equal(bankSource({ ...bank, observed_at: at(3 * 86_400_000) }, NOW).stale, true);
	assert.equal(bankSource(undefined, NOW).text, 'sem leitura');
});
