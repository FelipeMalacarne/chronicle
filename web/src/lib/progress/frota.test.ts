import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
	compactGroups,
	fleetSummary,
	lane,
	lanes,
	needs,
	nextUp,
	parseDensity,
	REFUSED,
	scriptsLine,
	sessionCommand,
	type FleetInput
} from './frota.ts';
import type { AccountState, ProgressSummary, TimerView } from './types.ts';
import type { WorkerLine } from './workers.ts';

const now = Date.parse('2026-11-12T18:00:00Z');
const sec = now / 1000;

function input(p: Partial<FleetInput> = {}): FleetInput {
	return {
		account: {
			id: 'acc_a',
			rsn: 'Iron A',
			mode: 'ironman',
			guide_id: 'bruhsailer-main',
			layer: 'enhanced'
		},
		state: {
			account: {
				id: 'acc_a',
				rsn: 'Iron A',
				mode: 'ironman',
				guide_id: 'bruhsailer-main',
				layer: 'enhanced'
			},
			total_level: 919,
			skills: [
				{ skill: 'fishing', level: 75, xp: 1_300_000 },
				{ skill: 'firemaking', level: 99, xp: 13_100_000 }
			]
		} as unknown as AccountState,
		summary: null,
		timers: [],
		worker: null,
		client: null,
		scripts: [],
		...p
	};
}

function summary(pluginMinAgo: number): ProgressSummary {
	return {
		account: 'acc_a',
		active_guide: 'bruhsailer-main',
		routes: [
			{
				guide: 'bruhsailer-main',
				title: 'BRUHsailer',
				active: true,
				cursor_ref: 'CH1.34',
				counts: { total: 226, done: 34 }
			}
		],
		sources: [
			{
				source: 'plugin',
				last_observed_at: new Date(now - pluginMinAgo * 60_000).toISOString(),
				facts: 1
			}
		]
	} as unknown as ProgressSummary;
}

function worker(p: Partial<WorkerLine>): WorkerLine {
	return {
		account: 'acc_a',
		rsn: 'Iron A',
		where: 'saradomin · pod abc',
		host: 'saradomin',
		runtime: 'pod · abc',
		state: 'parado',
		tone: 'idle',
		session: 'x',
		sessionTone: 'ok',
		sessionWhen: 'push há 60 min',
		actions: ['start'],
		cockpit: false,
		outdated: false,
		...p
	};
}

test('an account nobody runs is only followed, with its face and route', () => {
	const l = lane(input({ summary: summary(600) }), now);
	assert.equal(l.tone, 'off');
	assert.equal(l.activity, 'Só acompanhada');
	assert.equal(l.detail, 'plugin há 10 h 00');
	assert.deepEqual(l.face, { skill: 'firemaking', level: 99 });
	assert.equal(l.meta, 'ironman · 919 total');
	assert.deepEqual(l.route, { label: 'BRUHsailer', ref: 'CH1.34', done: 34, total: 226, pct: 15 });
});

test('a fresh plugin reading without an agent is the owner playing by hand', () => {
	const l = lane(input({ summary: summary(1) }), now);
	assert.equal(l.tone, 'ok');
	assert.equal(l.activity, 'No jogo');
	assert.match(l.detail ?? '', /à mão/);
});

test('a logged-in client shows its scripts and world', () => {
	const l = lane(
		input({
			worker: worker({
				tone: 'ok',
				state: 'na tela de login ou no jogo · agent ligado',
				cockpit: true
			}),
			client: {
				running: true,
				headless: true,
				vnc: false,
				state: { loggedIn: true },
				login: { currentWorld: 370 }
			},
			scripts: ['Tick Fisher']
		}),
		now
	);
	assert.equal(l.tone, 'ok');
	assert.equal(l.activity, 'Tick Fisher');
	assert.equal(l.detail, 'mundo 370');
	assert.equal(l.where, 'saradomin');
	assert.equal(l.client, true);
});

test('a break of the Break Handler rests the lane: still playing, back in a while', () => {
	const l = lane(
		input({
			worker: worker({ tone: 'ok', cockpit: true }),
			client: {
				running: true,
				headless: true,
				vnc: false,
				state: { loggedIn: false },
				break: { active: true, left_s: 360 }
			},
			scripts: ['Break Handler', 'Yew Chopper']
		}),
		now
	);
	assert.equal(l.tone, 'ok');
	assert.equal(l.rest, true);
	assert.equal(l.activity, 'Em pausa');
	assert.equal(l.detail, 'Break Handler · volta para Yew Chopper em 6 min');
	// The V2 handler does not say how long.
	const v2 = lane(
		input({
			worker: worker({ tone: 'ok', cockpit: true }),
			client: { running: true, headless: true, vnc: false, break: { active: true } }
		}),
		now
	);
	assert.equal(v2.detail, 'Break Handler');
	// Not in a break: the next one does not change the lane.
	const playing = lane(
		input({
			worker: worker({ tone: 'ok', cockpit: true }),
			client: {
				running: true,
				headless: true,
				vnc: false,
				state: { loggedIn: true },
				break: { active: false, in_s: 1800 }
			}
		}),
		now
	);
	assert.equal(playing.rest, false);
	assert.equal(playing.activity, 'No jogo');
});

