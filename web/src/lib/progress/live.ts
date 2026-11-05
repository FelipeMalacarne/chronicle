/**
 * Tempo real (docs/04, seção 6.5): o servidor manda sinais leves por SSE e a tela refaz as
 * leituras pela API de sempre. Aqui fica a parte pura: validar a mensagem, decidir o que
 * invalidar e o texto do toast de detecção (V4AgoraOziris: "1.1.72 detectado · 50 Firemaking
 * via hiscores · 20:14").
 */

/** Etiquetas de invalidação dos recursos (createResource). */
export type LiveTag = 'progress' | 'sync';

export type LiveSource = 'plugin' | 'hiscores' | 'owner';

export interface LiveEvent {
	type: 'progress' | 'sync';
	account: string;
	source: LiveSource;
	guides: string[];
	steps_changed: number;
	status?: string;
	at: string;
}

const SOURCES: readonly string[] = ['plugin', 'hiscores', 'owner'];

/** Lê o `data:` de um evento `progress` ou `sync`; qualquer outra coisa vira null. */
export function parseLiveEvent(data: string): LiveEvent | null {
	let raw: unknown;
	try {
		raw = JSON.parse(data);
	} catch {
		return null;
	}
	if (!raw || typeof raw !== 'object') return null;
	const o = raw as Record<string, unknown>;
	if (o.type !== 'progress' && o.type !== 'sync') return null;
	if (typeof o.account !== 'string' || typeof o.source !== 'string') return null;
	if (!SOURCES.includes(o.source)) return null;
	const steps = typeof o.steps_changed === 'number' && o.steps_changed > 0 ? o.steps_changed : 0;
	return {
		type: o.type,
		account: o.account,
		source: o.source as LiveSource,
		guides: Array.isArray(o.guides)
			? o.guides.filter((g): g is string => typeof g === 'string')
			: [],
		steps_changed: steps,
		status: typeof o.status === 'string' ? o.status : undefined,
		at: typeof o.at === 'string' ? o.at : ''
	};
}

/** O que um evento invalida: progresso refaz board, metas, mundo; sync refaz o estado da fonte. */
export function tagsFor(e: LiveEvent): LiveTag[] {
	return e.type === 'progress' ? ['progress'] : ['sync'];
}

const hhmm = (at: string): string => {
	const d = new Date(at);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

/**
 * Toast de detecção: só quando o motor mudou passos por uma fonte automática (plugin ou
 * hiscores). Marcas do dono já têm o próprio toast na tela que as fez.
 */
export function detectionToast(
	e: LiveEvent,
	time: (at: string) => string = hhmm
): { title: string; text: string } | null {
	if (e.type !== 'progress' || e.source === 'owner' || e.steps_changed <= 0) return null;
	const n = e.steps_changed;
	const title = n === 1 ? '1 passo detectado' : `${n} passos detectados`;
	const at = time(e.at);
	return { title, text: at ? `via ${e.source} · ${at}` : `via ${e.source}` };
}
