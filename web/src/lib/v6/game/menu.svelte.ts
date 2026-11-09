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

export async function pick(i: number, typed?: string) {
	const o = gameMenu.options[i];
	if (!o) return;
	if (o.confirm && gameMenu.armed !== i) {
		gameMenu.armed = i;
		return;
	}
	if (o.ask && !typed?.trim()) return;
	closeMenu();
	await runAction(o, o.ask ? { action: typed!.trim() } : undefined);
}
