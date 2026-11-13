import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	accountQueue,
	accountRows,
	doSteps,
	draftOf,
	draftProblem,
	fleetRoutines,
	fleetRuns,
	headline,
	intervalText,
	newDraft,
	offlineHold,
	pluginLabel,
	queueHead,
	queueOrder,
	recurringBody,
	recurringWhen,
	runLine,
	runsOfRow,
	savePlan,
	triggerBody,
	watchText,
	watcherLine,
	whenRule,
	whenText,
	withEnabled,
	type RoutineRow
} from './rotinas.ts';
import type { Recurring, RecurringState, Run } from '../api/microbot.ts';

// Local time: the screen reads the browser's clock.
const now = new Date(2026, 10, 13, 14, 0, 0);
const at = (h: number, m = 0, day = 13) => new Date(2026, 10, day, h, m, 0).toISOString();

const BIRD = 'a.fm.birdhouse.BirdhouseRunnerPlugin';
const FISH = 'a.fm.tickfisher.TickFisherPlugin';

const rc = (over: Partial<RecurringState> = {}): RecurringState => ({
	id: 'rtn_0001aaaa',
	name: 'Bird houses',
	enabled: true,
	clock: 'BIRD_HOUSE',
	priority: 50,
	run: { className: BIRD, stop: [FISH] },
	updatedAt: at(9),
	due: false,
	...over
});

const run = (over: Partial<Run> = {}): Run => ({
	id: 'run-1',
	className: BIRD,
	stop: [FISH],
	state: 'done',
	startedAt: at(12),
	finishedAt: at(12, 6),
	events: [],
	...over
});

test('intervalText: minutes, hours and days in PT-BR', () => {
	assert.equal(intervalText(10), '10 min');
	assert.equal(intervalText(60), '1 h');
	assert.equal(intervalText(150), '2 h 30');
	assert.equal(intervalText(1440), '1 dia');
	assert.equal(intervalText(1500), '1 dia 1 h');
	assert.equal(intervalText(1505), '1 dia 1 h 05');
	assert.equal(intervalText(10080), '7 dias');
});

test('whenText: now, within the hour, today, tomorrow, later', () => {
	assert.equal(whenText(at(13, 59), now), 'agora');
	assert.equal(whenText(at(14, 12), now), 'em 12 min');
	assert.equal(whenText(new Date(now.getTime() + 20_000).toISOString(), now), 'em 1 min');
	assert.equal(whenText(at(15), now), '15:00');
	assert.equal(whenText(at(8, 10, 14), now), 'amanhã 08:10');
	assert.equal(whenText(at(9, 0, 16), now), '16/11 09:00');
});

test('pluginLabel: the client name when known, else the class split into words', () => {
	assert.equal(pluginLabel(BIRD), 'Birdhouse Runner');
	assert.equal(pluginLabel('a.fm.catswap.CatSwapPlugin'), 'Cat Swap');
	assert.equal(pluginLabel(BIRD, new Map([[BIRD, 'Bird House Runner']])), 'Bird House Runner');
});

test('whenRule: clock, clock with a fallback interval, interval only', () => {
	assert.equal(whenRule({ clock: 'HERB' }), 'quando Herb Patches fica pronto');
	assert.equal(
		whenRule({ clock: 'BIRD_HOUSE', intervalMin: 50 }),
		'quando Bird Houses fica pronto; sem leitura, a cada 50 min'
	);
	assert.equal(whenRule({ intervalMin: 120 }), 'a cada 2 h depois do último run');
});

