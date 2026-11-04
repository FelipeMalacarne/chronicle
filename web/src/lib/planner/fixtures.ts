/**
 * Dados de exemplo do planner. Importado SOMENTE por `adapter.ts`: quando a API existir,
 * este arquivo sai inteiro e as telas não mudam.
 */
import type { AccountVM, EdgeVM, EntityVM, GoalVM, MicrobotVM, SkillVM } from './types.ts';

const MIN = 60_000;
const H = 60 * MIN;

/* ------------------------------------------------------------------ conta (níveis atuais) */

export const LEVELS: Record<string, number> = {
	attack: 40,
	strength: 42,
	defence: 30,
	ranged: 45,
	prayer: 31,
	magic: 47,
	runecraft: 20,
	construction: 8,
	hitpoints: 42,
	agility: 31,
	herblore: 18,
	thieving: 38,
	crafting: 35,
	fletching: 12,
	slayer: 1,
	hunter: 9,
	mining: 30,
	smithing: 20,
	fishing: 35,
	cooking: 34,
	firemaking: 50,
	woodcutting: 41,
	farming: 12,
	sailing: 1
};
export const QUEST_POINTS = 41;

/* ------------------------------------------------------------------ entidades */

type Q = [slug: string, name: string, summary: string, observed?: 'done' | 'in_progress'];

const QUESTS: Q[] = [
	['cooks-assistant', 'Cook’s Assistant', 'Novice · 1 QP', 'done'],
	['restless-ghost', 'The Restless Ghost', 'Novice · 1 QP', 'done'],
	['priest-in-peril', 'Priest in Peril', 'Novice · 1 QP', 'done'],
	['waterfall-quest', 'Waterfall Quest', 'Intermediate · 1 QP', 'done'],
	['tree-gnome-village', 'Tree Gnome Village', 'Intermediate · 2 QP', 'done'],
	['demon-slayer', 'Demon Slayer', 'Novice · 3 QP', 'done'],
	['goblin-diplomacy', 'Goblin Diplomacy', 'Novice · 5 QP', 'done'],
	['fishing-contest', 'Fishing Contest', 'Novice · 1 QP', 'done'],
	['plague-city', 'Plague City', 'Novice · 1 QP', 'done'],
	['rune-mysteries', 'Rune Mysteries', 'Novice · 1 QP', 'done'],
	['biohazard', 'Biohazard', 'Novice · 3 QP', 'in_progress'],
	['murder-mystery', 'Murder Mystery', 'Novice · 3 QP'],
	['nature-spirit', 'Nature Spirit', 'Novice · 2 QP'],
	['big-chompy-bird-hunting', 'Big Chompy Bird Hunting', 'Intermediate · 2 QP'],
	['the-golem', 'The Golem', 'Novice · 1 QP'],
	['shadow-of-the-storm', 'Shadow of the Storm', 'Intermediate · 1 QP'],
	['the-grand-tree', 'The Grand Tree', 'Experienced · 5 QP'],
	['monkey-madness-i', 'Monkey Madness I', 'Master · 3 QP'],
	['death-plateau', 'Death Plateau', 'Novice · 1 QP'],
	['troll-stronghold', 'Troll Stronghold', 'Experienced · 1 QP'],
	['temple-of-ikov', 'Temple of Ikov', 'Experienced · 1 QP'],
	['the-dig-site', 'The Dig Site', 'Intermediate · 2 QP'],
	['the-tourist-trap', 'The Tourist Trap', 'Intermediate · 2 QP'],
	['alfred-grimhands-barcrawl', 'Alfred Grimhand’s Barcrawl', 'Miniquest'],
	['horror-from-the-deep', 'Horror from the Deep', 'Experienced · 2 QP'],
	['desert-treasure-i', 'Desert Treasure I', 'Master · 3 QP · Ancient Magicks'],
	['dragon-slayer-i', 'Dragon Slayer I', 'Experienced · 2 QP · Rune platebody'],
	['pandemonium', 'Pandemonium', 'Novice · 1 QP · desbloqueia Sailing'],
	['rfd-start', 'RFD: Another Cook’s Quest', 'Recipe for Disaster · início', 'done'],
	['rfd-dwarf', 'RFD: Mountain Dwarf', 'Recipe for Disaster', 'done'],
	['rfd-goblin', 'RFD: Goblin Generals', 'Recipe for Disaster'],
	['rfd-pirate', 'RFD: Pirate Pete', 'Recipe for Disaster'],
	['rfd-lumbridge-guide', 'RFD: Lumbridge Guide', 'Recipe for Disaster'],
	['rfd-evil-dave', 'RFD: Evil Dave', 'Recipe for Disaster'],
	['rfd-skrach', 'RFD: Skrach Uglogwee', 'Recipe for Disaster'],
	['rfd-sir-amik', 'RFD: Sir Amik Varze', 'Recipe for Disaster'],
	['rfd-awowogei', 'RFD: King Awowogei', 'Recipe for Disaster'],
	['rfd-final', 'RFD: Culinaromancer', 'Recipe for Disaster · final']
];

