<script lang="ts">
	import type { GuideSummary } from '#lib/planner/types';

	let {
		guides,
		value,
		label,
		onchange,
		size = 'compact'
	}: {
		guides: GuideSummary[];
		value: string;
		label: string;
		onchange: (id: string) => void;
		size?: 'compact' | 'touch';
	} = $props();
</script>

<div class="seg {size}" role="group" aria-label={label}>
	{#each guides as g (g.id)}
		<button
			type="button"
			aria-pressed={g.id === value}
			title={g.title}
			onclick={() => g.id !== value && onchange(g.id)}
		>
			{g.label}
		</button>
	{/each}
</div>

<style>
	.seg {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border-radius: var(--r-button);
		background: var(--bg);
		border: 1px solid var(--divider);
	}
	button {
		height: 26px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-chip);
		font-size: var(--fs-control);
		font-weight: 500;
		color: var(--muted);
		white-space: nowrap;
	}
	button:hover:not([aria-pressed='true']) {
		color: var(--text);
	}
	button[aria-pressed='true'] {
		background: var(--accent);
		color: var(--accent-text);
	}
	.touch {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: minmax(0, 1fr);
		width: 100%;
		padding: 3px;
		border-radius: var(--r-tile);
		background: var(--surface);
		border: 0;
	}
	.touch button {
		height: 38px;
		border-radius: 9px;
	}
</style>