test('recurringWhen: every reason of the watcher, with the queue column and the why', () => {
	assert.deepEqual(recurringWhen(rc({ reason: 'clock_ready', due: true }), now).when, 'agora');
	assert.equal(recurringWhen(rc({ reason: 'first', due: true }), now).tone, 'due');
	const waiting = recurringWhen(rc({ reason: 'clock_waiting', nextAt: at(14, 32) }), now);
	assert.equal(waiting.when, 'em 32 min');
	assert.equal(waiting.tone, 'wait');
	assert.equal(recurringWhen(rc({ reason: 'clock_waiting' }), now).when, 'relógio parado');
	assert.equal(
		recurringWhen(rc({ reason: 'interval_waiting', clock: undefined, nextAt: at(16) }), now).when,
		'16:00'
	);
	assert.equal(recurringWhen(rc({ reason: 'clock_stale' }), now).when, 'esperando o relógio');
	assert.equal(recurringWhen(rc({ reason: 'no_clock' }), now).when, 'sem relógio');
	const held = recurringWhen(rc({ reason: 'held' }), now);
	assert.equal(held.when, 'segura');
	assert.match(held.why, /até você salvá-la de novo/);
	assert.match(recurringWhen(rc({ reason: 'held_agent' }), now).why, /salvar uma delas/);
	assert.equal(recurringWhen(rc({ reason: 'off', enabled: false }), now).tone, 'off');
	// From the database (no watcher): nothing to say unless a hold is known.
	assert.equal(recurringWhen(rc(), now).when, '');
	assert.equal(recurringWhen(rc(), now, 'held').when, 'segura');
	assert.equal(recurringWhen(rc({ enabled: false }), now).when, 'desligada');
});

test('offlineHold: the agent rules from what the database holds', () => {
	// The last run failed after the last save: held until saved again.
	const failed = rc({ lastRun: { state: 'failed', startedAt: at(12), finishedAt: at(12, 3) } });
	assert.equal(offlineHold(failed, [failed], []), 'held');
	// Saved again after the failure: free.
	const saved = { ...failed, updatedAt: at(13) };
	assert.equal(offlineHold(saved, [saved], []), null);
	// A run that never finished counts as failed when it started.
	const lost = rc({ lastRun: { state: 'running', startedAt: at(12) } });
	assert.equal(offlineHold(lost, [lost], []), 'held');
	// Switched off: never held.
	assert.equal(offlineHold({ ...failed, enabled: false }, [failed], []), null);
	// The account's last run of any kind failed after the latest save of any routine.
	const ok = rc();
	const other: Recurring = { ...rc({ id: 'rtn_0002bbbb' }), updatedAt: at(10) };
	assert.equal(
		offlineHold(ok, [ok, other], [run({ state: 'canceled', finishedAt: at(11) })]),
		'held_agent'
	);
	assert.equal(
		offlineHold(ok, [ok, other], [run({ state: 'canceled', finishedAt: at(9, 30) })]),
		null
	);
	assert.equal(offlineHold(ok, [ok, other], [run()]), null);
});

test('queueOrder: due by priority, then by time, events and waiting, held, off', () => {
	const row = (key: string, over: Partial<RoutineRow>): RoutineRow => ({
		key,
		kind: 'recurring',
		name: key,
		line: '',
		enabled: true,
		priority: 0,
		when: '',
		why: '',
		tone: 'idle',
		run: { className: BIRD },
		...over
	});
	const order = queueOrder([
		row('off', { tone: 'off', priority: 100 }),
		row('late', { tone: 'wait', nextAt: at(16) }),
		row('held', { tone: 'held' }),
		row('soon', { tone: 'wait', nextAt: at(14, 30) }),
		row('due-low', { tone: 'due', priority: 10 }),
		row('event', { tone: 'idle' }),
		row('due-high', { tone: 'due', priority: 90 })
	]).map((r) => r.key);
	assert.deepEqual(order, ['due-high', 'due-low', 'soon', 'late', 'event', 'held', 'off']);
});

test('accountRows: recurring routines and triggers in queue order, holds marked', () => {
	const rows = accountRows(
		{
			recurring: [
				rc({
					id: 'rtn_0001aaaa',
					name: 'Bird houses',
					reason: 'clock_waiting',
					nextAt: at(14, 32)
				}),
				rc({
					id: 'rtn_0002bbbb',
					name: 'Farm run',
					clock: 'HERB',
					reason: 'held',
					run: { className: 'a.FarmRunnerPlugin' }
				}),
				rc({ id: 'rtn_0003cccc', name: 'Herbs', reason: 'clock_ready', due: true, priority: 80 })
			]
		},
		{
			triggers: [
				{ event: 'kitten_grown', enabled: true, run: { className: 'a.fm.catswap.CatSwapPlugin' } }
			],
			watching: false
		},
		[],
		now
	);
	assert.deepEqual(
		rows.map((r) => [r.name, r.when]),
		[
			['Herbs', 'agora'],
			['Bird houses', 'em 32 min'],
			['Trocar o gato', 'sem vigia'],
			['Farm run', 'segura']
		]
	);
	assert.equal(rows[1].line, 'quando Bird Houses fica pronto · Birdhouse Runner');
	assert.equal(rows[2].line, 'quando o filhote crescer · Cat Swap');
	assert.equal(rows[3].held, 'held');
	// The state the watcher adds stays out of the routine the form edits.
	assert.equal('due' in (rows[0].recurring ?? {}), false);
});

