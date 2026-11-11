import type { CATierView, SkillView } from './types.ts';

/** Pedaços puros da Conta (V6Conta1440): XP por nível, progresso até o próximo e combate. */

/** XP mínimo de cada nível (1..99), a tabela do jogo. */
export const XP_TABLE: number[] = (() => {
	const out = [0, 0];
	let points = 0;
	for (let lvl = 1; lvl < 99; lvl++) {
		points += Math.floor(lvl + 300 * Math.pow(2, lvl / 7));
		out.push(Math.floor(points / 4));
	}
	return out;
})();

/** Fração (0..1) do caminho do nível atual até o próximo; 1 no 99. */
export function toNext(level: number, xp: number): number {
	if (level >= 99) return 1;
	const lo = XP_TABLE[level] ?? 0;
	const hi = XP_TABLE[level + 1] ?? lo;
	return hi > lo ? Math.min(1, Math.max(0, (xp - lo) / (hi - lo))) : 0;
}

/** Nível de combate pela fórmula do jogo; null se faltar algum skill de combate. */
export function combatLevel(skills: Pick<SkillView, 'skill' | 'level'>[]): number | null {
	const lv = (s: string) => skills.find((x) => x.skill === s)?.level;
	const [att, str, def, hp, pray, rng, mag] = [
		'attack',
		'strength',
		'defence',
		'hitpoints',
		'prayer',
		'ranged',
		'magic'
	].map(lv);
	if ([att, str, def, hp, pray, rng, mag].some((x) => x === undefined)) return null;
	const base = 0.25 * (def! + hp! + Math.floor(pray! / 2));
	const melee = 0.325 * (att! + str!);
	const range = 0.325 * Math.floor((3 * rng!) / 2);
	const magic = 0.325 * Math.floor((3 * mag!) / 2);
	return Math.floor(base + Math.max(melee, range, magic));
}

/** A ordem dos skills na aba do jogo (3 colunas). */
export const SKILL_ORDER = [
	'attack',
	'hitpoints',
	'mining',
	'strength',
	'agility',
	'smithing',
	'defence',
	'herblore',
	'fishing',
	'ranged',
	'thieving',
	'cooking',
	'prayer',
	'crafting',
	'firemaking',
	'magic',
	'fletching',
	'woodcutting',
	'runecraft',
	'slayer',
	'farming',
	'construction',
	'hunter',
	'sailing'
];

/** "8.2k", "1.2m": quantidade curta como no jogo. */
export function shortQty(n: number): string {
	if (n >= 10_000_000) return `${Math.floor(n / 1_000_000)}m`;
	if (n >= 100_000) return `${Math.floor(n / 1000)}k`;
	if (n >= 10_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
	return String(n);
}

export interface EquipCell {
	slot: string;
	/** Nome do slot no título da célula vazia ("Capa: vazio"). */
	label: string;
	row: number;
	col: number;
	item?: { id: number; name: string; qty: number };
	/** O escudo fica vazio porque a arma é de duas mãos. */
	blockedBy2h?: boolean;
}

/**
 * Os onze slots do boneco do jogo (V6Conta1440): cabeça no topo; capa, amuleto e munição; arma,
 * corpo e escudo; pernas; mãos, pés e anel. Arma de duas mãos ("2h") ocupa o slot da arma.
 */
const SLOTS: [string, string, number, number][] = [
	['head', 'Cabeça', 1, 2],
	['cape', 'Capa', 2, 1],
	['neck', 'Amuleto', 2, 2],
	['ammo', 'Munição', 2, 3],
	['weapon', 'Arma', 3, 1],
	['body', 'Corpo', 3, 2],
	['shield', 'Escudo', 3, 3],
	['legs', 'Pernas', 4, 2],
	['hands', 'Mãos', 5, 1],
	['feet', 'Pés', 5, 2],
	['ring', 'Anel', 5, 3]
];

/**
 * Põe cada item do equipamento no único slot que ele ocupa (slot do catálogo da wiki). O que
 * não tem slot conhecido, ou disputa um slot já ocupado, volta em `rest` para não sumir.
 */
export function equipmentLayout(
	items: { id: number; name: string; qty: number; slot?: string }[]
): { cells: EquipCell[]; rest: { id: number; name: string; qty: number }[] } {
	const cells: EquipCell[] = SLOTS.map(([slot, label, row, col]) => ({ slot, label, row, col }));
	const rest: { id: number; name: string; qty: number }[] = [];
	for (const it of items) {
		const slot = it.slot === '2h' ? 'weapon' : it.slot;
		const cell = cells.find((c) => c.slot === slot);
		const { id, name, qty } = it;
		if (!cell || cell.item) {
			rest.push({ id, name, qty });
			continue;
		}
		cell.item = { id, name, qty };
		if (it.slot === '2h') {
			const shield = cells.find((c) => c.slot === 'shield');
			if (shield) shield.blockedBy2h = true;
		}
	}
	return { cells, rest };
}

/** Os tiers de combat achievements como o jogo os chama. */
export const CA_TIER_LABEL: Record<CATierView['tier'], string> = {
	easy: 'Easy',
	medium: 'Medium',
	hard: 'Hard',
	elite: 'Elite',
	master: 'Master',
	grandmaster: 'Grandmaster'
};

/** Feitos e total somando os tiers lidos. */
export function caTotals(tiers: CATierView[]): { done: number; total: number } {
	return tiers.reduce((a, t) => ({ done: a.done + t.done, total: a.total + t.total }), {
		done: 0,
		total: 0
	});
}

/**
 * Quando a contagem pelos bits não bate com o total do próprio jogo (catálogo do wiki atrás do jogo,
 * ou um bit lido errado), o aviso que vai no title da linha; null quando bate ou o jogo não disse.
 */
export function caMismatch(t: CATierView): string | null {
	if (t.game_done === undefined || t.game_done === t.done) return null;
	return `o jogo conta ${t.game_done} feitos neste tier; o catálogo, ${t.done}`;
}
