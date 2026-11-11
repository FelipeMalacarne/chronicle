/**
 * O painel "Fazer agora" do cockpit (doc 08, fase 4, entrega 2): rodar um plugin entre a atividade
 * principal e a volta. O agent para o que estiver marcado, roda o plugin até ele se desligar, volta
 * ao lugar e religa. Puro, testado em node.
 */
import type { Run, Triggers } from '../api/microbot.ts';
import type { PluginRow } from './plugins.ts';

/** O estado do run como a tela diz. */
export const RUN_LABEL: Record<Run['state'], string> = {
	waiting: 'esperando um ponto seguro',
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
		.sort((a, b) => Number(isCatSwap(b.className)) - Number(isCatSwap(a.className)));
}

export const isCatSwap = (className: string) => className.endsWith('.CatSwapPlugin');

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

/**
 * O que o vigia do gatilho fez da última vez (entrega 3): "disparou", "não disparou: o gatilho
 * estava desligado" ou o motivo do agent. Sem evento visto, nada.
 */
export function firingText(last: Triggers['last']): string | null {
	if (!last) return null;
	if (last.runId) return 'o filhote cresceu: disparou';
	if (last.skipped === 'trigger off') return 'o filhote cresceu com o gatilho desligado';
	if (last.skipped?.includes('another run')) return 'o filhote cresceu durante outro run';
	return `o filhote cresceu, não disparou: ${last.skipped ?? ''}`.trim();
}

/** O que o plugin rodado deixou no resultado: "+100 death runes · filhote seguindo". */
export function runOutcome(r: Run): string {
	const res = r.result ?? {};
	const parts: string[] = [];
	if (typeof res.deathRunesGained === 'number') parts.push(`+${res.deathRunesGained} death runes`);
	if (res.kittenFollows === true) parts.push('filhote seguindo');
	return parts.join(' · ');
}

/** Quanto o run levou: "4 min", "menos de 1 min", "1 h 05 min". Sem fim, nada. */
export function runDuration(r: Run): string | null {
	if (!r.finishedAt) return null;
	const m = Math.round((Date.parse(r.finishedAt) - Date.parse(r.startedAt)) / 60000);
	if (m < 1) return 'menos de 1 min';
	if (m < 60) return `${m} min`;
	return `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, '0')} min`;
}

/** Quando o run começou, pelo dia da máquina do navegador: "hoje 14:02", "ontem 23:10", "08/11 09:15". */
export function runWhen(iso: string, now: Date = new Date()): string {
	const d = new Date(iso);
	const hm = d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
	const diff = Math.round((day(now) - day(d)) / 86400000);
	if (diff === 0) return `hoje ${hm}`;
	if (diff === 1) return `ontem ${hm}`;
	return `${d.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' })} ${hm}`;
}
