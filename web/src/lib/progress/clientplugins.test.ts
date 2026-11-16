import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	cardStatus,
	clientCard,
	findInstalled,
	lastResult,
	snapshotMachines,
	type CardInput
} from './clientplugins.ts';
import type { PluginsView } from '../api/microbot.ts';

const FM = 'net.runelite.client.plugins.microbot.fm.';
const view: PluginsView = {
	scripts: {
		count: 6,
		scripts: [
			{
				name: '[M] Tick Fisher',
				className: `${FM}tickfisher.TickFisherPlugin`,
				active: true,
				enabled: true
			},
			{
				name: 'Yew Chopper',
				className: 'com.hub.yewchopper.YewChopperPlugin',
				active: false,
				enabled: true
			},
			{ name: 'Cat Swap', className: `${FM}catswap.CatSwapPlugin`, active: false, enabled: true },
			{
				name: 'Break Handler',
				className: 'net.runelite.client.plugins.microbot.breakhandler.BreakHandlerPlugin',
				active: false,
				enabled: true
			},
			{
				name: 'Chronicle Link',
				className: `${FM}chroniclelink.ChronicleLinkPlugin`,
				active: true,
				enabled: true
			},
			{
				name: 'Example',
				className: 'net.runelite.client.plugins.microbot.example.ExamplePlugin',
				active: false,
				enabled: true
			}
		]
	},
	installed: {
		count: 1,
		total: 1,
		plugins: [
			{
				internalName: 'YewChopperPlugin',
				name: 'Yew Chopper',
				installedVersion: '1.4.0',
				loaded: true
			}
		]
	}
};

const base = (over: Partial<CardInput> = {}): CardInput => ({
	view,
	game: {
		observe: {
			scripts: [{ scriptClassName: `${FM}tickfisher.TickFisherScript`, loopCount: 1820 }]
		},
		machines: { scripts: [{ name: 'Tick Fisher', currentState: 'FISHING' }] }
	} as unknown as CardInput['game'],
	paused: false,
	profile: [
		{
			source: 'core',
			name: 'net.runelite.client.plugins.microbot.breakhandler.BreakHandlerPlugin',
			start: true
		},
		{
			source: 'hub',
			name: 'YewChopperPlugin',
			class_name: 'com.hub.yewchopper.YewChopperPlugin',
			version: '1.4.2',
			start: false
		}
	],
	triggers: [
		{ event: 'kitten_grown', enabled: true, run: { className: `${FM}catswap.CatSwapPlugin` } }
	],
	recurring: [],
	drift: null,
	groups: {},
	startedAt: { [`${FM}tickfisher.TickFisherPlugin`]: 0 },
	now: 42 * 60000,
	...over
});

test("the groups come from use: routine, profile start, the rest plays; the client's own hide", () => {
	const c = clientCard(base());
	assert.deepEqual(
		c.plays.map((r) => r.name),
		['Tick Fisher', 'Yew Chopper']
	);
	assert.deepEqual(
		c.routines.map((r) => r.name),
		['Cat Swap']
	);
	assert.deepEqual(c.always.map((r) => r.name).sort(), ['Break Handler', 'Chronicle Link']);
	assert.deepEqual(
		c.client.map((r) => r.name),
		['Example']
	);
});

test('the playing line has the state, the loops and since when; a routine says what wakes it', () => {
	const c = clientCard(base());
	assert.equal(c.plays[0].line, 'fishing · 1.820 voltas · 42 min');
	assert.equal(c.plays[0].lamp, 'ok');
	assert.equal(c.routines[0].line, 'parado · rotina quando o filhote crescer');
	assert.equal(c.always.find((r) => r.name === 'Chronicle Link')?.line, 'protegido · ligado');
});

test('a recurring routine shows its next time', () => {
	const at = Date.UTC(2026, 10, 16, 17, 32);
	const c = clientCard(
		base({
			triggers: [],
			recurring: [
				{
					name: 'Bird houses',
					enabled: true,
					priority: 0,
					due: false,
					nextAt: new Date(at).toISOString(),
					run: { className: `${FM}catswap.CatSwapPlugin` }
				}
			]
		})
	);
	const hhmm = new Date(at).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	assert.equal(c.routines[0].line, `parado · rotina Bird houses às ${hhmm}`);
});