type L = [skill: string, level: number];
const LEVEL_NODES: L[] = [
	['cooking', 10],
	['cooking', 25],
	['cooking', 30],
	['cooking', 31],
	['cooking', 40],
	['cooking', 41],
	['cooking', 70],
	['firemaking', 20],
	['firemaking', 50],
	['agility', 10],
	['agility', 15],
	['agility', 25],
	['agility', 35],
	['agility', 48],
	['magic', 50],
	['thieving', 5],
	['thieving', 25],
	['thieving', 42],
	['thieving', 53],
	['slayer', 10],
	['fishing', 10],
	['fletching', 5],
	['fletching', 10],
	['fletching', 15],
	['ranged', 30],
	['ranged', 40],
	['herblore', 10],
	['crafting', 20],
	['smithing', 20],
	['construction', 20]
];

const cap = (s: string) => s[0].toUpperCase() + s.slice(1);

function buildEntities(now: number): Record<string, EntityVM> {
	const out: Record<string, EntityVM> = {};
	for (const [slug, name, summary, observed] of QUESTS) {
		out[`quest:${slug}`] = {
			id: `quest:${slug}`,
			kind: 'quest',
			name,
			summary,
			observed: observed ?? 'none',
			evidence: observed ? { source: 'plugin', at: now - 3 * H } : undefined,
			wiki: `https://oldschool.runescape.wiki/w/${encodeURIComponent(name.replace(/^RFD: /, 'Recipe for Disaster/'))}`
		};
	}
	for (const [skill, level] of LEVEL_NODES) {
		const current = LEVELS[skill] ?? 1;
		out[`skill:${skill}:${level}`] = {
			id: `skill:${skill}:${level}`,
			kind: 'skill',
			name: `${level} ${cap(skill)}`,
			summary:
				current >= level
					? `você tem ${current}`
					: `você tem ${current} · faltam ${level - current}`,
			level,
			current,
			observed: current >= level ? 'done' : 'none',
			evidence: { source: 'hiscores', at: now - 4 * MIN },
			wiki: `https://oldschool.runescape.wiki/w/${cap(skill)}`
		};
	}
	const qp = (n: number): EntityVM => ({
		id: `skill:qp:${n}`,
		kind: 'skill',
		name: `${n} Quest points`,
		summary: `você tem ${QUEST_POINTS}`,
		level: n,
		current: QUEST_POINTS,
		observed: QUEST_POINTS >= n ? 'done' : 'none',
		evidence: { source: 'plugin', at: now - 3 * H }
	});
	for (const n of [32, 107, 175]) out[`skill:qp:${n}`] = qp(n);
	const extra: EntityVM[] = [
		{
			id: 'item:barrows-gloves',
			kind: 'item',
			name: 'Barrows gloves',
			summary: 'Melhores luvas do early/mid game',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Barrows_gloves'
		},
		{
			id: 'item:anti-dragon-shield',
			kind: 'item',
			name: 'Anti-dragon shield',
			summary: 'Duke Horacio, Lumbridge',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Anti-dragon_shield'
		},
		{
			id: 'item:ardougne-cloak-1',
			kind: 'item',
			name: 'Ardougne cloak 1',
			summary: 'Teleporte para o mosteiro',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Ardougne_cloak_1'
		},
		{
			id: 'diary:ardougne-easy',
			kind: 'diary',
			name: 'Ardougne Diary · Easy',
			summary: '10 tarefas',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Ardougne_Diary'
		},
		{
			id: 'unlock:ancient-magicks',
			kind: 'unlock',
			name: 'Ancient Magicks',
			summary: 'Spellbook · Ice barrage',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Ancient_Magicks'
		},
		{
			id: 'unlock:sailing',
			kind: 'unlock',
			name: 'Sailing',
			summary: 'Skill · barcos e rotas',
			observed: 'none',
			wiki: 'https://oldschool.runescape.wiki/w/Sailing'
		},
		{
			id: 'boss:elvarg',
			kind: 'boss',
			name: 'Elvarg',
			summary: 'Dragon Slayer I',
			observed: 'none'
		}
	];
	for (const e of extra) out[e.id] = e;
	return out;
}

