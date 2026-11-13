import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	chatLines,
	loopScripts,
	itemMenu,
	bankMenu,
	nearMenu,
	qtyLabel,
	equipMenu,
	prayerCells,
	prayerMenu,
	spellCells,
	spellMenu,
	selectedItemMenu,
	selectedNearMenu,
	selectedLabel,
	dialogueLine,
	equipItems,
	invSlots,
	machineRows,
	nearRows,
	scriptRows,
	skillCells,
	tiles
} from './jogo.ts';
import type { GameView } from '../api/microbot.ts';

test('scriptRows lists the plugins that play, with the machine state and the loops', () => {
	const scripts = [
		{ name: 'Tick Fisher', className: 'net.x.fm.tickfisher.TickFisherPlugin', active: true },
		{ name: 'Break Handler', className: 'net.x.breakhandler.BreakHandlerPlugin', active: true },
		{ name: 'Blackjacker', className: 'net.x.fm.bj.BlackjackerPlugin', active: false }
	];
	const v: GameView = {
		observe: {
			scripts: [{ scriptClassName: 'net.x.fm.tickfisher.TickFisherScript', loopCount: 1820 }]
		},
		machines: { count: 1, scripts: [{ name: 'tick', currentState: 'FISHING' }] }
	};
	assert.deepEqual(scriptRows(scripts, v), [
		{
			name: 'Tick Fisher',
			className: 'net.x.fm.tickfisher.TickFisherPlugin',
			detail: 'fishing · 1.820 voltas'
		}
	]);
	const paused: GameView = {
		observe: {
			scriptsPaused: true,
			scripts: [{ scriptClassName: 'net.x.fm.tickfisher.TickFisherScript', status: 'NO_HEARTBEAT' }]
		}
	};
	assert.equal(scriptRows(scripts, paused)[0].detail, 'pausado · sem heartbeat');
	assert.deepEqual(scriptRows([], null), []);
});

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
			// Use sem a ponte deixaria o item selecionado no jogo sem alvo: fica de fora.
			['Eat', 'inventory_use', false],
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
			['Withdraw-X', { name: 'Lobster' }],
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
		npc.map((o) => [o.label, o.ask ?? '']),
		[
			['Ação…', 'action'],
			['Walk here', '']
		]
	);
	assert.deepEqual(qtyLabel(1200), { text: '1200', tone: 'y' });
	assert.deepEqual(qtyLabel(613_400), { text: '613K', tone: 'w' });
	assert.deepEqual(qtyLabel(12_500_000), { text: '12M', tone: 'g' });
});

