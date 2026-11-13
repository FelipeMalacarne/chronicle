import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import type { AuditEntry, Run } from '../api/microbot.ts';
import {
	actionItem,
	counts,
	dayLabel,
	daySummary,
	feedDays,
	feedItems,
	levelItem,
	runItem,
	runLevels,
	runName,
	weekGains,
	weekRuns,
	windowStart,
	RUN_PAGE
} from './historico.ts';

// Tudo em hora local: os dias do Histórico são os do navegador.
const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m).getTime();
const iso = (d: number, h: number, m = 0) => new Date(at(d, h, m)).toISOString();
const now = at(13, 15, 0);

function run(p: Partial<Run> = {}): Run {
	return {
		id: 'run-1',
		className: 'net.runelite.client.plugins.microbot.fm.catswap.CatSwapPlugin',
		stop: [],
		state: 'done',
		startedAt: iso(13, 12, 24),
		finishedAt: iso(13, 12, 31),
		events: [
			{ at: iso(13, 12, 24), state: 'stopping', text: 'parou Tick Fisher num ponto seguro' },
			{ at: iso(13, 12, 31), state: 'done', text: 'religou Tick Fisher' }
		],
		...p
	};
}

function audit(p: Partial<AuditEntry> = {}): AuditEntry {
	return {
		id: 1,
		at: iso(13, 13, 28),
		actor: 'dev',
		origin: 'web',
		action: 'login',
		params: { world: 370 },
		ok: true,
		...p
	};
}

test('runName: the class without the package and "Plugin", in words', () => {
	assert.equal(
		runName('net.runelite.client.plugins.microbot.fm.catswap.CatSwapPlugin'),
		'Cat Swap'
	);
	assert.equal(runName('TickFisher'), 'Tick Fisher');
	assert.equal(runName('a.b.Plugin'), 'Plugin');
});

test('dayLabel and windowStart: by the local day', () => {
	assert.equal(dayLabel(at(13, 0, 5), now), 'Hoje');
	assert.equal(dayLabel(at(12, 23, 59), now), 'Ontem');
	// 10/11/2026 é uma terça.
	assert.equal(dayLabel(at(10, 9), now), 'Terça, 10/11');
	assert.equal(windowStart(now), at(7, 0));
	assert.equal(windowStart(now, 1), at(13, 0));
});

test('runItem: who started it, the outcome, how long and the trail', () => {
	const done = runItem(run({ result: { deathRunesGained: 100 } }), now);
	assert.equal(done.title, 'Cat Swap');
	assert.equal(done.time, '12:24');
	assert.equal(done.sub, 'você iniciou · +100 death runes');
	assert.equal(done.span, '7 min');
	assert.deepEqual(done.result, { label: 'feito', tone: 'ok' });
	assert.equal(done.live, undefined);
	assert.deepEqual(done.trail, [
		{ time: '12:24', text: 'parou Tick Fisher num ponto seguro' },
		{ time: '12:31', text: 'religou Tick Fisher' }
	]);

	const failed = runItem(
		run({ trigger: 'kitten_grown', state: 'failed', error: 'tempo esgotado' }),
		now
	);
	assert.equal(failed.sub, 'filhote cresceu · tempo esgotado');
	assert.equal(failed.problem, true);
	assert.deepEqual(failed.result, { label: 'falhou', tone: 'bad' });

	const going = runItem(
		run({ state: 'running', startedAt: iso(13, 14, 18), finishedAt: undefined }),
		now
	);
	assert.equal(going.live, 'ainda rodando');
	assert.equal(going.span, '42 min');
	assert.equal(going.result?.tone, 'going');
	// O servidor manda null quando o run ainda não tem trilha.
	assert.deepEqual(runItem(run({ events: null as unknown as Run['events'] }), now).trail, []);
});

test('runItem: a recurring routine says so; the levels gained while it ran join the line', () => {
	const levels = [
		{ account: 'a', skill: 'fishing', level: 75, at: iso(13, 12, 28) },
		{ account: 'a', skill: 'farming', level: 39, at: iso(13, 12, 30) },
		{ account: 'a', skill: 'farming', level: 40, at: iso(13, 12, 30) },
		// Antes do run e depois dele: fora.
		{ account: 'a', skill: 'hunter', level: 34, at: iso(13, 12, 0) },
		{ account: 'a', skill: 'cooking', level: 60, at: iso(13, 12, 40) }
	];
	const r = run({ routine: 'rtn_1' });
	assert.equal(runLevels(r, levels, now), 'Farming 38 → 40 · Fishing 74 → 75');
	const item = runItem(r, now, levels);
	assert.equal(item.sub, 'pela rotina · Farming 38 → 40 · Fishing 74 → 75');
	assert.equal(item.skill, 'farming');
	assert.equal(runItem(run(), now).skill, undefined);
	// Sem fim, a janela vai até agora.
	const going = run({ state: 'running', finishedAt: undefined });
	assert.equal(runLevels(going, levels, now), 'Farming 38 → 40 · Cooking 59 → 60');
	assert.equal(runLevels(r, [], now), '');
});