/** [requisito, dependente] */
const E: [string, string][] = [
	['quest:cooks-assistant', 'quest:rfd-start'],
	['skill:cooking:10', 'quest:rfd-start'],
	['quest:rfd-start', 'quest:rfd-dwarf'],
	['quest:fishing-contest', 'quest:rfd-dwarf'],
	['quest:rfd-start', 'quest:rfd-goblin'],
	['quest:goblin-diplomacy', 'quest:rfd-goblin'],
	['quest:rfd-start', 'quest:rfd-pirate'],
	['skill:cooking:31', 'quest:rfd-pirate'],
	['quest:rfd-start', 'quest:rfd-lumbridge-guide'],
	['quest:big-chompy-bird-hunting', 'quest:rfd-lumbridge-guide'],
	['quest:biohazard', 'quest:rfd-lumbridge-guide'],
	['quest:demon-slayer', 'quest:rfd-lumbridge-guide'],
	['quest:murder-mystery', 'quest:rfd-lumbridge-guide'],
	['quest:nature-spirit', 'quest:rfd-lumbridge-guide'],
	['skill:cooking:40', 'quest:rfd-lumbridge-guide'],
	['quest:rfd-start', 'quest:rfd-evil-dave'],
	['quest:shadow-of-the-storm', 'quest:rfd-evil-dave'],
	['skill:cooking:25', 'quest:rfd-evil-dave'],
	['quest:rfd-start', 'quest:rfd-skrach'],
	['quest:big-chompy-bird-hunting', 'quest:rfd-skrach'],
	['skill:cooking:41', 'quest:rfd-skrach'],
	['skill:firemaking:20', 'quest:rfd-skrach'],
	['quest:rfd-start', 'quest:rfd-sir-amik'],
	['skill:qp:107', 'quest:rfd-sir-amik'],
	['quest:rfd-start', 'quest:rfd-awowogei'],
	['quest:monkey-madness-i', 'quest:rfd-awowogei'],
	['skill:cooking:70', 'quest:rfd-awowogei'],
	['skill:agility:48', 'quest:rfd-awowogei'],
	['quest:rfd-dwarf', 'quest:rfd-final'],
	['quest:rfd-goblin', 'quest:rfd-final'],
	['quest:rfd-pirate', 'quest:rfd-final'],
	['quest:rfd-lumbridge-guide', 'quest:rfd-final'],
	['quest:rfd-evil-dave', 'quest:rfd-final'],
	['quest:rfd-skrach', 'quest:rfd-final'],
	['quest:rfd-sir-amik', 'quest:rfd-final'],
	['quest:rfd-awowogei', 'quest:rfd-final'],
	['quest:desert-treasure-i', 'quest:rfd-final'],
	['quest:horror-from-the-deep', 'quest:rfd-final'],
	['skill:qp:175', 'quest:rfd-final'],
	['quest:rfd-final', 'item:barrows-gloves'],
	['quest:plague-city', 'quest:biohazard'],
	['quest:priest-in-peril', 'quest:nature-spirit'],
	['quest:restless-ghost', 'quest:nature-spirit'],
	['skill:fletching:5', 'quest:big-chompy-bird-hunting'],
	['skill:cooking:30', 'quest:big-chompy-bird-hunting'],
	['skill:ranged:30', 'quest:big-chompy-bird-hunting'],
	['skill:crafting:20', 'quest:the-golem'],
	['skill:thieving:25', 'quest:the-golem'],
	['quest:demon-slayer', 'quest:shadow-of-the-storm'],
	['quest:the-golem', 'quest:shadow-of-the-storm'],
	['skill:agility:25', 'quest:the-grand-tree'],
	['quest:tree-gnome-village', 'quest:monkey-madness-i'],
	['quest:the-grand-tree', 'quest:monkey-madness-i'],
	['quest:death-plateau', 'quest:troll-stronghold'],
	['skill:agility:15', 'quest:troll-stronghold'],
	['skill:thieving:42', 'quest:temple-of-ikov'],
	['skill:ranged:40', 'quest:temple-of-ikov'],
	['skill:agility:10', 'quest:the-dig-site'],
	['skill:herblore:10', 'quest:the-dig-site'],
	['skill:thieving:25', 'quest:the-dig-site'],
	['skill:fletching:10', 'quest:the-tourist-trap'],
	['skill:smithing:20', 'quest:the-tourist-trap'],
	['quest:alfred-grimhands-barcrawl', 'quest:horror-from-the-deep'],
	['skill:agility:35', 'quest:horror-from-the-deep'],
	['quest:the-dig-site', 'quest:desert-treasure-i'],
	['quest:temple-of-ikov', 'quest:desert-treasure-i'],
	['quest:the-tourist-trap', 'quest:desert-treasure-i'],
	['quest:troll-stronghold', 'quest:desert-treasure-i'],
	['quest:priest-in-peril', 'quest:desert-treasure-i'],
	['quest:waterfall-quest', 'quest:desert-treasure-i'],
	['skill:magic:50', 'quest:desert-treasure-i'],
	['skill:thieving:53', 'quest:desert-treasure-i'],
	['skill:firemaking:50', 'quest:desert-treasure-i'],
	['skill:slayer:10', 'quest:desert-treasure-i'],
	['quest:desert-treasure-i', 'unlock:ancient-magicks'],
	['skill:qp:32', 'quest:dragon-slayer-i'],
	['item:anti-dragon-shield', 'quest:dragon-slayer-i'],
	['quest:dragon-slayer-i', 'boss:elvarg'],
	['quest:rune-mysteries', 'diary:ardougne-easy'],
	['skill:thieving:5', 'diary:ardougne-easy'],
	['diary:ardougne-easy', 'item:ardougne-cloak-1'],
	['quest:pandemonium', 'unlock:sailing'],
	['skill:fishing:10', 'quest:fishing-contest']
];

