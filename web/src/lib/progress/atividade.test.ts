import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import type { Run } from '../api/microbot.ts';
import { accountPills, diary, fleetRuns, runsHeader, xpRows } from './atividade.ts';
import { dayStart } from './historico.ts';
import type { AccountRhythm, AccountView, FleetRhythm } from './types.ts';

const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m).getTime();
const iso = (d: number, h: number, m = 0) => new Date(at(d, h, m)).toISOString();
const now = at(13, 15, 0);

function run(p: Partial<Run> = {}): Run {
	return {
		id: 'run-1',
		className: 'a.b.FarmRunPlugin',
		stop: [],
		state: 'done',
		startedAt: iso(13, 12, 24),
		finishedAt: iso(13, 12, 31),
		events: [],
		...p
	};
}

test('fleetRuns: every account, today only, newest first; the header counts them', () => {
	const rows = fleetRuns(
		[
			{
				account: 'acc_a',
				rsn: 'Felipe Pesca',
				runs: [
					run({ state: 'running', startedAt: iso(13, 14, 18), finishedAt: undefined }),
					run({ id: 'run-0', startedAt: iso(12, 22) })
				]
			},
			{
				account: 'acc_b',
				rsn: 'Iron Felipe',
				runs: [
					run({
						className: 'a.CatSwapPlugin',
						trigger: 'kitten_grown',
						state: 'failed',
						error: 'tempo esgotado',
						startedAt: iso(13, 13, 2),
						finishedAt: iso(13, 13, 7)
					})
				]
			},
			{ account: 'acc_c', rsn: 'Felipe Lenha', runs: [run()] }
		],
		dayStart(now),
		now
	);
	assert.deepEqual(
		rows.map((r) => [r.time, r.rsn, r.name, r.sub, r.span, r.end.label]),
		[
			['14:18', 'Felipe Pesca', 'Farm Run', 'você iniciou', '42 min', 'rodando'],
			['13:02', 'Iron Felipe', 'Cat Swap', 'filhote cresceu · tempo esgotado', '5 min', 'falhou'],
			['12:24', 'Felipe Lenha', 'Farm Run', 'você iniciou', '7 min', 'feito']
		]
	);
	assert.equal(runsHeader(rows), 'hoje · 1 feita · 1 falhou · 1 rodando');
	assert.equal(runsHeader([]), 'hoje');
});

test('fleetRuns: a run from yesterday still going stays; the account levels join its line', () => {
	const rows = fleetRuns(
		[
			{
				account: 'acc_a',
				rsn: 'Felipe Pesca',
				runs: [
					run({ id: 'run-9', state: 'running', startedAt: iso(12, 23), finishedAt: undefined }),
					run({ id: 'run-8', startedAt: iso(12, 20), finishedAt: iso(12, 21) })
				],
				levels: [{ account: 'acc_a', skill: 'fishing', level: 75, at: iso(13, 9, 12) }]
			}
		],
		dayStart(now),
		now
	);
	assert.deepEqual(
		rows.map((r) => [r.time, r.sub, r.end.label, r.skill]),
		[['23:00', 'você iniciou · Fishing 74 → 75', 'rodando', 'fishing']]
	);
});

const rhythm = (account: string, xp: number): AccountRhythm => ({
	account,
	training: null,
	session: null,
	gp: null,
	today: { since: iso(13, 0), xp, levels: 0, skills: [] }
});

test('xpRows: most XP first, bars relative to the top, accounts without XP left out', () => {
	const rh = {
		since: iso(13, 0),
		accounts: [rhythm('acc_b', 78_000), rhythm('acc_a', 114_000), rhythm('acc_c', 0)],
		totals: { xp_today: 192_000, levels_today: 0, gaining: 1 }
	} as FleetRhythm;
	const names = { acc_a: 'Felipe Pesca', acc_b: 'Iron Felipe' };
	const all = xpRows(rh, names);
	assert.deepEqual(
		all.rows.map((r) => [r.rsn, r.label, r.pct]),
		[
			['Felipe Pesca', '114k', 100],
			['Iron Felipe', '78k', 68]
		]
	);
	assert.equal(all.total, 192_000);
	assert.deepEqual(
		xpRows(rh, names, 'acc_b').rows.map((r) => r.account),
		['acc_b']
	);
	assert.deepEqual(xpRows(null, names), { rows: [], total: 0 });
});

test('diary: "subiu para Fishing 76" with the account name, filtered by account', () => {
	const events = [
		{ account: 'acc_a', skill: 'fishing', level: 76, at: iso(13, 14, 6) },
		{ account: 'acc_b', skill: 'woodcutting', level: 68, at: iso(13, 13, 40) }
	];
	const names = { acc_a: 'Felipe Pesca' };
	assert.deepEqual(
		diary(events, names).map((r) => [r.time, r.rsn, r.text]),
		[
			['14:06', 'Felipe Pesca', 'subiu para Fishing 76'],
			['13:40', 'acc_b', 'subiu para Woodcutting 68']
		]
	);
	assert.deepEqual(
		diary(events, names, 'acc_b').map((r) => r.account),
		['acc_b']
	);
});

test('accountPills: all up to the max, else the first ones and "+N"; the chosen stays visible', () => {
	const acc = (id: string): AccountView => ({
		id,
		rsn: id,
		mode: 'ironman',
		guide_id: 'bruhsailer-main',
		layer: 'enhanced'
	});
	const four = ['a', 'b', 'c', 'd'].map(acc);
	assert.deepEqual(accountPills(four, null, false), { shown: four, more: 0 });
	const six = ['a', 'b', 'c', 'd', 'e', 'f'].map(acc);
	assert.deepEqual(
		(({ shown, more }) => [shown.map((a) => a.id), more])(accountPills(six, null, false)),
		[['a', 'b', 'c'], 3]
	);
	assert.deepEqual(
		(({ shown, more }) => [shown.map((a) => a.id), more])(accountPills(six, 'f', false)),
		[['a', 'b', 'f'], 3]
	);
	assert.equal(accountPills(six, null, true).more, 0);
});