test('the rhythm: the skill being trained is the face, shown only while playing', () => {
	const rhythm = {
		account: 'acc_a',
		training: {
			skill: 'fishing',
			level: 76,
			xp: 1_425_500,
			xp_per_hour: 38_200,
			next_level_in_s: 1320,
			last_gain_at: '2026-11-12T17:59:00Z'
		},
		session: {
			started_at: '2026-11-12T17:00:00Z',
			xp: 26_700,
			gained: [{ item: 383, name: 'Raw shark', qty: 412 }],
			running_out: null
		},
		today: { since: '2026-11-12T03:00:00Z', xp: 96_400, levels: 2, skills: [] },
		gp: null
	};
	const l = lane(input({ summary: summary(1), rhythm }), now);
	assert.equal(l.tone, 'ok');
	assert.deepEqual(l.face, { skill: 'fishing', level: 76 });
	assert.equal(l.rhythm?.rate, '38,2k xp/h');
	assert.equal(l.rhythm?.bar?.next, '77 em 22 min');
	assert.deepEqual(
		l.rhythm?.gained.map((g) => g.qty),
		['412']
	);
	// Stopped, with only the last session: no rhythm, the highest skill is the face again.
	const stopped = lane(input({ rhythm: { ...rhythm, training: null } }), now);
	assert.equal(stopped.tone, 'off');
	assert.equal(stopped.rhythm, null);
	assert.deepEqual(stopped.face, { skill: 'firemaking', level: 99 });
	// Before the first reading: nothing.
	assert.equal(lane(input({ summary: summary(1) }), now).rhythm, null);
});

test('a client that is up but not in the game sits at the login screen', () => {
	const l = lane(
		input({
			worker: worker({ tone: 'ok', cockpit: true }),
			client: { running: true, headless: true, vnc: false }
		}),
		now
	);
	assert.equal(l.tone, 'up');
	assert.equal(l.activity, 'Na tela de login');
});

test('a failing worker is a problem and asks for the owner', () => {
	const l = lane(
		input({
			worker: worker({ tone: 'warn', state: 'esperando espaço no nó', detail: 'Insufficient cpu' })
		}),
		now
	);
	assert.equal(l.tone, 'bad');
	const n = needs([l]);
	assert.equal(n.length, 1);
	assert.equal(n[0].hard, true);
	assert.equal(n[0].text, 'esperando espaço no nó');
});

test('needs: no session, old version and ready clocks', () => {
	const timers: TimerView[] = [
		{
			key: 'special',
			label: 'Special patches',
			state: 'COMPLETED',
			source: 'plugin',
			observed_at: ''
		}
	];
	const l = lane(
		input({
			timers,
			worker: worker({ sessionWhen: undefined, blocked: 'conecte', outdated: true })
		}),
		now
	);
	assert.equal(l.detail, 'sem sessão do jogo');
	const n = needs([l]);
	assert.deepEqual(
		n.map((x) => [x.text, x.hard]),
		[
			['não tem sessão do jogo', true],
			['roda numa versão antiga', false],
			['tem Special patches prontos', false]
		]
	);
	// No command in the paragraph: the guidance says where, the button copies the command.
	assert.equal(n[0].sub, 'Abra o launcher no zaros e envie a sessão.');
	assert.equal(n[0].command, `chronicle accounts connect --character "${l.rsn}"`);
});

test('nextUp: ready first, then the soonest; empty and unknown are left out', () => {
	const t = (key: string, state: TimerView['state'], ready_at?: number): TimerView => ({
		key,
		label: key,
		state,
		ready_at,
		source: 'plugin',
		observed_at: ''
	});
	assert.deepEqual(
		nextUp(
			[
				t('herb', 'IN_PROGRESS', sec + 2400),
				t('tree', 'EMPTY'),
				t('bird', 'IN_PROGRESS', sec + 720),
				t('special', 'IN_PROGRESS', sec - 10),
				t('contract', 'UNKNOWN')
			],
			now
		),
		[
			{ label: 'special', when: 'pronto', ready: true },
			{ label: 'bird', when: '12 min', ready: false }
		]
	);
});

test('lanes: problems first; the summary counts by state', () => {
	const all = lanes(
		[
			input({ summary: summary(1) }),
			input({
				account: { id: 'acc_b', rsn: 'Alt B', mode: 'ironman', guide_id: '', layer: 'classic' },
				state: null,
				worker: worker({ tone: 'warn', state: 'falhando' })
			})
		],
		now
	);
	assert.deepEqual(
		all.map((l) => l.id),
		['acc_b', 'acc_a']
	);
	assert.equal(fleetSummary(all), '2 contas · 1 no jogo · 1 com problema');
});

test('scriptsLine leaves the always-on plugins out', () => {
	assert.equal(
		scriptsLine(['Break Handler V2', 'Tick Fisher', 'Auto Login', 'Chronicle Link']),
		'Tick Fisher'
	);
	assert.equal(scriptsLine(['Tick Fisher', 'AIO Fighter']), 'Tick Fisher +1');
	assert.equal(scriptsLine(['Break Handler V2']), '');
});

