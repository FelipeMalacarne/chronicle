import { invalidate } from './resource.svelte';
import { refreshSummary, session } from '../progress/session.svelte';
import { loadMe } from '../progress/me.svelte';
import {
	detectionToast,
	parseLiveEvent,
	tagsFor,
	type LiveEvent,
	type LiveSource,
	type LiveTag
} from '../progress/live';

/**
 * Tempo real (docs/04, seção 6.6): um EventSource em /api/events por conta. Cada evento é só
 * um sinal; os recursos com a etiqueta certa se refazem pela API de sempre (invalidate) e o
 * resumo da sessão também. Como as outras rotas por conta, existe em AUTH_MODE=dev e, em oidc,
 * com o cookie de sessão (o EventSource o manda na mesma origem); sem sessão o servidor responde
 * 401 e o EventSource desiste sozinho, enquanto as outras chamadas levam ao login.
 */
export const live = $state({
	connected: false,
	last: null as LiveEvent | null,
	/** Toast de detecção (plugin ou hiscores mudou passos); `id` remonta o toast a cada um. */
	toast: null as { id: number; title: string; text: string } | null
});

/** Eventos que chegam juntos (sync manda progress + sync) viram uma releitura só. */
const COALESCE_MS = 250;
/** Aba escondida por mais que isso fecha o stream; ao voltar, reabre e relê tudo. */
const HIDDEN_CLOSE_MS = 60_000;

const pending: Record<LiveTag, boolean> = { progress: false, sync: false };
let timer: ReturnType<typeof setTimeout> | undefined;
let toastSeq = 0;
const quietUntil: Partial<Record<LiveSource, number>> = {};

function schedule(tags: LiveTag[]) {
	for (const t of tags) pending[t] = true;
	if (timer) return;
	timer = setTimeout(() => {
		timer = undefined;
		const due = (Object.keys(pending) as LiveTag[]).filter((t) => pending[t]);
		for (const t of due) pending[t] = false;
		invalidate(...due);
		void refreshSummary();
		void loadMe(session.account);
	}, COALESCE_MS);
}

/**
 * Silencia o toast de detecção de uma fonte por um tempo: a tela que pediu o sync manual já
 * mostra o próprio resultado, e o evento pode chegar antes ou depois da resposta.
 */
export function quietDetection(source: LiveSource, ms = 15_000) {
	quietUntil[source] = Date.now() + ms;
}

export function dismissDetection() {
	live.toast = null;
}

/** Abre o stream da conta; devolve o encerramento (use como retorno de um $effect). */
export function connectLive(account: string): () => void {
	if (typeof EventSource === 'undefined') return () => {};
	let es: EventSource | null = null;
	let openedBefore = false;
	let hiddenTimer: ReturnType<typeof setTimeout> | undefined;

	const onEvent = (msg: MessageEvent<string>) => {
		const e = parseLiveEvent(msg.data);
		if (!e || e.account !== account) return;
		live.last = e;
		schedule(tagsFor(e));
		const t = detectionToast(e);
		if (t && Date.now() >= (quietUntil[e.source] ?? 0)) live.toast = { id: ++toastSeq, ...t };
	};
	const open = () => {
		const source = new EventSource(`/api/events?account=${encodeURIComponent(account)}`);
		source.onopen = () => {
			live.connected = true;
			// Reconexão: o que mudou enquanto o stream esteve fora não chegou; relê tudo.
			if (openedBefore) schedule(['progress', 'sync']);
			openedBefore = true;
		};
		source.onerror = () => {
			live.connected = false;
		};
		source.addEventListener('progress', onEvent);
		source.addEventListener('sync', onEvent);
		es = source;
	};
	const close = () => {
		es?.close();
		es = null;
		live.connected = false;
	};
	const onVisibility = () => {
		clearTimeout(hiddenTimer);
		if (document.hidden) hiddenTimer = setTimeout(close, HIDDEN_CLOSE_MS);
		else if (!es) open();
	};

	open();
	document.addEventListener('visibilitychange', onVisibility);
	return () => {
		clearTimeout(hiddenTimer);
		document.removeEventListener('visibilitychange', onVisibility);
		close();
	};
}
