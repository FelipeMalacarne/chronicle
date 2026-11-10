/**
 * O painel "Fazer agora" do cockpit (doc 08, fase 4, entrega 2): rodar um plugin entre a atividade
 * principal e a volta. O agent para o que estiver marcado, roda o plugin até ele se desligar, volta
 * ao lugar e religa. Puro, testado em node.
 */
import type { Run } from '../api/microbot.ts';
import type { PluginRow } from './plugins.ts';

/** O estado do run como a tela diz. */
export const RUN_LABEL: Record<Run['state'], string> = {
	stopping: 'parando o principal',
	running: 'rodando',
	returning: 'voltando ao lugar',
	restarting: 'religando o principal',
	done: 'feito',
	failed: 'falhou',
	canceled: 'cancelado'
};

export const runFinal = (r: Run | null | undefined) =>
	!!r && (r.state === 'done' || r.state === 'failed' || r.state === 'canceled');

/** Os plugins que dá para rodar: os seus e os do Hub, fora os do núcleo. O Cat Swap vem primeiro. */
export function runnable(rows: PluginRow[]): PluginRow[] {
	return rows
		.filter((r) => !r.protected && r.source !== 'cliente')
		.sort((a, b) => Number(isCatSwap(b)) - Number(isCatSwap(a)));
}

const isCatSwap = (r: PluginRow) => r.className.endsWith('.CatSwapPlugin');

/**
 * Os plugins ligados que o run para e religa: os seus e os do Hub, menos o próprio. O do gato
 * (Kitten) fica ligado por padrão: ele precisa ver a mensagem "Gertrude gives you another kitten."
 * para zerar o crescimento, e plugin parado não recebe mensagem.
 */
export function stoppable(rows: PluginRow[], run: string): { row: PluginRow; checked: boolean }[] {
	return rows
		.filter((r) => r.active && !r.protected && r.source !== 'cliente' && r.className !== run)
		.map((row) => ({ row, checked: !/kitten/i.test(row.className) }));
}
