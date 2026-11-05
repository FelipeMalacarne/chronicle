<script lang="ts">
	import ObjectiveChip from './ObjectiveChip.svelte';
	import type { Chip } from '#lib/progress/view';
	import type { Role } from '#lib/progress/types';

	/** Chips por papel, com o papel como rótulo do grupo (V6Fundamentos, átomo 1). */
	let { chips }: { chips: Chip[] } = $props();
	const ORDER: [Role, string][] = [
		['completes', 'Conclui'],
		['starts', 'Inicia'],
		['requires', 'Requer'],
		['uses', 'Usa']
	];
	const groups = $derived(
		ORDER.map(([role, label]) => ({
			role,
			label,
			chips: chips.filter((c) => c.role === role)
		})).filter((g) => g.chips.length > 0)
	);
</script>

<div class="cg">
	{#each groups as g, i (g.role)}
		<span class="lbl" class:gap={i > 0}>{g.label}</span>
		{#each g.chips as c (c.key)}<ObjectiveChip chip={c} />{/each}
	{/each}
</div>

<style>
	.cg {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.lbl {
		margin-right: 4px;
	}
	.gap {
		margin-left: 8px;
	}
</style>
