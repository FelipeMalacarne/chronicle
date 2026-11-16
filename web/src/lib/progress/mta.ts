/*
 * Conta › Personagem › Mage Training Arena: the view model of the calculator card. The arithmetic
 * is the server's (POST /api/mta/calculate, internal/core/mta); this file only turns the account's
 * state into its input (Magic level, rewards already owned, runes carried) and the answer into
 * rows. Pure, tested in node.
 */
import type { AccountState } from './types.ts';

export const ROOM_IDS = ['telekinetic', 'graveyard', 'enchantment', 'alchemist'] as const;
export type RoomId = (typeof ROOM_IDS)[number];
export type RuneId = 'law' | 'nature' | 'cosmic';

export interface MtaPoints {
	telekinetic: number;
	graveyard: number;
	enchantment: number;
	alchemist: number;
}
export interface MtaRunes {
	law: number;
	nature: number;
	cosmic: number;
}
export interface MtaReward {
	id: string;
	name: string;
	item_id: number;
	cost: MtaPoints;
	requires?: string;
}
export interface MtaRoom {
	id: RoomId;
	name: string;
	rune: RuneId;
	cap: number;
}
export interface MtaTable {
	source: string;
	rooms: MtaRoom[];
	enchant_levels: { level: number; magic: number }[];
	rewards: MtaReward[];
}
export interface MtaInput {
	rewards: string[];
	owned?: string[];
	points?: Partial<MtaPoints>;
	magic?: number;
	enchant_level?: number;
}
export interface MtaResult {
	buy: string[];
	needed: MtaPoints;
	missing: MtaPoints;
	runes: MtaRunes;
	enchant_level: number;
}

/** Item ids of the client's ItemID (RUNE_LAW, RUNE_NATURE, RUNE_COSMIC). */
const RUNE_ITEM: Record<number, RuneId> = { 563: 'law', 561: 'nature', 564: 'cosmic' };
export const RUNE_LABEL: Record<RuneId, string> = {
	law: 'Law',
	nature: 'Nature',
	cosmic: 'Cosmic'
};
export const RUNE_ITEM_ID: Record<RuneId, number> = { law: 563, nature: 561, cosmic: 564 };
export const RUNE_ORDER: RuneId[] = ['law', 'nature', 'cosmic'];

const containers = (st: AccountState) =>
	st.containers.filter((c) => c.state !== 'unavailable' && c.items);

/** The Magic level the account was last read at; undefined without a reading. */
export function magicLevel(st: AccountState): number | undefined {
	return st.skills.find((s) => s.skill === 'magic')?.level;
}

/**
 * The runes inventory, equipment and bank hold together; null when no container was ever read
 * (unknown is never zero, rule 8).
 */
export function carriedRunes(st: AccountState): MtaRunes | null {
	const read = containers(st);
	if (!read.length) return null;
	const out: MtaRunes = { law: 0, nature: 0, cosmic: 0 };
	for (const c of read) for (const i of c.items) if (RUNE_ITEM[i.id]) out[RUNE_ITEM[i.id]] += i.qty;
	return out;
}

/** The rewards the account holds (inventory, equipment or bank), by item id; Bones to Peaches is an unlock, read by the plugin. */
export function ownedRewards(table: MtaTable, st: AccountState): string[] {
	const have = new Set<number>();
	for (const c of containers(st)) for (const i of c.items) have.add(i.id);
	const unlocked = (st.minigame ?? []).some((m) => m.key === 'mta.bones_to_peaches' && m.value > 0);
	return table.rewards
		.filter((r) => have.has(r.item_id) || (r.id === 'bones_to_peaches' && unlocked))
		.map((r) => r.id);
}

/** The pizazz points Chronicle Link read in the arena (mta.<room>), and when; null until it read every room. */
export function readPoints(st: AccountState): { points: MtaPoints; observedAt: number } | null {
	const points: Partial<MtaPoints> = {};
	let observedAt = 0;
	for (const m of st.minigame ?? []) {
		const room = ROOM_IDS.find((r) => m.key === `mta.${r}`);
		if (!room) continue;
		points[room] = m.value;
		observedAt = Math.max(observedAt, Date.parse(m.observed_at) || 0);
	}
	return ROOM_IDS.every((r) => points[r] !== undefined)
		? { points: points as MtaPoints, observedAt }
		: null;
}

/** A points field: whole numbers only, 0 up to the room's cap; anything else is 0. */
export function parsePoints(text: string, cap: number): number {
	const v = Math.floor(Number(text.replace(/\D/g, '')));
	return Number.isFinite(v) ? Math.min(Math.max(v, 0), cap) : 0;
}

export interface RuneRow {
	rune: RuneId;
	need: number;
	/** What the account carries; null when no container was read. */
	have: number | null;
	/** need - have, never negative; null without a reading. */
	short: number | null;
}

export function runeRows(res: MtaResult, have: MtaRunes | null): RuneRow[] {
	return RUNE_ORDER.map((rune) => {
		const need = res.runes[rune];
		const h = have ? have[rune] : null;
		return { rune, need, have: h, short: h === null ? null : Math.max(need - h, 0) };
	});
}

/** The rewards of `buy`, in purchase order, by name. */
export function buyNames(table: MtaTable, buy: string[]): string[] {
	const by = new Map(table.rewards.map((r) => [r.id, r.name]));
	return buy.map((id) => by.get(id) ?? id);
}

/** Why the calculator cannot answer, for the 422 of a Magic that casts no Enchant. */
export const NO_ENCHANT =
	'Com esse Magic nenhum Enchant é conjurado (o primeiro pede Magic 7): o Enchanting Chamber não rende pontos.';