export function fixtureGraph(now: number): { entities: Record<string, EntityVM>; edges: EdgeVM[] } {
	return { entities: buildEntities(now), edges: E.map(([from, to]) => ({ from, to })) };
}

export const GOALS: GoalVM[] = [
	{ id: 'g-gloves', target: 'item:barrows-gloves', pinned: true, note: 'meta longa do early game' },
	{ id: 'g-ds1', target: 'quest:dragon-slayer-i', pinned: true },
	{ id: 'g-ardy', target: 'item:ardougne-cloak-1', pinned: false },
	{ id: 'g-dt1', target: 'unlock:ancient-magicks', pinned: false }
];

/* ------------------------------------------------------------------ conta */

const SKILL_NEXT: Record<string, SkillVM['next']> = {
	construction: { level: 20, stepId: '1.1.81' },
	fletching: { level: 15, stepId: '1.1.81' },
	thieving: { level: 42, stepId: '1.1.95' },
	agility: { level: 40, stepId: '1.2.7' },
	cooking: { level: 40, stepId: '1.3.12' },
	magic: { level: 50, stepId: '1.3.40' },
	fishing: { level: 40, stepId: '1.2.2' },
	firemaking: { level: 60, stepId: '1.4.3' }
};

const XP_AT = (l: number) => {
	let pts = 0;
	for (let i = 1; i < l; i++) pts += Math.floor(i + 300 * 2 ** (i / 7));
	return Math.floor(pts / 4);
};