test('accountRows: from the database, the hold is computed from the last run', () => {
	const rows = accountRows(
		{
			recurring: [rc({ lastRun: { state: 'failed', startedAt: at(12), finishedAt: at(12, 2) } })],
			watchError: 'no agent connected: it gets the recurring routines when it links'
		},
		null,
		[],
		now
	);
	assert.equal(rows[0].when, 'segura');
	assert.equal(rows[0].held, 'held');
});

test('watchText and watcherLine: the server reasons in PT-BR, then the watcher', () => {
	assert.equal(
		watchText('no agent connected: it gets the recurring routines when it links'),
		'sem agent ligado: recebe as rotinas quando ligar'
	);
	assert.match(watchText('the agent does not run recurring routines yet: update') ?? '', /antigo/);
	assert.equal(watchText('the agent did not answer: timeout'), 'o agent não respondeu');
	assert.equal(watchText(undefined), null);
	assert.equal(
		watcherLine({ recurring: [], waiting: 'esperando: em combate' }, now),
		'esperando: em combate'
	);
	assert.equal(
		watcherLine(
			{ recurring: [], clockError: 'the Time Tracking plugin is off in the client' },
			now
		),
		'o Time Tracking está desligado no cliente'
	);
	assert.equal(
		watcherLine(
			{
				recurring: [],
				last: { at: at(13, 58), routine: 'rtn_0001aaaa', name: 'Bird houses', runId: 'run-3' }
			},
			now
		),
		'13:58 · Bird houses começou'
	);
	assert.equal(watcherLine({ recurring: [], checkedAt: at(13, 59) }, now), 'olhou às 13:59');
});

test('runsOfRow: a recurring routine by its id, a trigger by its event', () => {
	const runs = [
		run({ id: 'run-3', routine: 'rtn_0001aaaa' }),
		run({ id: 'run-2', trigger: 'kitten_grown' }),
		run({ id: 'run-1' })
	];
	assert.deepEqual(
		runsOfRow({ kind: 'recurring', id: 'rtn_0001aaaa' }, runs).map((r) => r.id),
		['run-3']
	);
	assert.deepEqual(
		runsOfRow({ kind: 'trigger', event: 'kitten_grown' }, runs).map((r) => r.id),
		['run-2']
	);
});

test('fleetRoutines: the copies of each account joined by name, triggers by event', () => {
	const pesca = accountRows(
		{
			recurring: [
				rc({ id: 'rtn_0001aaaa', reason: 'clock_waiting', nextAt: at(14, 32) }),
				rc({ id: 'rtn_0002bbbb', name: 'Farm run', clock: 'HERB', reason: 'off', enabled: false })
			]
		},
		{
			triggers: [
				{ event: 'kitten_grown', enabled: true, run: { className: 'a.fm.catswap.CatSwapPlugin' } }
			],
			watching: true
		},
		[],
		now
	);
	const iron = accountRows(
		{
			recurring: [
				rc({ id: 'rtn_0009ffff', reason: 'clock_ready', due: true }),
				rc({ id: 'rtn_0008eeee', name: 'Alt run', reason: 'held', priority: 10 })
			]
		},
		null,
		[],
		now
	);
	const all = fleetRoutines([
		{ account: 'acc_pesca', rows: pesca },
		{ account: 'acc_iron', rows: iron }
	]);
	// Switched on first, recurring before triggers, then by name; off at the end.
	assert.deepEqual(
		all.map((f) => [f.key, f.members.map((m) => m.account), f.on]),
		[
			['recurring:Alt run', ['acc_iron'], 1],
			['recurring:Bird houses', ['acc_pesca', 'acc_iron'], 2],
			['trigger:kitten_grown', ['acc_pesca'], 1],
			['recurring:Farm run', ['acc_pesca'], 0]
		]
	);
	const bird = all[1];
	assert.equal(bird.differs, false);
	assert.equal(bird.line, 'quando Bird Houses fica pronto · Birdhouse Runner');
	// The headline: the copy that is due beats the one that waits; a held one beats both.
	assert.equal(headline(bird)?.account, 'acc_iron');
	assert.equal(headline(all[0])?.row.when, 'segura');
	// A different priority in one account: the copies differ.
	const other = accountRows(
		{ recurring: [rc({ id: 'rtn_0007dddd', priority: 90 })] },
		null,
		[],
		now
	);
	const mixed = fleetRoutines([
		{ account: 'acc_pesca', rows: pesca },
		{ account: 'acc_alt', rows: other }
	]).find((f) => f.key === 'recurring:Bird houses');
	assert.equal(mixed?.differs, true);
	// The head of an account's queue: the first that is due or has a time.
	assert.equal(queueHead(iron)?.name, 'Bird houses');
	assert.equal(queueHead(pesca)?.when, 'em 32 min');
	assert.equal(queueHead([]), null);
});

