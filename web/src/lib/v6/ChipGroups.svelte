<script lang="ts">
	import ObjectiveChip from './ObjectiveChip.svelte';
	import type { Chip } from '#lib/progress/view';
	import type { Role } from '#lib/progress/types';

	/**
	 * Chips por papel, com o papel como rótulo do grupo (V6Fundamentos, átomo 1: uma linha por
	 * grupo). O rótulo abre a sua linha, então os chips que quebram nunca ficam sem rótulo.
	 */
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
	{#each groups as g (g.role)}
		<div class="grp">
			<span class="lbl">{g.label}</span>
			{#each g.chips as c (c.key)}<ObjectiveChip chip={c} />{/each}
		</div>
	{/each}
</div>

<style>
	.cg {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.grp {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.lbl {
		margin-right: 4px;
	}
</style>
