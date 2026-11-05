import { untrack } from 'svelte';
import type { LiveTag } from '../progress/live';

/**
 * Recursos vivos por etiqueta: `invalidate(tag)` refaz cada um em silêncio. Fora do grafo
 * reativo de propósito (só funções de releitura), por isso listas simples.
 */
const registry: Record<LiveTag, (() => void)[]> = { progress: [], sync: [] };

/**
 * Refaz em silêncio todo recurso aberto com alguma das etiquetas (eventos do servidor, docs/04
 * seção 6.5). Sem `loading`, sem trocar o dado por null: a tela mantém linhas abertas, rolagem
 * e diálogos; só o dado muda quando a resposta chega.
 */
export function invalidate(...tags: LiveTag[]) {
	const run: (() => void)[] = [];
	for (const t of tags) for (const fn of registry[t]) if (!run.includes(fn)) run.push(fn);
	for (const fn of run) fn();
}

/**
 * Recurso assíncrono mínimo para páginas: data/error/loading + reload().
 * O `fetcher` roda dentro de um `$effect`: o estado reativo que ele lê (ex.: o slug
 * da rota) vira dependência e dispara um novo fetch quando muda. Chame no init do componente.
 * `tags` liga o recurso aos eventos do servidor: `invalidate('progress')` o refaz em silêncio.
 */
export function createResource<T>(fetcher: () => Promise<T>, opts: { tags?: LiveTag[] } = {}) {
	let data = $state<T | null>(null);
	let error = $state<Error | null>(null);
	let loading = $state(true);
	let seq = 0;

	function run(promise: Promise<T>, silent = false) {
		const my = ++seq;
		if (!silent) loading = true;
		promise.then(
			(d) => {
				if (my !== seq) return;
				data = d;
				error = null;
				loading = false;
			},
			(e: unknown) => {
				if (my !== seq) return;
				// Uma releitura silenciosa que falha mantém o dado; o indicador global já marcou a falha.
				if (!silent || data === null) error = e instanceof Error ? e : new Error(String(e));
				loading = false;
			}
		);
	}

	$effect(() => {
		// Só as leituras feitas pelo fetcher são rastreadas; o resto fica fora do grafo.
		const promise = fetcher();
		untrack(() => run(promise));
	});

	const refresh = () => untrack(() => run(fetcher(), true));
	const tags = opts.tags ?? [];
	$effect(() => {
		for (const t of tags) registry[t].push(refresh);
		return () => {
			for (const t of tags) registry[t] = registry[t].filter((fn) => fn !== refresh);
		};
	});

	return {
		get data() {
			return data;
		},
		get error() {
			return error;
		},
		get loading() {
			return loading;
		},
		reload: () => untrack(() => run(fetcher())),
		/** Refaz sem marcar `loading` (eventos do servidor). */
		refresh,
		/** Troca o dado pela resposta de uma escrita (PATCH devolve o board novo). */
		set: (d: T) => {
			seq++;
			data = d;
			error = null;
			loading = false;
		}
	};
}
