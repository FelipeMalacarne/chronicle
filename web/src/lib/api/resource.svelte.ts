import { untrack } from 'svelte';

/**
 * Recurso assíncrono mínimo para páginas: data/error/loading + reload().
 * O `fetcher` roda dentro de um `$effect`: o estado reativo que ele lê (ex.: o slug
 * da rota) vira dependência e dispara um novo fetch quando muda. Chame no init do componente.
 */
export function createResource<T>(fetcher: () => Promise<T>) {
	let data = $state<T | null>(null);
	let error = $state<Error | null>(null);
	let loading = $state(true);
	let seq = 0;

	function run(promise: Promise<T>) {
		const my = ++seq;
		loading = true;
		promise.then(
			(d) => {
				if (my !== seq) return;
				data = d;
				error = null;
				loading = false;
			},
			(e: unknown) => {
				if (my !== seq) return;
				error = e instanceof Error ? e : new Error(String(e));
				loading = false;
			}
		);
	}

	$effect(() => {
		// Só as leituras feitas pelo fetcher são rastreadas; o resto fica fora do grafo.
		const promise = fetcher();
		untrack(() => run(promise));
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
		/** Troca o dado pela resposta de uma escrita (PATCH devolve o board novo). */
		set: (d: T) => {
			seq++;
			data = d;
			error = null;
			loading = false;
		}
	};
}
