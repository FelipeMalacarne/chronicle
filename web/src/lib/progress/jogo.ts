/**
 * Automação › Cockpit, o jogo visto pelo agent (doc 08): inventário, equipamento, skills, banco,
 * diálogo, o que está por perto, as state machines e o chat do jogo, a partir de GET
 * /api/microbot/game. Puro, testado em node.
 */
import type { GameCommand, GameItem, GameView, Pos } from '../api/microbot.ts';
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
	/** As opções do jogo, quando a ponte de controle as dá (NPC e objeto). */
	actions?: string[];
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
	const ent = v?.bridge?.entities;
	// Com a ponte, NPCs e objetos vêm dela, com as opções reais do menu.
	for (const n of ent?.npcs ?? [])
		rows.push({
			kind: 'npc',
			id: n.id,
			name: n.name,
			detail: n.combatLevel ? `nível ${n.combatLevel}` : '',
			distance: tiles(me, n.position),
			position: n.position,
			actions: n.actions
		});
	for (const o of ent?.objects ?? [])
		rows.push({
			kind: 'objeto',
			id: o.id,
			name: o.name,
			detail: '',
			distance: tiles(me, o.position),
			position: o.position,
			actions: o.actions
		});
	for (const n of ent ? [] : (v?.observe?.npcs ?? []))
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
	for (const o of ent ? [] : (v?.observe?.objects ?? []))
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

/** Uma opção do menu "Choose Option", como o jogo mostra: a ação e o alvo na cor do tipo. */
export interface MenuOption {
	label: string;
	target: string;
	tone: 'item' | 'npc' | 'object';
	command?: GameCommand;
	body?: Record<string, unknown>;
	/** Perde coisa (dropar, depositar tudo): pede um segundo clique. */
	confirm?: boolean;
	/**
	 * Pede um valor ao dono antes de agir: o nome da ação (NPC e objeto sem a ponte) ou um número
	 * (Deposit-X, Withdraw-X), que vai no campo `askField` do corpo.
	 */
	ask?: 'action' | 'number';
	askField?: string;
}

/**
 * As opções de um item do inventário: exatamente as do jogo (GET /inventory traz as ações do item).
 * Com o banco aberto o jogo troca o menu pelo de depósito; o Microbot procura a ação no widget do
 * inventário do banco, então Deposit-1, 5, 10 e All passam pelo mesmo inventory_use (Deposit-X
 * pede um número no jogo e fica para a ponte).
 */
export function itemMenu(it: GameItem, bankOpen: boolean, bridge = false): MenuOption[] {
	const base = { target: it.name, tone: 'item' as const };
	if (bankOpen) {
		const out: MenuOption[] = ['Deposit-1', 'Deposit-5', 'Deposit-10', 'Deposit-All'].map((a) => ({
			...base,
			label: a,
			command: 'inventory_use' as const,
			body: { name: it.name, action: a },
			confirm: a === 'Deposit-All'
		}));
		if (bridge)
			out.splice(3, 0, {
				...base,
				label: 'Deposit-X',
				command: 'bank_deposit_x',
				body: { name: it.name },
				ask: 'number',
				askField: 'amount'
			});
		return out;
	}
	return (it.actions ?? []).map((a) =>
		a === 'Drop'
			? {
					...base,
					label: a,
					command: 'inventory_drop' as const,
					body: { name: it.name },
					confirm: true
				}
			: { ...base, label: a, command: 'inventory_use' as const, body: { name: it.name, action: a } }
	);
}

/** As opções de um item do banco que o agent sabe fazer: Withdraw-1, 5, 10 e All. */
export function bankMenu(it: GameItem): MenuOption[] {
	const base = { target: it.name, tone: 'item' as const, command: 'bank_withdraw' as const };
	return [
		...[1, 5, 10].map((n) => ({
			...base,
			label: `Withdraw-${n}`,
			body: { name: it.name, quantity: n }
		})),
		{
			...base,
			label: 'Withdraw-X',
			body: { name: it.name },
			ask: 'number' as const,
			askField: 'quantity'
		},
		{ ...base, label: 'Withdraw-All', body: { name: it.name, all: true } }
	];
}

