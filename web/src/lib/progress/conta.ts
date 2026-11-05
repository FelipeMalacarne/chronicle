import type { SkillView } from './types.ts';

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
