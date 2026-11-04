/**
 * Único ponto de entrada de dados do planner multi-guia.
 *
 * Hoje devolve fixtures locais (nada vem do backend e nada é persistido). Quando a API
 * multi-guia existir, só este módulo muda: as telas dependem de `PlannerAdapter` e dos
 * tipos de `types.ts`. Não importar fixtures de fora daqui.
 */
import { stepKey } from './compare.ts';
import { fixtureAccount, fixtureGraph, fixtureMicrobot, GOALS } from './fixtures.ts';
import type {
	AccountVM,
	BankLeg,
	BankStatus,
	EntityGraph,
	GoalVM,
	GuideRef,
	MicrobotVM,
	GuideId,
	GuidePlan,
	GuideSummary,
	MockScenario,
	SectionVM,
	StepVM
} from './types.ts';

export interface PlannerAdapter {
	listGuides(): Promise<GuideSummary[]>;
	loadPlan(guideId: GuideId): Promise<GuidePlan>;
	/** Próxima perna de banco da rota; null quando não há mais bancos à frente. */
	nextBankLeg(guideId: GuideId): Promise<BankLeg | null>;
	/** Grafo de entidades (quests, levels, itens, diaries, desbloqueios) com o estado da conta. */
	graph(): Promise<EntityGraph>;
	goals(): Promise<GoalVM[]>;
	/** Onde cada entidade aparece em cada guia: entidade → passos. */
	guideRefs(): Promise<Record<string, GuideRef[]>>;
	account(): Promise<AccountVM>;
	microbot(): Promise<MicrobotVM>;
}

export interface FixtureOptions {
	scenario?: MockScenario;
	/** Estado do banco simulado por guia. */
	bank?: Partial<Record<GuideId, BankStatus['freshness']>>;
	now?: number;
	/** Atraso artificial (ms) para o estado de carregamento ser visível. */
	latencyMs?: number;
	/** `MICROBOT_CONTROL` simulado. */
	microbotControl?: boolean;
}

/** [id local, texto, local, flags]: `bank` marca passo de banco; `ms:<chave>|<rótulo>` marca marco. */
type RawStep = [localId: string, text: string, location: string, flags?: string];
type RawSection = { id: string; title: string; steps: RawStep[] };

const PANDEMONIUM = 'ms:quest:pandemonium|Pandemonium (Sailing)';
const DS1 = 'ms:quest:dragon-slayer-i|Dragon Slayer I';
const GLOVES = 'ms:item:barrows-gloves|Barrows gloves';

// Os IDs locais repetem de propósito entre os guias (1.1.33, 1.2.3…): a UI usa sempre `key`.
const OZIRIS: RawSection[] = [
	{
		id: '1.1',
		title: 'Early quests, wintertodt, ardy cloak 1',
		steps: [
			['1.1.71', 'Buy 2 cabbages and run to the Draynor jail', 'Draynor Village'],
			[
				'1.1.72',
				'Train Firemaking to 50 on the Draynor willows',
				'Draynor Village',
				'ms:skill:firemaking:50|50 Firemaking'
			],
			['1.1.73', 'Bank the logs and grab the axe', 'Draynor Village'],
			['1.1.74', 'Talk to Aggie for the Prince Ali Rescue dye', 'Draynor Village'],
			['1.1.75', 'Run to Draynor bank', 'Draynor Village', 'bank'],
			['1.1.76', 'Buy a chronicle and 2 teleport cards from Diango', 'Draynor Village'],
			['1.1.77', 'Train construction with your planks until out of planks', 'Rimmington'],
			['1.1.78', 'Walk to Port Sarim, buy 1000 feathers', 'Port Sarim'],
			['1.1.78a', 'Complete Pandemonium to unlock Sailing', 'Port Sarim', PANDEMONIUM],
			['1.1.79', 'Fletch arrow shafts to 15 Fletching', 'Port Sarim'],
			['1.1.80', 'Run to the Rimmington house portal', 'Rimmington'],
			[
				'1.1.81',
				'Should now have 15 fletching and 20 construction',
				'Rimmington',
				'ms:skill:construction:20|20 Construction'
			],
			['1.1.82', 'Bank at Falador west', 'Falador', 'bank']
		]
	},
	{
		id: '1.2',
		title: 'Thieving, fishing and mining',
		steps: [
			['1.2.1', 'Thieve men in Lumbridge to 5 Thieving', 'Lumbridge'],
			['1.2.2', 'Fish shrimp at Draynor until 20 Fishing', 'Draynor Village'],
			['1.2.3', 'Mine copper and tin at Varrock east', 'Varrock'],
			[
				'1.2.4',
				'Complete The Grand Tree',
				'Tree Gnome Stronghold',
				'ms:quest:the-grand-tree|The Grand Tree'
			]
		]
	},
	{
		id: '1.3',
		title: 'Fairy rings, 43 prayer, kingdom',
		steps: [
			['1.3.1', 'Start Fairytale I, Growing Pains', 'Lumbridge Swamp'],
			['1.3.2', 'Finish Dragon Slayer I', 'Crandor', DS1],
			['1.3.3', 'Buy bones for prayer at the Gilded Altar', 'Rimmington'],
			[
				'1.3.4',
				'Nature Spirit, then collect Mort Myre fungus',
				'Mort Myre',
				'ms:quest:nature-spirit|Nature Spirit'
			]
		]
	},
	{ id: '1.4', title: 'Various skilling, graceful', steps: grind('1.4', 'Graceful lap', 180) },
	{
		id: '1.5',
		title: 'Diaries and finishing RFD',
		steps: [
			['1.5.20', 'Finish Recipe for Disaster subquests', 'Lumbridge'],
			['1.5.21', 'Buy Barrows gloves from the chest', 'Lumbridge Castle', GLOVES]
		]
	}
];