test('newDraft: never enabled; draftProblem says what is missing', () => {
	const d = newDraft([]);
	assert.equal('enabled' in d, false);
	assert.equal(draftProblem(d), 'Escolha ao menos uma conta.');
	d.accounts = ['acc_a'];
	assert.match(draftProblem(d) ?? '', /nome/);
	d.name = 'Bird houses';
	assert.match(draftProblem(d) ?? '', /plugin/);
	d.className = BIRD;
	assert.equal(draftProblem(d), null);
	assert.match(draftProblem({ ...d, interval: 5 }) ?? '', /intervalo/);
	assert.match(draftProblem({ ...d, kind: 'interval', interval: null }) ?? '', /quanto em quanto/);
	assert.match(draftProblem({ ...d, priority: 101 }) ?? '', /prioridade/);
	assert.match(draftProblem({ ...d, timeout: 500 }) ?? '', /limite/);
	// A run that waits for an objective may take up to 12 h.
	assert.equal(draftProblem({ ...d, timeout: 500, until: 'skill:fishing>=76' }), null);
	assert.match(draftProblem({ ...d, timeout: 721, until: 'skill:fishing>=76' }) ?? '', /720/);
	// A trigger has no name of its own.
	assert.equal(draftProblem({ ...d, kind: 'event', name: '' }), null);
});

test('a routine that waits for an objective keeps it through the form (the server writes the account)', () => {
	const d = {
		...newDraft(['acc_a']),
		name: 'Pescar até 76',
		kind: 'interval' as const,
		interval: 60,
		className: FISH,
		timeout: 360
	};
	assert.equal('until' in recurringBody(d, 'acc_a').run, false);
	const stored = recurringBody({ ...d, until: 'skill:fishing>=76' }, 'acc_a');
	assert.deepEqual(stored.run, {
		className: FISH,
		timeoutS: 21600,
		until: { objective: 'skill:fishing>=76' }
	});
	const rows = accountRows(
		{
			recurring: [
				rc({
					id: 'rtn_0001aaaa',
					run: { ...stored.run, until: { objective: 'skill:fishing>=76', account: 'acc_a' } }
				})
			]
		},
		null,
		[],
		now
	);
	const back = draftOf(fleetRoutines([{ account: 'acc_a', rows }])[0]);
	assert.equal(back.until, 'skill:fishing>=76');
	assert.deepEqual(recurringBody(back, 'acc_a').run.until, { objective: 'skill:fishing>=76' });
});

test('recurringBody and triggerBody: what the server accepts, off unless the copy is on', () => {
	const d = {
		...newDraft(['acc_a']),
		name: '  Bird houses ',
		className: BIRD,
		stop: [FISH, BIRD],
		back: false,
		timeout: 10,
		interval: 50
	};
	assert.deepEqual(recurringBody(d, 'acc_a'), {
		name: 'Bird houses',
		enabled: false,
		priority: 50,
		clock: 'BIRD_HOUSE',
		intervalMin: 50,
		run: { className: BIRD, stop: [FISH], timeoutS: 600, stay: true }
	});
	assert.deepEqual(
		recurringBody(
			{ ...d, kind: 'interval', ids: { acc_a: 'rtn_0001aaaa' }, back: true, timeout: null },
			'acc_a',
			true
		),
		{
			id: 'rtn_0001aaaa',
			name: 'Bird houses',
			enabled: true,
			priority: 50,
			intervalMin: 50,
			run: { className: BIRD, stop: [FISH] }
		}
	);
	assert.deepEqual(triggerBody({ ...d, kind: 'event' }), {
		event: 'kitten_grown',
		enabled: false,
		run: { className: BIRD, stop: [FISH], timeoutS: 600, stay: true }
	});
});

