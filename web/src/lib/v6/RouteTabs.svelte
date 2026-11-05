<script lang="ts">
	import { resolve } from '$app/paths';
	import { session } from '#lib/progress/session.svelte';
	import { routeLabel } from '#lib/progress/view';

	/**
	 * Abas de Rotas (V6Rota1440, V6RotaBruh1440, V6Comparar1440): uma por rota, com feitos / total,
	 * onde a inativa estaria e Comparar. Cada rota guarda o próprio cursor.
	 */
	let { current }: { current: string | 'compare' } = $props();
	const ORDER = ['oziris-main', 'bruhsailer-main'];
	const rank = (g: string) => (ORDER.includes(g) ? ORDER.indexOf(g) : ORDER.length);
	const routes = $derived(
		[...(session.summary?.routes ?? [])].sort((a, b) => rank(a.guide) - rank(b.guide))
	);
</script>

<div class="tabsrow">
	<div class="seg" role="tablist" aria-label="Rota">
		{#each routes as r (r.guide)}
			{@const on = current === r.guide}
			<a
				class="tab"
				class:on
				role="tab"
				aria-selected={on}
				href={resolve('/rotas/[guide]', { guide: r.guide })}
				>{#if r.active}<span class="st st-cur dot" aria-hidden="true"></span>{/if}{routeLabel(
					r.guide,
					r.title
				)}<span class="n"
					>{r.counts.done} / {r.counts.total}{#if !r.active && r.cursor_ref && !on}
						· em {r.cursor_ref}{/if}{#if r.active && !on}
						· ativa{/if}</span
				></a
			>
		{/each}
		<a
			class="tab"
			class:on={current === 'compare'}
			role="tab"
			aria-selected={current === 'compare'}
			href={resolve('/compare')}>Comparar</a
		>
	</div>
	<span class="su hint">cada rota guarda o próprio lugar; o que você fez vale nas duas</span>
</div>

<style>
	.tabsrow {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
	}
	.seg {
		align-self: flex-start;
		max-width: 100%;
		overflow-x: auto;
	}
	.tab {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		height: 38px;
		padding: 0 14px;
		text-decoration: none;
		color: var(--ink-muted);
		background: var(--elevated);
		font:
			500 13px Inter,
			system-ui,
			sans-serif;
		white-space: nowrap;
	}
	.tab + .tab {
		border-left: 1px solid var(--border);
	}
	.tab.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.n {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.on .n {
		color: var(--on-frame-muted);
	}
	.dot {
		width: 10px;
		height: 10px;
		box-shadow: inset 0 0 0 2px var(--elevated);
	}
	.on .dot {
		box-shadow: inset 0 0 0 2px var(--frame);
	}
	.hint {
		font-size: 12px;
	}
</style>