const BRUHSAILER: RawSection[] = [
	{
		id: '1.1',
		title: 'Opening: Lumbridge, Draynor, Port Sarim',
		steps: [
			['1.1.29', 'Pick up the bucket and pot in Lumbridge castle', 'Lumbridge'],
			['1.1.30', 'Cook’s Assistant', 'Lumbridge Castle'],
			['1.1.31', 'Run to Port Sarim and board the ship', 'Port Sarim'],
			['1.1.32', 'Bank at Port Sarim deposit box', 'Port Sarim', 'bank'],
			['1.1.33', 'Complete Pandemonium to unlock Sailing', 'Port Sarim', PANDEMONIUM],
			['1.1.34', 'Sail to Musa Point and pick bananas', 'Karamja'],
			['1.1.35', 'Charter back and train Sailing to 10', 'The Sea']
		]
	},
	{
		id: '1.2',
		title: 'Early boats and Kandarin',
		steps: [
			['1.2.1', 'Sail to Catherby and buy a lobster pot', 'Catherby'],
			['1.2.2', 'Fish lobsters to 40 Fishing', 'Catherby'],
			['1.2.3', 'Finish Dragon Slayer I', 'Crandor', DS1],
			['1.2.4', 'Ernest the Chicken', 'Draynor Manor'],
			['1.2.5', 'Bank at Draynor', 'Draynor Village', 'bank'],
			[
				'1.2.6',
				'Nature Spirit for the Morytania start',
				'Mort Myre',
				'ms:quest:nature-spirit|Nature Spirit'
			],
			[
				'1.2.7',
				'The Grand Tree via the gnome glider',
				'Tree Gnome Stronghold',
				'ms:quest:the-grand-tree|The Grand Tree'
			]
		]
	},
	{ id: '2.4', title: 'Skilling contínuo', steps: grind('2.4', 'Sailing trip', 90) },
	{
		id: '2.5',
		title: 'Gloves e Slayer',
		steps: [
			['2.5.1', 'Buy Barrows gloves from the chest', 'Lumbridge Castle', GLOVES],
			['2.5.2', 'Start Slayer at Turael', 'Burthorpe']
		]
	}
];

function grind(section: string, what: string, n: number): RawStep[] {
	return Array.from({ length: n }, (_, i): RawStep => {
		const bank = i % 25 === 24;
		return [
			`${section}.${i + 1}`,
			bank ? 'Bank and restock supplies' : `${what} ${i + 1}`,
			bank ? 'Grand Exchange' : 'Kourend',
			bank ? 'bank' : undefined
		];
	});
}

const GUIDES: Record<
	GuideId,
	{ label: string; title: string; author: string; raw: RawSection[]; done: number }
> = {
	'oziris-main': {
		label: 'Oziris',
		title: 'Oziris Ironman Efficiency Guide',
		author: 'OzirisLoL',
		raw: OZIRIS,
		done: 4
	},
	'bruhsailer-main': {
		label: 'BRUHsailer',
		title: 'BRUHsailer Ironman Guide',
		author: 'BRUHsailer',
		raw: BRUHSAILER,
		done: 3
	}
};

function sectionsOf(guideId: GuideId, raw: RawSection[]): SectionVM[] {
	return raw.map((s) => ({
		key: `${guideId}::sec:${s.id}`,
		guideId,
		id: s.id,
		title: s.title,
		steps: s.steps.map(([localId, text, location, flags]): StepVM => {
			const ms = flags?.startsWith('ms:') ? flags.slice(3).split('|') : null;
			return {
				key: stepKey(guideId, localId),
				guideId,
				localId,
				text,
				location,
				bank: flags === 'bank' || undefined,
				milestone: ms?.[0],
				milestoneLabel: ms?.[1]
			};
		})
	}));
}