test('paused and without heartbeat change the lamp and the line', () => {
	assert.equal(clientCard(base({ paused: true })).plays[0].lamp, 'rest');
	assert.match(clientCard(base({ paused: true })).plays[0].line, /^em pausa/);
	const stalled = clientCard(
		base({
			game: {
				observe: {
					scripts: [{ scriptClassName: `${FM}tickfisher.TickFisherScript`, status: 'NO_HEARTBEAT' }]
				}
			} as unknown as CardInput['game']
		})
	);
	assert.equal(stalled.plays[0].lamp, 'bad');
	assert.equal(stalled.plays[0].line, 'sem heartbeat');
	assert.deepEqual(cardStatus(stalled, false), { text: '1 sem resposta', tone: 'bad' });
});

test('the drift becomes badges on the row and a count in the title', () => {
	const c = clientCard(
		base({
			groups: { [`${FM}tickfisher.TickFisherPlugin`]: 'tickfisher' },
			drift: {
				account: 'acc_x',
				checked_at: '',
				settings: [
					{ group: 'tickfisher', key: 'method', profile: '2tick', client: '3tick' },
					{ group: 'tickfisher', key: 'stopAt', profile: '80', client: '85' },
					{ group: 'other', key: 'k', profile: 'a', client: null }
				],
				plugins: [
					{
						source: 'hub',
						name: 'YewChopperPlugin',
						class_name: 'com.hub.yewchopper.YewChopperPlugin',
						problem: 'version',
						version: '1.4.0'
					},
					{
						source: 'core',
						name: 'net.runelite.client.plugins.microbot.breakhandler.BreakHandlerPlugin',
						problem: 'not_started'
					},
					{ source: 'hub', name: 'FarmRunnerPlugin', problem: 'not_installed' }
				],
				unchecked: ['a', 'b']
			}
		})
	);
	assert.deepEqual(
		c.plays[0].badges.map((b) => b.text),
		['2 fora do perfil']
	);
	assert.equal(c.plays[0].drift.length, 2);
	assert.deepEqual(
		c.plays[1].badges.map((b) => b.text),
		['1.4.0 ≠ 1.4.2']
	);
	assert.deepEqual(
		c.always.find((r) => r.name === 'Break Handler')?.badges.map((b) => b.text),
		['devia estar ligado']
	);
	assert.equal(c.withDrift, 4);
	assert.equal(c.unchecked, 2);
	assert.deepEqual(cardStatus(c, false), { text: '4 com desvio', tone: 'stale' });
});

test('nothing playing, and the search over installed plugins', () => {
	const idle = clientCard(
		base({
			view: {
				...view,
				scripts: {
					...view.scripts,
					scripts: view.scripts.scripts.map((s) => ({ ...s, active: false }))
				}
			}
		})
	);
	assert.deepEqual(cardStatus(idle, false), { text: 'nada jogando' });
	const c = clientCard(base());
	assert.deepEqual(
		findInstalled(c, 'exa').map((r) => r.name),
		['Example']
	);
	assert.deepEqual(findInstalled(c, 'chronicle'), []);
	assert.deepEqual(findInstalled(c, ''), []);
});

test("a plugin's snapshot and its last result, for /plugins", () => {
	assert.deepEqual(
		snapshotMachines({
			scripts: [
				{ name: 'Tick Fisher', currentState: 'FISHING', msInCurrentState: 180000, loopCount: 1820 }
			]
		}),
		[{ name: 'Tick Fisher', state: 'FISHING', detail: 'há 3 min · 1.820 voltas' }]
	);
	assert.deepEqual(snapshotMachines(null), []);
	assert.deepEqual(lastResult({ results: [{ timestamp: 1, fish: 1512, ok: true }] }), [
		{ k: 'fish', v: '1.512' },
		{ k: 'ok', v: 'true' }
	]);
	assert.deepEqual(lastResult(undefined), []);
});
