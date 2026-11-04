<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { statusView } from '#lib/progress/view';
	import type { BoardStep } from '#lib/progress/types';

	/**
	 * "Fixar outro" (V4AgoraOziris, revisão 3, ponto 5): busca passos da rota ativa com a posição
	 * relativa ao cursor. O pin vale só nesta rota e cai sozinho quando o passo concluir.
	 */
	let {
		guideLabel,
		steps,
		cursorId,
		onpin,
		onclose
	}: {
		guideLabel: string;
		steps: BoardStep[];
		cursorId: string | undefined;
		onpin: (stepId: string) => void;
		onclose: () => void;
	} = $props();

	let query = $state('');
	let selected = $state(0);
	let input: HTMLInputElement | undefined = $state();

	const cursorIndex = $derived(steps.findIndex((s) => s.id === cursorId));
	const results = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const list = steps
			.map((s, i) => ({ s, i }))
			.filter(({ s }) => !statusView(s.status).done && s.id !== cursorId)
			.filter(
				({ s }) => !q || s.ref.toLowerCase().includes(q) || s.title.toLowerCase().includes(q)
			);
		return list.slice(0, 8);
	});

	$effect(() => {
		input?.focus();
	});
	$effect(() => {
		if (selected >= results.length) selected = Math.max(0, results.length - 1);
	});

	function key(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
		else if (e.key === 'ArrowDown') {
			selected = Math.min(results.length - 1, selected + 1);
			e.preventDefault();
		} else if (e.key === 'ArrowUp') {
			selected = Math.max(0, selected - 1);
			e.preventDefault();
		} else if (e.key === 'Enter' && results[selected]) onpin(results[selected].s.id);
	}
	const where = (i: number) =>
		cursorIndex < 0 ? '' : i < cursorIndex ? ' · atrás do cursor' : ' · à frente';
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
	<div
		class="pal"
		role="dialog"
		aria-modal="true"
		aria-label="Fixar como passo atual"
		tabindex="-1"
		onkeydown={key}
	>
		<div class="in">
			<Glyph name="search" size={16} />
			<input
				bind:this={input}
				bind:value={query}
				placeholder="Fixar como passo atual · {guideLabel}"
				aria-label="Buscar passo"
			/>
			<span class="kbd">ESC</span>
		</div>
		<div class="list">
			{#each results as r, n (r.s.id)}
				<button
					class="it"
					class:it-on={n === selected}
					type="button"
					onclick={() => onpin(r.s.id)}
					onmouseenter={() => (selected = n)}
				>
					<span class="k">{r.s.ref}</span><span class="tx">{r.s.title}</span>
					<span class="m"
						>pos. {r.i + 1} · {statusView(r.s.status).label || 'pendente'}{where(r.i)}</span
					>
				</button>
			{:else}
				<p class="su empty">Nenhum passo pendente com esse texto.</p>
			{/each}
		</div>
		<div class="ft">
			<span
				>O pin vale só nesta rota (route_cursor · cause = pin) e cai sozinho quando o passo
				concluir.</span
			>
			<span class="push"><span class="kbd">↵</span> fixar</span>
		</div>
	</div>
</div>

<style>
	.list {
		padding: 6px 8px;
		display: flex;
		flex-direction: column;
		gap: 2px;
		max-height: 360px;
		overflow: auto;
	}
	.empty {
		margin: 8px;
		font-size: 13px;
	}
	.push {
		margin-left: auto;
		white-space: nowrap;
	}
</style>