const LOADOUTS: Record<GuideId, { withdraw: BankLeg['withdraw']; deposit: string[] }> = {
	'oziris-main': {
		withdraw: [
			{ name: 'GP', qty: 5000, inBank: 12400 },
			{ name: 'Bronze nails', qty: 500, inBank: 500 },
			{ name: 'Hammer', qty: 1, inBank: 1 },
			{ name: 'Saw', qty: 1, inBank: 0 },
			{ name: 'Noted planks', qty: 120, inBank: 120 },
			{ name: 'House tab', qty: 1, inBank: 0 }
		],
		deposit: ['Willow logs', 'Bronze axe']
	},
	'bruhsailer-main': {
		withdraw: [
			{ name: 'GP', qty: 3000, inBank: 8100 },
			{ name: 'Bucket', qty: 1, inBank: 1 },
			{ name: 'Lobster', qty: 8, inBank: 5 },
			{ name: 'Rope', qty: 1, inBank: 0 }
		],
		deposit: ['Bananas', 'Pot of flour']
	}
};

export function createFixtureAdapter(opts: FixtureOptions = {}): PlannerAdapter {
	const scenario = opts.scenario ?? 'ready';
	const now = opts.now ?? Date.now();
	const latency = opts.latencyMs ?? 0;

	const gate = async () => {
		if (latency) await new Promise((r) => setTimeout(r, latency));
		// `loading` nunca resolve: congela a UI no esqueleto para revisão visual.
		if (scenario === 'loading') await new Promise<never>(() => {});
		if (scenario === 'error') throw new Error('Fixture: falha simulada ao carregar o plano.');
	};

	const bankFor = (guideId: GuideId): BankStatus => {
		const freshness = opts.bank?.[guideId] ?? (guideId === 'oziris-main' ? 'live' : 'restored');
		if (freshness === 'unavailable') return { freshness, capturedAt: null };
		return { freshness, capturedAt: now - (freshness === 'live' ? 30_000 : 3 * 3_600_000) };
	};

	const microbotControl = opts.microbotControl ?? true;

	const plan = (guideId: GuideId): GuidePlan => {
		const g = GUIDES[guideId];
		if (!g) throw new Error(`Guia desconhecido: ${guideId}`);
		const sections = scenario === 'empty' ? [] : sectionsOf(guideId, g.raw);
		const steps = sections.reduce((n, s) => n + s.steps.length, 0);
		return {
			guide: { id: guideId, label: g.label, title: g.title, author: g.author, steps },
			sections,
			doneCount: scenario === 'empty' ? 0 : g.done
		};
	};

	const refs = (): Record<string, GuideRef[]> => {
		const out: Record<string, GuideRef[]> = {};
		for (const id of Object.keys(GUIDES)) {
			const p = plan(id);
			let i = 0;
			for (const s of p.sections)
				for (const st of s.steps) {
					if (st.milestone)
						(out[st.milestone] ??= []).push({
							guideId: id,
							guideLabel: p.guide.label,
							stepKey: st.key,
							localId: st.localId,
							sectionId: s.id,
							text: st.text,
							done: i < p.doneCount
						});
					i++;
				}
		}
		return out;
	};

	return {
		async graph() {
			await gate();
			return fixtureGraph(now);
		},
		async goals() {
			await gate();
			return scenario === 'empty' ? [] : GOALS.map((g) => ({ ...g }));
		},
		async guideRefs() {
			await gate();
			return refs();
		},
		async account() {
			await gate();
			return { ...fixtureAccount(now), bank: bankFor('oziris-main') };
		},
		async microbot() {
			await gate();
			return fixtureMicrobot(now, microbotControl);
		},
		async listGuides() {
			await gate();
			return Object.keys(GUIDES).map((id) => plan(id).guide);
		},
		async loadPlan(guideId) {
			await gate();
			return plan(guideId);
		},
		async nextBankLeg(guideId) {
			await gate();
			const p = plan(guideId);
			const steps = p.sections.flatMap((s) => s.steps);
			const idx = steps.findIndex((s, i) => i >= p.doneCount && s.bank);
			if (idx < 0) return null;
			const next = steps.findIndex((s, i) => i > idx && s.bank);
			const bank = bankFor(guideId);
			const trusted = bank.freshness !== 'unavailable';
			return {
				guideId,
				atStepKey: steps[idx].key,
				atStepId: steps[idx].localId,
				location: steps[idx].location ?? 'Banco',
				withdraw: (LOADOUTS[guideId]?.withdraw ?? []).map((i) => ({
					...i,
					inBank: trusted ? i.inBank : null
				})),
				deposit: LOADOUTS[guideId]?.deposit ?? [],
				stepsUntilNextBank: next < 0 ? steps.length - idx - 1 : next - idx - 1,
				bank
			};
		}
	};
}