test('actionItem: sentences for the owner, runs left to the runs, failures are problems', () => {
	const login = actionItem(audit())!;
	assert.equal(login.title, 'Você entrou no mundo 370');
	assert.equal(login.sub, 'pelo Cliente');
	assert.equal(login.mine, true);
	assert.equal(login.glyph, 'conta');

	const stop = actionItem(audit({ action: 'worker_stop', params: {} }))!;
	assert.equal(stop.title, 'Worker parado');
	assert.equal(stop.sub, 'você parou · saiu do jogo');
	assert.equal(stop.glyph, 'porta');

	const refused = actionItem(
		audit({ action: 'worker_start', params: {}, ok: false, error: 'no game session' })
	)!;
	assert.equal(refused.problem, true);
	assert.equal(refused.sub, 'você iniciou · no game session');
	assert.deepEqual(refused.result, { label: 'falhou', tone: 'bad' });

	// O run já aparece como execução.
	assert.equal(actionItem(audit({ action: 'run_start', params: { className: 'x.Y' } })), null);
	assert.equal(actionItem(audit({ action: 'run_end', origin: 'agent', ok: false })), null);
	// Um fazer agora que nem começou só existe na auditoria.
	const notStarted = actionItem(
		audit({
			action: 'run_start',
			params: { className: 'a.CatSwapPlugin' },
			ok: false,
			error: 'busy'
		})
	)!;
	assert.equal(notStarted.title, 'Você pediu Cat Swap');
	assert.equal(notStarted.sub, 'não começou · busy');

	const routineSet = actionItem(
		audit({ action: 'recurring_set', params: { name: 'Farm run', enabled: true } })
	)!;
	assert.equal(routineSet.title, 'Você gravou a rotina Farm run');
	assert.equal(routineSet.sub, 'ligada');
	const saved = actionItem(
		audit({ action: 'profile_save', params: { to: 'account', keys: [{ group: 'g', key: 'k' }] } })
	)!;
	assert.equal(saved.title, 'Você guardou 1 ajuste do cliente no perfil');
	assert.equal(saved.sub, 'só desta conta');

	const walk = actionItem(audit({ action: 'walk', params: { x: 3200, y: 3201 } }))!;
	assert.equal(walk.title, 'Andar · 3200, 3201');

	const routine = actionItem(audit({ origin: 'routine', action: 'logout', params: {} }))!;
	assert.equal(routine.mine, false);
	assert.equal(routine.sub, 'sozinho');
});

test('feedItems: merged newest first, inside the window only; counts per filter', () => {
	const since = windowStart(now);
	const items = feedItems(
		{
			runs: [
				run(),
				run({ id: 'run-0', startedAt: iso(5, 10), finishedAt: iso(5, 11) }),
				run({ id: 'run-2', state: 'failed', startedAt: iso(12, 18, 3), finishedAt: iso(12, 18, 4) })
			],
			levels: [
				{ account: 'acc_a', skill: 'fishing', level: 76, at: iso(13, 14, 6) },
				{ account: 'acc_a', skill: 'fishing', level: 75, at: iso(13, 14, 6) },
				{ account: 'acc_a', skill: 'hunter', level: 34, at: iso(12, 16, 30) }
			],
			audit: [audit(), audit({ id: 2, action: 'run_end', origin: 'agent' })]
		},
		since,
		now
	);
	assert.deepEqual(
		items.map((i) => `${i.time} ${i.title}`),
		[
			'14:06 Fishing 76',
			'14:06 Fishing 75',
			'13:28 Você entrou no mundo 370',
			'12:24 Cat Swap',
			'18:03 Cat Swap',
			'16:30 Hunter 34'
		]
	);
	assert.deepEqual(counts(items), { all: 6, runs: 2, levels: 3, mine: 1, problems: 1 });
});

test('feedDays: one group per day, summary of the whole day, filtered items', () => {
	const items = [
		levelItem({ account: 'a', skill: 'fishing', level: 76, at: iso(13, 14, 6) }),
		runItem(run(), now),
		levelItem({ account: 'a', skill: 'hunter', level: 34, at: iso(12, 16, 30) }),
		runItem(
			run({ id: 'r2', state: 'failed', startedAt: iso(12, 18), finishedAt: iso(12, 18, 1) }),
			now
		)
	].sort((a, b) => b.at - a.at);
	const days = feedDays(items, 'all', now, 114_000);
	assert.deepEqual(
		days.map((d) => [d.label, d.summary, d.items.length]),
		[
			['Hoje', '114k xp · 1 execução', 2],
			['Ontem', '1 nível · 1 execução · 1 problema', 2]
		]
	);
	// Hoje não teve problema: o dia some no filtro; o resumo segue o dia inteiro.
	assert.deepEqual(
		feedDays(items, 'problems', now).map((d) => [d.label, d.summary, d.items.length]),
		[['Ontem', '1 nível · 1 execução · 1 problema', 1]]
	);
	assert.equal(daySummary([]), '');
});

test('weekRuns: counted inside the window; partial when the page is full', () => {
	const since = windowStart(now);
	const w = weekRuns(
		[
			run(),
			run({ state: 'failed', error: 'sem sementes', startedAt: iso(12, 18) }),
			run({ state: 'canceled' }),
			run({ state: 'failed', startedAt: iso(1, 10) })
		],
		since
	);
	assert.deepEqual(w, {
		done: 1,
		failed: 1,
		canceled: 1,
		lastFailed: { name: 'Cat Swap', error: 'sem sementes' },
		partial: false
	});
	const full = Array.from({ length: RUN_PAGE }, () => run());
	assert.equal(weekRuns(full, since).partial, true);
});

test('weekGains: "71 → 76 +412k", no arrow without a new level', () => {
	assert.deepEqual(
		weekGains([
			{ skill: 'fishing', from_level: 71, to_level: 76, xp: 412_000 },
			{ skill: 'cooking', from_level: 60, to_level: 60, xp: 8_200 },
			{ skill: 'magic', from_level: 50, to_level: 50, xp: 0 }
		]),
		[
			{ skill: 'fishing', name: 'Fishing', levels: '71 → 76', up: true, xp: '+412k' },
			{ skill: 'cooking', name: 'Cooking', levels: '60', up: false, xp: '+8,2k' }
		]
	);
});
