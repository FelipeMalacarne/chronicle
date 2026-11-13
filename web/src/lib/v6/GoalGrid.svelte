<script lang="ts">
	import { entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { goalGroups } from '#lib/progress/objetivo';
	import type { BoardObjective } from '#lib/progress/types';
	import type { Chip } from '#lib/progress/view';

	/**
	 * Os objetivos de um passo como a v7 mostra (V7Fundamentos, "Objetivo com progresso"; V7Rota1440):
	 * um grupo por papel, o rótulo abrindo a linha, e cada objetivo com o quanto falta ("240 /
	 * 1.250") quando o board sabe o número. Abre o painel da entidade por cima da tela.
	 */
	let {
		chips,
		objectives
	}: {
		chips: Chip[];
		objectives: Pick<BoardObjective, 'key' | 'role' | 'observed'>[] | undefined;
	} = $props();
	const groups = $derived(goalGroups(chips, objectives));
</script>

<div class="goals">
	{#each groups as g (g.role)}
		<span class="gk">{g.label}</span>
		<div class="gv">
			{#each g.goals as o (o.key)}
				{@const icon = iconFor(o.ref)}
				<button
					class="goal7 {o.cls}"
					class:sel={!!o.ref && entityPanel.ref === o.ref}
					type="button"
					title={o.title}
					disabled={!o.ref}
					aria-haspopup="dialog"
					onclick={() => openEntity(o.ref)}
					>{#if icon}<img src={icon} alt="" />{:else}<span class="gi"></span>{/if}<b>{o.name}</b
					><span class="pg7"><i><span style="width: {o.pct}%"></span></i>{o.text}</span></button
				>
			{/each}
		</div>
	{/each}
</div>

<style>
	.goal7:disabled {
		cursor: default;
	}
	.gi {
		width: 22px;
		height: 22px;
	}
</style>