/**
 * NPC, objeto ou item do chão. Do chão o jogo tem Take; de NPC e objeto o Agent Server não diz as
 * opções, então em vez de inventar o menu pede a ação ao dono. Walk here vale para todos.
 */
export function nearMenu(r: NearRow): MenuOption[] {
	const tone = r.kind === 'npc' ? 'npc' : r.kind === 'objeto' ? 'object' : 'item';
	const out: MenuOption[] = [];
	const command = r.kind === 'npc' ? ('npc_interact' as const) : ('object_interact' as const);
	if (r.kind === 'chão')
		out.push({ label: 'Take', target: r.name, tone, command: 'ground_pickup', body: { id: r.id } });
	else if (r.actions)
		for (const a of r.actions)
			out.push({ label: a, target: r.name, tone, command, body: { id: r.id, action: a } });
	else
		out.push({
			label: 'Ação…',
			target: r.name,
			tone,
			ask: 'action',
			askField: 'action',
			command,
			body: { id: r.id }
		});
	if (r.position)
		out.push({ label: 'Walk here', target: '', tone, command: 'walk', body: { ...r.position } });
	return out;
}

/** A quantidade como o jogo escreve no slot: amarelo até 99.999, branco em K, verde em M. */
export function qtyLabel(q: number): { text: string; tone: 'y' | 'w' | 'g' } {
	if (q >= 10_000_000) return { text: `${Math.floor(q / 1_000_000)}M`, tone: 'g' };
	if (q >= 100_000) return { text: `${Math.floor(q / 1000)}K`, tone: 'w' };
	return { text: String(q), tone: 'y' };
}

/** O menu de um item vestido: as ações dele pela ponte (Remove, Rub, Operate). */
export function equipMenu(v: GameView | null, name: string): MenuOption[] {
	const item = v?.bridge?.equipment?.items.find((i) => i.name === name);
	return (item?.actions ?? []).map((a) => ({
		label: a,
		target: name,
		tone: 'item' as const,
		command: 'equipment_use' as const,
		body: { name, action: a }
	}));
}

export interface PrayerCell {
	key: string;
	name: string;
	level: number;
	unlocked: boolean;
	active: boolean;
}

/** As orações na ordem do livro (o enum do cliente segue o jogo). */
export function prayerCells(v: GameView | null): PrayerCell[] {
	return v?.bridge?.prayers?.prayers ?? [];
}

export function prayerMenu(p: PrayerCell): MenuOption[] {
	if (!p.unlocked) return [];
	return [
		{
			label: p.active ? 'Deactivate' : 'Activate',
			target: p.name,
			tone: 'object',
			command: 'prayer_toggle',
			body: { key: p.key, on: !p.active }
		}
	];
}

export interface SpellCell {
	key: string;
	name: string;
	level: number;
	castable: boolean;
	onItem: boolean;
}

/** Os feitiços do livro atual, por nível como o jogo os ordena. */
export function spellCells(v: GameView | null): SpellCell[] {
	return [...(v?.bridge?.spells?.spells ?? [])].sort((a, b) => a.level - b.level);
}

/**
 * O menu de um feitiço: Cast. Feitiço de item (alquimia) só seleciona, sem comando; o próximo
 * clique num item do inventário lança nele, como no jogo.
 */
export function spellMenu(s: SpellCell): MenuOption[] {
	if (!s.castable) return [];
	if (s.onItem) return [{ label: 'Cast', target: s.name, tone: 'object', body: { key: s.key } }];
	return [
		{ label: 'Cast', target: s.name, tone: 'object', command: 'spell_cast', body: { key: s.key } }
	];
}

/** Com um feitiço de item selecionado, o menu do item é só "Cast <feitiço> -> <item>". */
export function castOnMenu(spell: { key: string; name: string }, it: GameItem): MenuOption[] {
	return [
		{
			label: `Cast ${spell.name} ->`,
			target: it.name,
			tone: 'item',
			command: 'spell_cast',
			body: { key: spell.key, item: it.name }
		}
	];
}