test('draftOf and withEnabled: a fleet routine back into the form and a switch', () => {
	const rows = (id: string, enabled: boolean) =>
		accountRows(
			{ recurring: [rc({ id, intervalMin: 50, enabled, reason: enabled ? undefined : 'off' })] },
			null,
			[],
			now
		);
	const [fr] = fleetRoutines([
		{ account: 'acc_a', rows: rows('rtn_0001aaaa', false) },
		{ account: 'acc_b', rows: rows('rtn_0002bbbb', true) }
	]);
	const d = draftOf(fr);
	assert.equal(d.kind, 'clock');
	assert.deepEqual(d.ids, { acc_a: 'rtn_0001aaaa', acc_b: 'rtn_0002bbbb' });
	assert.deepEqual(d.accounts, ['acc_a', 'acc_b']);
	assert.equal(d.interval, 50);
	assert.equal(d.back, true);
	const on = withEnabled({ ...rc(), lastRun: { state: 'done', startedAt: at(12) } }, true);
	assert.equal(on.enabled, true);
	assert.equal('updatedAt' in on || 'lastRun' in on, false);
});

test('savePlan: each account keeps its switch, new copies are off, unticked ones go', () => {
	const pesca = accountRows(
		{ recurring: [rc({ id: 'rtn_0001aaaa', reason: 'held' })] },
		null,
		[],
		now
	);
	const iron = accountRows(
		{ recurring: [rc({ id: 'rtn_0002bbbb', reason: 'off', enabled: false })] },
		null,
		[],
		now
	);
	const [fr] = fleetRoutines([
		{ account: 'acc_pesca', rows: pesca },
		{ account: 'acc_iron', rows: iron }
	]);
	const d = { ...draftOf(fr), accounts: ['acc_pesca', 'acc_alt'], priority: 70 };
	const plan = savePlan(d, fr, new Set());
	assert.deepEqual(
		plan.puts.map((p) => [p.account, p.recurring?.id, p.recurring?.enabled, p.recurring?.priority]),
		[
			['acc_pesca', 'rtn_0001aaaa', true, 70],
			['acc_alt', undefined, false, 70]
		]
	);
	assert.deepEqual(plan.removes, [{ account: 'acc_iron', id: 'rtn_0002bbbb' }]);
	// Saving the held copy (switched on) lets it start at once.
	assert.deepEqual(plan.releases, ['acc_pesca']);
	// An account whose last run holds every routine: any save there releases them.
	assert.deepEqual(savePlan({ ...d, accounts: ['acc_alt'] }, fr, new Set(['acc_alt'])).releases, [
		'acc_alt'
	]);
	// A new routine: puts only, off everywhere.
	const fresh = savePlan(
		{ ...newDraft(['acc_a', 'acc_b']), name: 'Herbs', clock: 'HERB', className: BIRD },
		null,
		new Set()
	);
	assert.deepEqual(
		fresh.puts.map((p) => [p.account, p.recurring?.enabled, p.recurring?.id]),
		[
			['acc_a', false, undefined],
			['acc_b', false, undefined]
		]
	);
	// A trigger: POST /triggers per account, never removed.
	const cat = { ...newDraft(['acc_a']), kind: 'event' as const, className: 'a.CatSwapPlugin' };
	assert.deepEqual(savePlan(cat, null, new Set(['acc_a'])), {
		puts: [
			{
				account: 'acc_a',
				trigger: { event: 'kitten_grown', enabled: false, run: { className: 'a.CatSwapPlugin' } }
			}
		],
		removes: [],
		releases: []
	});
});

test('fleetRuns: the runs of every copy, newest first', () => {
	const rows = (id: string) => accountRows({ recurring: [rc({ id })] }, null, [], now);
	const [fr] = fleetRoutines([
		{ account: 'acc_a', rows: rows('rtn_0001aaaa') },
		{ account: 'acc_b', rows: rows('rtn_0002bbbb') }
	]);
	const runs: Record<string, Run[]> = {
		acc_a: [run({ id: 'a2', routine: 'rtn_0001aaaa', startedAt: at(11) }), run({ id: 'a1' })],
		acc_b: [run({ id: 'b1', routine: 'rtn_0002bbbb', startedAt: at(13) })]
	};
	assert.deepEqual(
		fleetRuns(fr, (a) => runs[a] ?? []).map((x) => [x.account, x.run.id]),
		[
			['acc_b', 'b1'],
			['acc_a', 'a2']
		]
	);
});