const refused = { reason: 'Invalid credentials', when: 'há 3 h', character: 'Iron A' };

test('a refused login stops the account and asks for a new session, with the command', () => {
	const l = lane(input({ worker: worker({ refused }) }), now);
	assert.equal(l.tone, 'bad');
	assert.equal(l.activity, REFUSED);
	assert.equal(l.detail, 'o jogo recusou o login há 3 h · Invalid credentials');
	assert.deepEqual(needs([l]), [
		{
			account: 'acc_a',
			rsn: 'Iron A',
			kind: 'session',
			text: 'não entra: o jogo recusou o login',
			sub: 'Abra o launcher no zaros e envie a sessão de novo.',
			hard: true,
			command: 'chronicle accounts connect --character "Iron A"'
		}
	]);
});

test('sessionCommand: the refused character, the game name without a session, else nothing', () => {
	assert.equal(
		sessionCommand(
			lane(input({ worker: worker({ refused: { ...refused, character: 'Alt' } }) }), now)
		),
		'chronicle accounts connect --character "Alt"'
	);
	assert.equal(
		sessionCommand(lane(input({ worker: worker({ sessionWhen: undefined, blocked: 'x' }) }), now)),
		'chronicle accounts connect --character "Iron A"'
	);
	assert.equal(sessionCommand(lane(input({ worker: worker({}) }), now)), undefined);
	assert.equal(sessionCommand(lane(input(), now)), undefined);
});

test('a refused login is stale once the account is in the game', () => {
	const l = lane(
		input({
			worker: worker({ refused, tone: 'ok', cockpit: true }),
			client: { running: true, headless: true, vnc: false, state: { loggedIn: true } }
		}),
		now
	);
	assert.equal(l.tone, 'ok');
	assert.deepEqual(needs([l]), []);
});

test('a worker the quota has no room for asks softly, with what is left', () => {
	const l = lane(
		input({ worker: worker({ tone: 'warn', state: 'não cabe (quota)', noRoom: true }) }),
		now
	);
	assert.equal(l.tone, 'bad');
	const n = needs([l], {
		fits: 0,
		limited_by: 'requests.cpu',
		cpu_hard_cores: 2.2,
		cpu_free_cores: 0.3,
		worker_cpu_cores: 0.7,
		worker_memory_bytes: 2 ** 30,
		seen: ''
	});
	assert.deepEqual(
		n.map((x) => [x.text, x.hard, x.sub]),
		[
			[
				'não subiu: falta lugar',
				false,
				'O worker pede 0,7 núcleo e sobram 0,3. Pare outra conta ou deixe esta na espera.'
			]
		]
	);
});

test('the tag title says how the worker runs and what its pod uses', () => {
	const l = lane(
		input({
			worker: worker({
				usage: { cpu_cores: 0.404, memory_bytes: 877350912, measured_at: '' }
			})
		}),
		now
	);
	assert.equal(l.whereTitle, 'pod · abc · CPU 0,4 · RAM 837 MiB');
	assert.equal(lane(input(), now).whereTitle, undefined);
});

test('compactGroups: needs first, then playing and stopped; filtered by name', () => {
	const mk = (id: string, rsn: string, p: Partial<FleetInput>) =>
		input({
			account: { id, rsn, mode: 'ironman', guide_id: '', layer: 'classic' },
			...p
		});
	const all = lanes(
		[
			mk('acc_a', 'Felipe Pesca', { summary: summary(1) }),
			mk('acc_b', 'Felipe Alt', { worker: worker({ account: 'acc_b', refused }) }),
			mk('acc_c', 'Lenhador', {}),
			// Stopped, but without a game session: a hard need puts it with the problems.
			mk('acc_d', 'Felipe Horta', {
				worker: worker({ account: 'acc_d', sessionWhen: undefined, blocked: 'conecte' })
			})
		],
		now
	);
	const groups = compactGroups(all, needs(all));
	assert.deepEqual(
		groups.map((g) => [g.label, g.lanes.map((l) => l.rsn)]),
		[
			['Precisa de você', ['Felipe Alt', 'Felipe Horta']],
			['Jogando', ['Felipe Pesca']],
			['Paradas', ['Lenhador']]
		]
	);
	assert.deepEqual(
		compactGroups(all, needs(all), '  LENHA ').map((g) => [g.key, g.lanes.length]),
		[['stopped', 1]]
	);
	assert.deepEqual(compactGroups(all, needs(all), 'ninguém'), []);
});

test('the density remembered falls back to the lanes', () => {
	assert.equal(parseDensity('compacta'), 'compacta');
	assert.equal(parseDensity('raias'), 'raias');
	assert.equal(parseDensity(null), 'raias');
	assert.equal(parseDensity('lista'), 'raias');
});

test('scriptsLine cleans the client HTML out of the names', () => {
	assert.equal(
		scriptsLine([
			'<html>[<font color=#b8f704M>M</font>] Break Handler',
			'<html>[<font color=#b8f704M>M</font>] Tick Fisher'
		]),
		'Tick Fisher'
	);
});
