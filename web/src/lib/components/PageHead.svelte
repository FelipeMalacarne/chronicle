<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		eyebrow,
		title,
		description,
		actions
	}: { eyebrow: string; title: string; description?: string; actions?: Snippet } = $props();
</script>

<div class="head">
	<div>
		<p class="eyebrow">{eyebrow}</p>
		<h1 class="display">{title}</h1>
		{#if description}
			<p class="muted desc">{description}</p>
		{/if}
	</div>
	{#if actions}
		<div class="actions">{@render actions()}</div>
	{/if}
</div>

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		margin-bottom: var(--sp-6);
	}
	.eyebrow {
		margin-bottom: 6px;
	}
	.desc {
		margin-top: 6px;
		max-width: 60ch;
	}
	.actions {
		display: flex;
		gap: var(--sp-2);
		flex-wrap: wrap;
	}
	@media (max-width: 719px) {
		.head {
			flex-direction: column;
			align-items: stretch;
		}
		h1 {
			font-size: var(--fs-heading);
		}
	}
</style>
