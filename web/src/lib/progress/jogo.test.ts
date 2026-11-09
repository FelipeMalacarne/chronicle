import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	chatLines,
	loopScripts,
	itemMenu,
	bankMenu,
	nearMenu,
	qtyLabel,
	dialogueLine,
	equipItems,
	invSlots,
	machineRows,
	nearRows,
	skillCells,
	tiles
} from './jogo.ts';
import type { GameView } from '../api/microbot.ts';

const view: GameView = {
	observe: {
		loggedIn: true,
		player: { name: 'EventSourced', position: { x: 3222, y: 3218, plane: 0 } },
		equipment: [
			{ slot: 'WEAPON', id: 1351, name: 'Bronze axe', quantity: 1 },
			{ slot: 'GLOVES', id: 1059, name: 'Leather gloves', quantity: 1 }
		],
		dialogue: {
			inDialogue: true,
			hasOptions: true,
			question: 'What would you like?',
			options: ['Buy', 'Goodbye']
		},
		npcs: [
			{
				id: 3010,
				name: 'Guard',
				combatLevel: 21,
				distance: 5,
				position: { x: 3215, y: 3219, plane: 0 }
			}
		],
		objects: [
			{ id: 10820, name: 'Oak tree', position: { x: 3220, y: 3218, plane: 0 } },
			{ id: 1, name: 'null', position: { x: 3222, y: 3218, plane: 0 } }
		],
		chat: {
			cursor: 3,
			messages: [
				{ type: 'GAMEMESSAGE', message: 'You swing your axe.', seq: 2, at: 0 },
				{ type: 'GAMEMESSAGE', message: 'You get some logs.', seq: 3, at: 0 }
			]
		}
	},
	inventory: {
		count: 2,
		capacity: 28,
		items: [
			{ id: 1511, name: 'Logs', quantity: 1, slot: 3, actions: ['Drop'] },
			{ id: 995, name: 'Coins', quantity: 1200, slot: 0, actions: [] }
		]
	},
	skills: {
		skills: [
			{ name: 'Woodcutting', level: 15, boostedLevel: 15, xp: 2411 },
			{ name: 'Attack', level: 10, boostedLevel: 12, xp: 1200 }
		]
	},
	ground: {
		items: [{ id: 526, name: 'Bones', quantity: 2, position: { x: 3230, y: 3218, plane: 0 } }]
	},
	machines: {
		count: 1,
		scripts: [
			{ name: 'Birdhouses', currentState: 'WALKING', msInCurrentState: 48000, loopCount: 40 }
		]
	}
};

test('invSlots places items in their slots', () => {
	const s = invSlots(view);
	assert.equal(s.length, 28);
	assert.equal(s[0]?.name, 'Coins');
	assert.equal(s[3]?.name, 'Logs');
	assert.equal(s[1], null);
	assert.equal(
		invSlots(null).every((x) => x === null),
		true
	);
});

test('equipItems maps the RuneLite slots to the paper doll', () => {
	assert.deepEqual(
		equipItems(view).map((e) => e.slot),
		['weapon', 'hands']
	);
});

test('skillCells follows the game order', () => {
	assert.deepEqual(
		skillCells(view).map((s) => [s.skill, s.boosted]),
		[
			['attack', 12],
			['woodcutting', 15]
		]
	);
});

test('nearRows sorts by distance and drops nameless objects', () => {
	assert.deepEqual(
		nearRows(view).map((r) => [r.kind, r.name, r.distance, r.detail]),
		[
			['objeto', 'Oak tree', 2, ''],
			['npc', 'Guard', 7, 'nível 21'],
			['chão', 'Bones', 8, '×2']
		]
	);
	assert.equal(tiles({ x: 1, y: 1, plane: 0 }, { x: 1, y: 1, plane: 1 }), null);
});

test('machines, chat and dialogue', () => {
	assert.deepEqual(machineRows(view), [
		{ name: 'Birdhouses', state: 'WALKING', detail: 'há 48 s · 40 loops' }
	]);
	assert.deepEqual(
		chatLines(view).map((c) => c.text),
		['You swing your axe.', 'You get some logs.']
	);
	assert.deepEqual(dialogueLine(view), {
		text: 'What would you like?',
		options: ['Buy', 'Goodbye'],
		more: false
	});
	assert.equal(dialogueLine({ observe: { dialogue: { inDialogue: false } } }), null);
});

test('loopScripts: recent heartbeats while the scripts run', () => {
	const busy: GameView = {
		observe: {
			scriptsPaused: false,
			scripts: [{ scriptClassName: 'a.B', status: 'OK', stalledMs: 300 }]
		}
	};
	assert.deepEqual(loopScripts(busy), ['B']);
	assert.deepEqual(loopScripts({ observe: { ...busy.observe, scriptsPaused: true } }), []);
	assert.deepEqual(
		loopScripts({ observe: { scripts: [{ scriptClassName: 'a.B', status: 'NO_HEARTBEAT' }] } }),
		[]
	);
	assert.deepEqual(
		loopScripts({ observe: { scripts: [{ scriptClassName: 'a.B', stalledMs: 60000 }] } }),
		[]
	);
});

test('NPC distance in tiles from the position, not local units', () => {
	const v: GameView = {
		observe: {
			player: { position: { x: 1678, y: 2966, plane: 0 } },
			npcs: [{ id: 1, name: 'Snake', distance: 286, position: { x: 1680, y: 2966, plane: 0 } }]
		}
	};
	assert.equal(nearRows(v)[0].distance, 2);
});

test('menus carry only what the game and the agent can do', () => {
	const lobster = {
		id: 379,
		name: 'Lobster',
		quantity: 1,
		slot: 1,
		actions: ['Eat', 'Use', 'Drop']
	};
	assert.deepEqual(
		itemMenu(lobster, false).map((o) => [o.label, o.command, !!o.confirm]),
		[
			['Eat', 'inventory_use', false],
			['Use', 'inventory_use', false],
			['Drop', 'inventory_drop', true]
		]
	);
	assert.deepEqual(itemMenu({ ...lobster, actions: [] }, false), []);
	assert.deepEqual(
		itemMenu(lobster, true).map((o) => [o.label, o.command, !!o.confirm]),
		[
			['Deposit-1', 'inventory_use', false],
			['Deposit-5', 'inventory_use', false],
			['Deposit-10', 'inventory_use', false],
			['Deposit-All', 'inventory_use', true]
		]
	);

	assert.deepEqual(
		bankMenu(lobster).map((o) => [o.label, o.body]),
		[
			['Withdraw-1', { name: 'Lobster', quantity: 1 }],
			['Withdraw-5', { name: 'Lobster', quantity: 5 }],
			['Withdraw-10', { name: 'Lobster', quantity: 10 }],
			['Withdraw-All', { name: 'Lobster', all: true }]
		]
	);
	const rows = nearRows(view);
	assert.deepEqual(
		nearMenu(rows.find((r) => r.kind === 'chão')!).map((o) => o.label),
		['Take', 'Walk here']
	);
	const npc = nearMenu(rows.find((r) => r.kind === 'npc')!);
	assert.deepEqual(
		npc.map((o) => [o.label, !!o.ask]),
		[
			['Ação…', true],
			['Walk here', false]
		]
	);
	assert.deepEqual(qtyLabel(1200), { text: '1200', tone: 'y' });
	assert.deepEqual(qtyLabel(613_400), { text: '613K', tone: 'w' });
	assert.deepEqual(qtyLabel(12_500_000), { text: '12M', tone: 'g' });
});
