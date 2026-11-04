<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import { visibleWindow } from '#lib/planner/compare';

	/**
	 * Lista de linhas com altura fixa. Acima de `threshold` itens só a janela visível vai
	 * para o DOM; "Lista completa" desliga a janela (fallback acessível, sem rolagem interna).
	 */
	let {
		items,
		row,
		label,
		rowHeight = 46,
		height = 360,
		threshold = 40
	}: {
		items: T[];
		row: Snippet<[T, number]>;
		label: string;
		rowHeight?: number;
		height?: number;
		threshold?: number;
	} = $props();

	let scrollTop = $state(0);
	let showAll = $state(false);
	const long = $derived(items.length > threshold);
	const virtual = $derived(long && !showAll);
	const win = $derived(
		virtual
			? visibleWindow({ count: items.length, rowHeight, scrollTop, viewport: height })
			: { start: 0, end: items.length, padTop: 0, padBottom: 0 }
	);
	const firstVisible = $derived(Math.min(items.length, Math.floor(scrollTop / rowHeight) + 1));
	const lastVisible = $derived(Math.min(items.length, Math.ceil((scrollTop + height) / rowHeight)));
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div
	class="scroller"
	class:virtual
	style:max-height={virtual ? `${height}px` : undefined}
	tabindex={virtual ? 0 : undefined}
	role={virtual ? 'region' : undefined}
	aria-label={virtual ? `${label} (lista virtualizada)` : undefined}
	onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
>
	<ol
		class="list"
		aria-label={label}
		style:padding-top="{win.padTop}px"
		style:padding-bottom="{win.padBottom}px"
	>
		{#each items.slice(win.start, win.end) as item, i (win.start + i)}
			{@render row(item, win.start + i)}
		{/each}
	</ol>
</div>
{#if long}
	<div class="foot">
		<span class="data" aria-live="polite">
			{virtual
				? `linhas ${firstVisible}–${lastVisible} de ${items.length} · lista virtualizada, role para carregar`
				: `lista completa · ${items.length} linhas, sem janela`}
		</span>
		<button
			class="btn btn-ghost btn-compact"
			type="button"
			aria-pressed={showAll}
			onclick={() => (showAll = !showAll)}
		>
			{showAll ? 'Voltar à janela' : 'Lista completa (acessível)'}
		</button>
	</div>
{/if}

<style>
	.virtual {
		overflow-y: auto;
		overscroll-behavior: contain;
		border-radius: var(--r-button);
	}
	.list {
		list-style: none;
		margin: 0;
		padding-left: 4px;
		padding-right: 4px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		flex-wrap: wrap;
		padding: 6px var(--sp-3) var(--sp-2);
		color: var(--subtle);
	}
	.foot .btn {
		margin-left: auto;
		height: 28px;
		font-size: var(--fs-label);
		color: var(--muted);
	}
</style>
