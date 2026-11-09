import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	chatLines,
	scriptBusy,
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
			['npc', 'Guard', 5, 'nível 21'],
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

test('scriptBusy: a recent heartbeat with the scripts running', () => {
	const busy: GameView = {
		observe: {
			scriptsPaused: false,
			scripts: [{ scriptClassName: 'a.B', status: 'OK', stalledMs: 300 }]
		}
	};
	assert.equal(scriptBusy(busy), true);
	assert.equal(scriptBusy({ observe: { ...busy.observe, scriptsPaused: true } }), false);
	assert.equal(
		scriptBusy({ observe: { scripts: [{ scriptClassName: 'a.B', status: 'NO_HEARTBEAT' }] } }),
		false
	);
	assert.equal(
		scriptBusy({ observe: { scripts: [{ scriptClassName: 'a.B', stalledMs: 60000 }] } }),
		false
	);
});