export function fixtureAccount(now: number): Omit<AccountVM, 'bank'> {
	const skills: SkillVM[] = Object.entries(LEVELS).map(([skill, level]) => ({
		skill,
		level,
		xp: XP_AT(level) + 1200,
		next: SKILL_NEXT[skill]
	}));
	const totalLevel = skills.reduce((n, s) => n + s.level, 0);
	const areas = [
		'Ardougne',
		'Desert',
		'Falador',
		'Fremennik',
		'Kandarin',
		'Karamja',
		'Kourend',
		'Lumbridge',
		'Morytania',
		'Varrock',
		'Western',
		'Wilderness'
	];
	const diaries = areas.map((area) => ({
		area,
		tiers: (area === 'Lumbridge' || area === 'Karamja'
			? ['done', 'available', 'blocked', 'blocked']
			: area === 'Ardougne' || area === 'Falador' || area === 'Varrock'
				? ['available', 'blocked', 'blocked', 'blocked']
				: ['blocked', 'blocked', 'blocked', 'blocked']) as ('done' | 'available' | 'blocked')[]
	}));
	return {
		rsn: 'Iron Felipe',
		mode: 'Ironman',
		totalLevel,
		combat: 52,
		questPoints: QUEST_POINTS,
		skills,
		diaries,
		bankItems: [
			{ name: 'Coins', qty: 12_400 },
			{ name: 'Noted planks', qty: 120, reservedFor: '1.1.77' },
			{ name: 'Bronze nails', qty: 500, reservedFor: '1.1.77' },
			{ name: 'Willow logs', qty: 214 },
			{ name: 'Lobster', qty: 14, reservedFor: '1.3.2' },
			{ name: 'Rope', qty: 0, reservedFor: '1.1.64' },
			{ name: 'Ghostspeak amulet', qty: 1, reservedFor: '1.5.20' },
			{ name: 'Arrow shafts', qty: 1000, reservedFor: '1.1.79' }
		],
		sessions: [
			{
				id: 's2',
				start: now - 3 * H,
				end: now - 1.5 * H,
				changes: [
					{ kind: 'level', text: 'Firemaking 47 → 50' },
					{ kind: 'quest', text: 'Plague City concluída' },
					{ kind: 'step', text: '14 passos concluídos (8 auto, 6 presumidos)' },
					{ kind: 'unlock', text: 'Biohazard ficou disponível' }
				]
			},
			{
				id: 's1',
				start: now - 27 * H,
				end: now - 25 * H,
				changes: [
					{ kind: 'level', text: 'Thieving 33 → 38' },
					{ kind: 'kc', text: 'Wintertodt +12 KC' },
					{ kind: 'step', text: '9 passos concluídos' }
				]
			}
		],
		sources: [
			{
				name: 'Plugin (somente leitura)',
				at: now - 2 * MIN,
				state: 'ok',
				detail: 'quests, varbits, banco'
			},
			{ name: 'Hiscores', at: now - 4 * MIN, state: 'ok', detail: 'skills, KC' },
			{ name: 'Wise Old Man', at: now - 31 * H, state: 'stale', detail: 'histórico de XP' },
			{ name: 'WikiSync', at: null, state: 'off', detail: 'desligado' }
		]
	};
}

