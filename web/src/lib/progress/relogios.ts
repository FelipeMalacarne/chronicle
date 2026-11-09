/**
 * O cartão Relógios do cockpit (doc 08, fase 4, entrega 1): os relógios do Time Tracking do
 * RuneLite, do que está pronto ao que falta mais. Puro, testado em node.
 */
import type { Timer } from '../api/microbot.ts';

export type ClockTone = 'ready' | 'growing' | 'idle';

export interface ClockRow {
	key: string;
	label: string;
	tone: ClockTone;
	/** "pronto", "em 23 min", "plantado", "vazio". */
	when: string;
	/** Quando fica pronto, em segundos (para o title com a hora). */
	at?: number;
}

/** "<1 min", "23 min", "2 h 05", "1 d 3 h". */
export function duration(seconds: number): string {
	const m = Math.ceil(seconds / 60);
	if (m < 1) return '<1 min';
	if (m < 60) return `${m} min`;
	const h = Math.floor(m / 60);
	if (h < 24) return `${h} h ${String(m % 60).padStart(2, '0')}`;
	return `${Math.floor(h / 24)} d ${h % 24} h`;
}

const ORDER: Record<Timer['state'], number> = {
	COMPLETED: 0,
	IN_PROGRESS: 1,
	OCCUPIED: 2,
	EMPTY: 3,
	UNKNOWN: 4
};

/**
 * Os relógios em linhas, no instante now (segundos): o que está pronto primeiro, depois o que
 * cresce pelo que fica pronto antes. Os que o Time Tracking nunca viu neste perfil vão para
 * `unseen` (desconhecido não é vazio).
 */
export function clockRows(timers: Timer[], now: number): { rows: ClockRow[]; unseen: string[] } {
	const rows: ClockRow[] = [];
	const unseen: string[] = [];
	const sorted = [...timers].sort(
		(a, b) =>
			ORDER[a.state] - ORDER[b.state] ||
			(a.readyAt ?? Infinity) - (b.readyAt ?? Infinity) ||
			a.label.localeCompare(b.label)
	);
	for (const t of sorted) {
		const base = { key: t.key, label: t.label, at: t.readyAt };
		// Um "crescendo" cuja hora já passou está pronto: o Time Tracking só vira o estado no próximo tick dele.
		const done =
			t.state === 'COMPLETED' ||
			(t.state === 'IN_PROGRESS' && t.readyAt !== undefined && t.readyAt <= now);
		if (done) rows.push({ ...base, tone: 'ready', when: 'pronto' });
		else if (t.state === 'IN_PROGRESS')
			rows.push({
				...base,
				tone: 'growing',
				when: t.readyAt ? `em ${duration(t.readyAt - now)}` : 'crescendo'
			});
		else if (t.state === 'OCCUPIED') rows.push({ ...base, tone: 'idle', when: 'plantado' });
		else if (t.state === 'EMPTY') rows.push({ ...base, tone: 'idle', when: 'vazio' });
		else unseen.push(t.label);
	}
	// Prontos que viraram prontos agora sobem para o topo.
	rows.sort((a, b) => (a.tone === 'ready' ? 0 : 1) - (b.tone === 'ready' ? 0 : 1));
	return { rows, unseen };
}
