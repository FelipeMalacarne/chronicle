<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		eyebrow,
		title,
		hint,
		children
	}: { eyebrow?: string; title: string; hint?: string; children?: Snippet } = $props();
</script>

<div class="empty" role="status">
	{#if eyebrow}<span class="eyebrow">{eyebrow}</span>{/if}
	<p class="title">{title}</p>
	{#if hint}<p class="muted hint">{hint}</p>{/if}
	{#if children}<div class="act">{@render children()}</div>{/if}
</div>

<style>
	.empty {
		background: var(--surface);
		border-radius: var(--r-panel);
		padding: 48px var(--sp-6);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--sp-2);
		text-align: center;
	}
	.title {
		font-family: var(--font-serif);
		font-size: var(--fs-heading);
	}
	.hint {
		font-size: var(--fs-control);
		max-width: 46ch;
	}
	.act {
		margin-top: var(--sp-2);
	}
</style>
