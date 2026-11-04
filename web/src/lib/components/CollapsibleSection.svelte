<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * Bloco recolhível: botão com aria-expanded no cabeçalho; o corpo só é montado aberto,
	 * então seções fechadas não custam DOM.
	 */
	let {
		id,
		title,
		code,
		done,
		count,
		meta,
		variant = 'section',
		open = true,
		onchange,
		children
	}: {
		id: string;
		title: string;
		code?: string;
		done?: number;
		count?: number;
		meta?: string;
		variant?: 'section' | 'segment';
		open?: boolean;
		onchange?: (open: boolean) => void;
		children: Snippet;
	} = $props();

	const pct = $derived(count ? Math.round(((done ?? 0) / count) * 100) : 0);
</script>

<section class="box {variant}" aria-labelledby="{id}-h">
	<h2 id="{id}-h" class="h">
		<button
			type="button"
			aria-expanded={open}
			aria-controls="{id}-body"
			onclick={() => onchange?.(!open)}
		>
			<span class="chev" class:open aria-hidden="true">›</span>
			{#if code}<span class="data code">{code}</span>{/if}
			<span class="t">{title}</span>
			{#if variant === 'section' && count !== undefined}
				<span class="bar" aria-hidden="true"><span style:width="{pct}%"></span></span>
				<span class="data c">{done ?? 0}/{count}</span>
			{/if}
			{#if meta}<span class="data c">{meta}</span>{/if}
		</button>
	</h2>
	<div id="{id}-body" class="body" hidden={!open}>
		{#if open}{@render children()}{/if}
	</div>
</section>

<style>
	.box {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 6px;
		border-radius: var(--r-card);
	}
	.section {
		background: var(--surface);
	}
	.segment {
		background: var(--mantle);
		border: 1px solid var(--divider);
		padding: 4px;
	}
	.h {
		margin: 0;
		font-family: var(--font-sans);
		font-size: var(--fs-body);
		font-weight: 500;
	}
	button {
		width: 100%;
		min-height: 44px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-button);
		display: flex;
		align-items: center;
		gap: 10px;
		text-align: left;
		font-size: var(--fs-control);
		font-weight: 500;
		color: var(--text);
	}
	.segment button {
		min-height: 40px;
		font-size: var(--fs-label);
		font-weight: 400;
		color: var(--muted);
	}
	button:hover {
		background: color-mix(in srgb, var(--elevated) 55%, transparent);
	}
	.chev {
		display: inline-block;
		width: 12px;
		color: var(--muted);
		transition: transform 120ms ease;
	}
	.chev.open {
		transform: rotate(90deg);
	}
	.code {
		color: var(--accent);
		width: 32px;
		flex: none;
	}
	.t {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.bar {
		width: 120px;
		height: 4px;
		border-radius: 2px;
		background: var(--elevated);
		overflow: hidden;
		flex: none;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
	}
	.c {
		color: var(--muted);
		text-align: right;
		flex: none;
	}
	.section .c {
		min-width: 64px;
	}
	@media (max-width: 719px) {
		.bar {
			width: 56px;
		}
	}
</style>
