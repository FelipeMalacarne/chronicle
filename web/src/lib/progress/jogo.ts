/**
 * Automação › Cockpit, o jogo visto pelo agent (doc 08): inventário, equipamento, skills, banco,
 * diálogo, o que está por perto, as state machines e o chat do jogo, a partir de GET
 * /api/microbot/game. Puro, testado em node.
 */
import type { GameItem, GameView, Pos } from '../api/microbot.ts';
import { SKILL_ORDER } from './conta.ts';

/** EquipmentInventorySlot do RuneLite para os slots do boneco da Conta. */
const EQUIP_SLOT: Record<string, string> = {
	HEAD: 'head',
	CAPE: 'cape',
	AMULET: 'neck',
	AMMO: 'ammo',
	WEAPON: 'weapon',
	BODY: 'body',
	SHIELD: 'shield',
	LEGS: 'legs',
	GLOVES: 'hands',
	BOOTS: 'feet',
	RING: 'ring'
};

/** As 28 casas do inventário, na ordem do jogo; casa vazia é null. */
export function invSlots(v: GameView | null): (GameItem | null)[] {
	const items = v?.inventory?.items ?? v?.observe?.inventory?.items ?? [];
	const slots: (GameItem | null)[] = Array.from({ length: 28 }, () => null);
	const loose: GameItem[] = [];
	for (const it of items) {
		if (typeof it.slot === 'number' && it.slot >= 0 && it.slot < 28 && !slots[it.slot])
			slots[it.slot] = it;
		else loose.push(it);
	}
	for (const it of loose) {
		const free = slots.indexOf(null);
		if (free >= 0) slots[free] = it;
	}
	return slots;
}

/** O equipamento no formato de equipmentLayout (conta.ts). */
export function equipItems(
	v: GameView | null
): { id: number; name: string; qty: number; slot?: string }[] {
	return (v?.observe?.equipment ?? []).map((e) => ({
		id: e.id,
		name: e.name,
		qty: e.quantity,
		slot: EQUIP_SLOT[e.slot] ?? e.slot.toLowerCase()
	}));
}

export interface SkillCell {
	skill: string;
	level: number;
	boosted: number;
	xp: number;
}

export function skillCells(v: GameView | null): SkillCell[] {
	const order = (s: string) => {
		const i = SKILL_ORDER.indexOf(s);
		return i < 0 ? 99 : i;
	};
	return (v?.skills?.skills ?? [])
		.map((s) => ({
			skill: s.name.toLowerCase(),
			level: s.level,
			boosted: s.boostedLevel,
			xp: s.xp
		}))
		.sort((a, b) => order(a.skill) - order(b.skill));
}

export type NearKind = 'npc' | 'objeto' | 'chão';

export interface NearRow {
	kind: NearKind;
	id: number;
	name: string;
	detail: string;
	distance: number | null;
	position?: Pos;
}

/** Distância em tiles como o RuneLite (WorldPoint.distanceTo): no mesmo plano, o maior eixo. */
export function tiles(from: Pos | undefined, to: Pos | undefined): number | null {
	if (!from || !to || from.plane !== to.plane) return null;
	return Math.max(Math.abs(from.x - to.x), Math.abs(from.y - to.y));
}

/** NPCs, objetos e itens do chão por perto, do mais próximo ao mais longe. */
export function nearRows(v: GameView | null): NearRow[] {
	const me = v?.observe?.player?.position;
	const rows: NearRow[] = [];
	for (const n of v?.observe?.npcs ?? [])
		rows.push({
			kind: 'npc',
			id: n.id,
			name: n.name,
			detail: n.combatLevel ? `nível ${n.combatLevel}` : '',
			// The Agent Server's `distance` is in local units (128 per tile): tiles come from the position.
			distance:
				tiles(me, n.position) ?? (n.distance !== undefined ? Math.round(n.distance / 128) : null),
			position: n.position
		});
	for (const o of v?.observe?.objects ?? [])
		rows.push({
			kind: 'objeto',
			id: o.id,
			name: o.name,
			detail: o.reachable === false ? 'fora de alcance' : '',
			distance: tiles(me, o.position),
			position: o.position
		});
	for (const g of v?.ground?.items ?? [])
		rows.push({
			kind: 'chão',
			id: g.id,
			name: g.name,
			detail: g.quantity > 1 ? `×${g.quantity}` : '',
			distance: tiles(me, g.position),
			position: g.position
		});
	return rows
		.filter((r) => r.name && r.name !== 'null')
		.sort(
			(a, b) => (a.distance ?? 999) - (b.distance ?? 999) || a.name.localeCompare(b.name, 'pt-BR')
		);
}

export function since(ms: number): string {
	const s = Math.floor(ms / 1000);
	if (s < 60) return `${s} s`;
	const m = Math.floor(s / 60);
	return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
}

export interface MachineRow {
	name: string;
	state: string;
	detail: string;
}

/** As state machines registradas no cliente (scripts rodando que as usam). */
export function machineRows(v: GameView | null): MachineRow[] {
	return (v?.machines?.scripts ?? []).map((m) => ({
		name: m.name,
		state: m.currentState ?? 'NOT_INITIALIZED',
		detail: [
			m.msInCurrentState !== undefined ? `há ${since(m.msInCurrentState)}` : '',
			m.loopCount ? `${m.loopCount} loops` : ''
		]
			.filter(Boolean)
			.join(' · ')
	}));
}

/** As últimas mensagens do jogo, a mais nova por último. */
export function chatLines(
	v: GameView | null,
	limit = 8
): { seq: number; at: string; text: string }[] {
	const hhmm = (ms: number) =>
		new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	return (v?.observe?.chat?.messages ?? [])
		.slice(-limit)
		.map((m) => ({ seq: m.seq, at: hhmm(m.at), text: m.message }));
}

/** A linha do diálogo aberto, ou null. */
export function dialogueLine(
	v: GameView | null
): { text: string; options: string[]; more: boolean } | null {
	const d = v?.observe?.dialogue;
	if (!d?.inDialogue) return null;
	return {
		text: d.question ?? d.dialogueText ?? '',
		options: d.options ?? [],
		more: !!d.hasContinue
	};
}

/**
 * Os scripts de loop com heartbeat recente, com os scripts não pausados. Plugins ligados mantêm
 * loops passivos (PIN do banco, ataque especial, antiban), então isso só avisa: uma ação manual
 * pode disputar o mouse com um script que esteja jogando.
 */
export function loopScripts(v: GameView | null): string[] {
	const o = v?.observe;
	if (!o || o.scriptsPaused) return [];
	return (o.scripts ?? [])
		.filter((h) => h.status !== 'NO_HEARTBEAT' && (h.stalledMs ?? 0) < 15000)
		.map((h) => h.scriptClassName.split('.').pop() ?? h.scriptClassName);
}

/** Ações comuns para sugerir (o jogo aceita qualquer uma do menu; o campo é livre). */
export const SUGGESTED: Record<'npc' | 'objeto', string[]> = {
	npc: ['Talk-to', 'Attack', 'Bank', 'Trade', 'Pickpocket'],
	objeto: ['Open', 'Bank', 'Chop down', 'Mine', 'Climb-up', 'Climb-down', 'Search']
};