test('with the bridge, menus come from the game', () => {
	const v: GameView = {
		observe: {
			player: { position: { x: 3222, y: 3218, plane: 0 } },
			npcs: [{ id: 9, name: 'Old', position: { x: 1, y: 1, plane: 0 } }]
		},
		bridge: {
			entities: {
				npcs: [
					{
						id: 3010,
						index: 77,
						name: 'Banker',
						actions: ['Talk-to', 'Bank', 'Collect'],
						position: { x: 3220, y: 3218, plane: 0 }
					}
				],
				objects: [
					{
						id: 10820,
						name: 'Oak tree',
						actions: ['Chop down'],
						position: { x: 3224, y: 3218, plane: 0 }
					}
				]
			},
			equipment: {
				items: [
					{
						slot: 'AMULET',
						id: 1712,
						name: 'Amulet of glory',
						quantity: 1,
						actions: ['Remove', 'Rub']
					}
				]
			},
			prayers: {
				level: 43,
				points: 31,
				prayers: [
					{ key: 'PROTECT_ITEM', name: 'Protect Item', level: 25, unlocked: true, active: false },
					{ key: 'EAGLE_EYE', name: 'Eagle Eye', level: 44, unlocked: false, active: false }
				]
			},
			spells: {
				spellbook: 'MODERN',
				spells: [
					{
						key: 'VARROCK_TELEPORT',
						name: 'Varrock Teleport',
						level: 25,
						castable: true,
						target: 'none',
						onItem: false
					},
					{
						key: 'LOW_LEVEL_ALCHEMY',
						name: 'Low Level Alchemy',
						level: 21,
						castable: true,
						target: 'item',
						onItem: true
					},
					{
						key: 'WIND_STRIKE',
						name: 'Wind Strike',
						level: 1,
						castable: true,
						target: 'npc',
						onItem: false
					},
					{
						key: 'ENCHANT_SAPPHIRE_JEWELLERY',
						name: 'Lvl-1 Enchant',
						level: 7,
						castable: true,
						target: 'unsupported',
						onItem: false
					},
					{
						key: 'CAMELOT_TELEPORT',
						name: 'Camelot Teleport',
						level: 45,
						castable: false,
						onItem: false
					}
				]
			}
		}
	};
	const rows = nearRows(v);
	assert.deepEqual(
		rows.map((r) => r.name),
		['Banker', 'Oak tree']
	);
	assert.deepEqual(
		nearMenu(rows[0]).map((o) => [o.label, o.command, o.body]),
		[
			['Talk-to', 'npc_interact', { id: 3010, action: 'Talk-to' }],
			['Bank', 'npc_interact', { id: 3010, action: 'Bank' }],
			['Collect', 'npc_interact', { id: 3010, action: 'Collect' }],
			['Walk here', 'walk', { x: 3220, y: 3218, plane: 0 }]
		]
	);
	assert.deepEqual(
		equipMenu(v, 'Amulet of glory').map((o) => o.label),
		['Remove', 'Rub']
	);
	const [item, locked] = prayerCells(v);
	assert.deepEqual(
		prayerMenu(item).map((o) => [o.label, o.body]),
		[['Activate', { key: 'PROTECT_ITEM', on: true }]]
	);
	assert.deepEqual(prayerMenu(locked), []);
	const spells = spellCells(v);
	assert.deepEqual(
		spells.map((s) => [s.key, s.target]),
		[
			['WIND_STRIKE', 'npc'],
			['ENCHANT_SAPPHIRE_JEWELLERY', 'unsupported'],
			['LOW_LEVEL_ALCHEMY', 'item'],
			['VARROCK_TELEPORT', 'none'],
			['CAMELOT_TELEPORT', 'none']
		]
	);
	const [strike, enchant, alch, varrock, camelot] = spells;
	// Feitiço com alvo seleciona, como no jogo; sem alvo lança; sem alvo alcançável, nada.
	assert.deepEqual(spellMenu(alch)[0].select, {
		kind: 'spell',
		key: 'LOW_LEVEL_ALCHEMY',
		name: 'Low Level Alchemy',
		target: 'item'
	});
	assert.equal(spellMenu(varrock)[0].command, 'spell_cast');
	assert.deepEqual(spellMenu(enchant), []);
	assert.deepEqual(spellMenu(camelot), []);
	const lobster = { id: 379, name: 'Lobster', quantity: 1, slot: 4, actions: ['Eat', 'Use'] };
	const alchSel = spellMenu(alch)[0].select!;
	assert.equal(selectedLabel(alchSel), 'Cast Low Level Alchemy ->');
	assert.deepEqual(
		selectedItemMenu(alchSel, lobster).map((o) => [o.label, o.target, o.command, o.body]),
		[
			[
				'Cast Low Level Alchemy ->',
				'Lobster',
				'spell_cast',
				{ key: 'LOW_LEVEL_ALCHEMY', item: 'Lobster' }
			]
		]
	);
	// Um feitiço de NPC lança no NPC pelo índice da ponte; num objeto só sobra andar e cancelar.
	const strikeSel = spellMenu(strike)[0].select!;
	assert.deepEqual(
		selectedNearMenu(strikeSel, rows[0]).map((o) => [o.label, o.command, o.body]),
		[
			['Cast Wind Strike ->', 'spell_cast', { key: 'WIND_STRIKE', npc: 77 }],
			['Walk here', 'walk', { x: 3220, y: 3218, plane: 0 }]
		]
	);
	assert.deepEqual(
		selectedNearMenu(strikeSel, rows[1]).map((o) => o.label),
		['Walk here']
	);
	// Use seleciona o item; o alvo é outro item, um NPC ou um objeto, nunca o próprio slot.
	// O jogo põe Use em todo item, antes do Drop; as ações do item não o trazem.
	assert.deepEqual(
		itemMenu({ id: 946, name: 'Knife', quantity: 1, actions: ['Drop'] }, false, true).map(
			(o) => o.label
		),
		['Use', 'Drop']
	);
	assert.deepEqual(
		itemMenu({ id: 1351, name: 'Bronze axe', quantity: 1, actions: ['Wield'] }, false, true).map(
			(o) => o.label
		),
		['Wield', 'Use']
	);
	const use = itemMenu(lobster, false, true).find((o) => o.label === 'Use')!;
	assert.deepEqual(use.select, { kind: 'item', name: 'Lobster', slot: 4 });
	assert.deepEqual(
		itemMenu(lobster, false, false).map((o) => o.label),
		['Eat']
	);
	const sel = use.select!;
	assert.deepEqual(
		selectedItemMenu(sel, { id: 590, name: 'Tinderbox', quantity: 1, slot: 0 }).map((o) => [
			o.label,
			o.body
		]),
		[['Use Lobster ->', { item: 'Lobster', onItem: 'Tinderbox' }]]
	);
	assert.deepEqual(
		selectedItemMenu(sel, lobster).map((o) => o.label),
		[]
	);
	assert.deepEqual(
		selectedNearMenu(sel, rows[0]).map((o) => [o.label, o.body]),
		[
			['Use Lobster ->', { item: 'Lobster', npc: 77 }],
			['Walk here', { x: 3220, y: 3218, plane: 0 }]
		]
	);
	assert.deepEqual(selectedNearMenu(sel, rows[1])[0].body, { item: 'Lobster', object: 10820 });
	assert.deepEqual(
		itemMenu(lobster, true, true).map((o) => [o.label, o.ask ?? '']),
		[
			['Deposit-1', ''],
			['Deposit-5', ''],
			['Deposit-10', ''],
			['Deposit-X', 'number'],
			['Deposit-All', '']
		]
	);
	assert.deepEqual(
		bankMenu(lobster).map((o) => o.label),
		['Withdraw-1', 'Withdraw-5', 'Withdraw-10', 'Withdraw-X', 'Withdraw-All']
	);
});
