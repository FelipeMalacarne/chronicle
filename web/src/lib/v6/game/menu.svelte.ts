import { microbotApi, MicrobotError } from '#lib/api/microbot';
import type { MenuOption } from '#lib/progress/jogo';

/*
 * O menu "Choose Option" do cockpit (doc 08): um só para o inventário, o banco, o chão, os NPCs e
 * os objetos, aberto onde o dono clicou. Executa a ação tipada pelo agent; as que perdem coisa
 * pedem um segundo clique e as de NPC/objeto pedem o nome da ação, porque o Agent Server não diz
 * as opções deles.
 */
export const gameMenu = $state({
	open: false,
	x: 0,
	y: 0,
	options: [] as MenuOption[],
	armed: -1,
	busy: false,
	error: null as string | null,
	/** Feitiço de item selecionado (Cast em alquimia): o próximo clique no inventário lança nele. */
	spell: null as { key: string; name: string } | null,
	/** Chamado depois de cada ação: o cockpit relê o jogo e a auditoria. */
	onDone: null as (() => void) | null
});

export function openMenu(e: MouseEvent, options: MenuOption[]) {
	e.preventDefault();
	e.stopPropagation();
	if (!options.length) return;
	gameMenu.x = e.clientX;
	gameMenu.y = e.clientY;
	gameMenu.options = options;
	gameMenu.armed = -1;
	gameMenu.open = true;
}

export function closeMenu() {
	gameMenu.open = false;
	gameMenu.armed = -1;
}

/** Roda uma ação fora do menu (diálogo, banco), com o mesmo erro e o mesmo onDone. */
export async function runAction(o: MenuOption, extra?: Record<string, unknown>) {
	if (!o.command || gameMenu.busy) return;
	gameMenu.busy = true;
	gameMenu.error = null;
	try {
		await microbotApi.gameAction(o.command, { ...o.body, ...extra });
	} catch (e) {
		gameMenu.error = e instanceof MicrobotError ? e.message : String(e);
	} finally {
		gameMenu.busy = false;
		gameMenu.onDone?.();
	}
}

/** O valor pedido no menu, no tipo do campo: um número positivo ou o nome de uma ação. */
export function askedValue(o: MenuOption, typed: string | undefined): string | number | null {
	const t = typed?.trim() ?? '';
	if (!t) return null;
	if (o.ask === 'number') {
		const n = Number(t.replace(/k$/i, '000').replace(/m$/i, '000000'));
		return Number.isInteger(n) && n > 0 ? n : null;
	}
	return t;
}

export async function pick(i: number, typed?: string) {
	const o = gameMenu.options[i];
	if (!o) return;
	if (o.confirm && gameMenu.armed !== i) {
		gameMenu.armed = i;
		return;
	}
	if (o.ask) {
		const v = askedValue(o, typed);
		if (v === null) return;
		closeMenu();
		await runAction(o, { [o.askField ?? 'action']: v });
		return;
	}
	closeMenu();
	if (!o.command && typeof o.body?.key === 'string') {
		// Cast num feitiço de item: seleciona e espera o item, como no jogo.
		gameMenu.spell = { key: o.body.key, name: o.target };
		return;
	}
	gameMenu.spell = null;
	await runAction(o);
}