/* ------------------------------------------------------------------ Microbot */

export function fixtureMicrobot(now: number, controlEnabled: boolean): MicrobotVM {
	return {
		controlEnabled,
		agentConnected: controlEnabled,
		client: controlEnabled ? 'logged_in' : 'offline',
		world: 420,
		character: 'Iron Felipe',
		position: { region: 'Fossil Island', x: 3764, y: 3880, plane: 0 },
		hp: { now: 38, max: 42 },
		prayer: { now: 31, max: 31 },
		run: 87,
		inventoryUsed: 12,
		humanInput: false,
		screenshotAt: now - 4_000,
		scripts: [
			{
				name: 'Birdhouse Runner',
				version: '1.4.2',
				state: 'running',
				runtimeMs: 12 * MIN + 31_000,
				machineState: 'WALKING_TO_VERDANT_VALLEY',
				health: 'ok',
				result: '2/4 birdhouses'
			},
			{ name: 'Herb Run', version: '2.0.0', state: 'stopped', runtimeMs: 0 },
			{
				name: 'Tithe Farm',
				version: '0.9.1',
				state: 'failed',
				runtimeMs: 4 * MIN,
				health: 'error',
				result: 'sem sementes no inventário'
			},
			{ name: 'Auto Login', version: '1.0.3', state: 'stopped', runtimeMs: 0 }
		],
		routines: [
			{
				id: 'r-birdhouses',
				name: 'Birdhouses a cada 50 min',
				enabled: true,
				schedule: 'a cada 50 min, 8h–23h',
				nextRunAt: now + 23 * MIN,
				steps: [
					{ kind: 'client_start', label: 'Abrir cliente (headless)' },
					{ kind: 'login', label: 'Logar no mundo 420' },
					{ kind: 'plugin_start', label: 'Iniciar Birdhouse Runner' },
					{ kind: 'wait_until', label: 'Esperar o plugin parar (máx. 15 min)' },
					{ kind: 'logout', label: 'Deslogar' }
				],
				lastRun: { at: now - 27 * MIN, status: 'ok', detail: '4 birdhouses · +2,1k Hunter XP' }
			},
			{
				id: 'r-herbs',
				name: 'Herb run diária',
				enabled: false,
				schedule: 'todo dia às 19h',
				steps: [
					{ kind: 'login', label: 'Logar' },
					{ kind: 'plugin_config', label: 'Herb Run: rota = 6 patches' },
					{ kind: 'plugin_start', label: 'Iniciar Herb Run' },
					{ kind: 'wait_until', label: 'Esperar resultado "run completa"' },
					{ kind: 'logout', label: 'Deslogar' }
				],
				lastRun: {
					at: now - 2 * 24 * H,
					status: 'failed',
					detail: 'Tithe seeds ausentes; parou no passo 3'
				}
			}
		],
		audit: [
			{
				at: now - 13 * MIN,
				actor: 'rotina · Birdhouses',
				action: 'plugin_start Birdhouse Runner',
				result: 'ok'
			},
			{
				at: now - 13.5 * MIN,
				actor: 'rotina · Birdhouses',
				action: 'login mundo 420',
				result: 'ok'
			},
			{ at: now - 41 * MIN, actor: 'felipe', action: 'plugin_stop Tithe Farm', result: 'ok' },
			{ at: now - 44 * MIN, actor: 'felipe', action: 'plugin_start Tithe Farm', result: 'failed' }
		]
	};
}