test('doSteps: stop, run, walk back and restart, as the artboard lists them', () => {
	const d = { ...newDraft(['acc_a']), className: BIRD, stop: [FISH] };
	assert.deepEqual(doSteps(d), [
		'Para Tick Fisher num ponto seguro',
		'Roda Birdhouse Runner até terminar',
		'Volta ao lugar de antes',
		'Religa Tick Fisher'
	]);
	assert.deepEqual(doSteps({ ...d, stop: [], back: false }), [
		'Espera um ponto seguro',
		'Roda Birdhouse Runner até terminar'
	]);
});

test('runLine: when, what happened and the result', () => {
	const done = runLine(run({ result: { deathRunesGained: 100 } }), now);
	assert.deepEqual(done, {
		at: '12:00',
		text: '6 min · +100 death runes',
		result: 'feito',
		tone: 'ok'
	});
	const failed = runLine(run({ state: 'failed', error: 'sem sementes' }), now);
	assert.equal(failed.tone, 'bad');
	assert.equal(failed.text, '6 min · sem sementes');
	const going = runLine(
		run({
			state: 'running',
			finishedAt: undefined,
			events: [{ at: at(13, 58), state: 'running', text: 'casa 3 de 4' }]
		}),
		now
	);
	assert.deepEqual(going, { at: '12:00', text: 'casa 3 de 4', result: 'rodando', tone: 'go' });
});

test('accountQueue: now, ready, by the clock with the break, events, held', () => {
	const rows = accountRows(
		{
			recurring: [
				rc({ id: 'rtn_0001aaaa', reason: 'clock_waiting', nextAt: at(14, 32) }),
				rc({
					id: 'rtn_0002bbbb',
					name: 'Farm run',
					clock: 'HERB',
					reason: 'clock_waiting',
					nextAt: at(15)
				}),
				rc({
					id: 'rtn_0003cccc',
					name: 'Herbs',
					clock: undefined,
					intervalMin: 60,
					reason: 'interval',
					due: true
				}),
				rc({ id: 'rtn_0004dddd', name: 'Alt run', reason: 'held' }),
				rc({ id: 'rtn_0005eeee', name: 'Off', reason: 'off', enabled: false })
			]
		},
		{
			triggers: [
				{ event: 'kitten_grown', enabled: true, run: { className: 'a.fm.catswap.CatSwapPlugin' } }
			],
			watching: true
		},
		[],
		now
	);
	const runs = [
		run({ id: 'r2', routine: 'rtn_0001aaaa', startedAt: at(10), finishedAt: at(10, 4) })
	];
	const q = accountQueue(rows, runs, { active: false, in_s: 51 * 60 }, now);
	assert.deepEqual(
		q.map((i) => [i.at, i.title, i.sub]),
		[
			['a seguir', 'Herbs', 'rotina · pelo intervalo'],
			['14:32', 'Bird houses', 'rotina · pelo relógio · ~4 min'],
			['14:51', 'Pausa', 'Break Handler'],
			['15:00', 'Farm run', 'rotina · pelo relógio'],
			['no evento', 'Trocar o gato', 'quando o filhote crescer · Cat Swap'],
			['segura', 'Alt run', 'falhou: espera você salvá-la de novo']
		]
	);
	assert.equal(q[1].routine, 'recurring:Bird houses');
	// A run going on, and a break going on: the run is the current one.
	const going = accountQueue(
		rows.filter((r) => r.id === 'rtn_0001aaaa'),
		[run({ id: 'r3', routine: 'rtn_0001aaaa', state: 'running', finishedAt: undefined })],
		{ active: true, left_s: 300 },
		now
	);
	assert.deepEqual(
		going.slice(0, 2).map((i) => [i.at, i.title, i.sub, i.cur]),
		[
			['agora', 'Bird houses', 'Birdhouse Runner · rodando', true],
			['agora', 'Pausa', 'Break Handler · volta em 5 min', false]
		]
	);
	assert.deepEqual(accountQueue([], [], undefined, now), []);
});
