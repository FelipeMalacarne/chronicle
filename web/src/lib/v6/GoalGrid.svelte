<script lang="ts">
	import { entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { goalGroups } from '#lib/progress/objetivo';
	import type { BoardObjective } from '#lib/progress/types';
	import type { Chip } from '#lib/progress/view';

	/**
	 * Os objetivos de um passo como a v7 mostra (V7Fundamentos, "Objetivo com progresso"; V7Rota1440):
	 * um grupo por papel, o rótulo abrindo a linha, e cada objetivo com o quanto falta ("240 /
	 * 1.250") quando o board sabe o número. Abre o painel da entidade por cima da tela. Com
	 * `onmark`/`onunmark`, o objetivo que falta ganha "Já fiz" e o marcado pelo dono, "Desfazer"
	 * (marca de objetivo, docs/09 seção 5; fora do canvas, versão mínima).
	 */
	let {
		chips,
		objectives,
		busy = false,
		onmark,
		onunmark
	}: {
		chips: Chip[];
		objectives: Pick<BoardObjective, 'key' | 'role' | 'observed'>[] | undefined;
		busy?: boolean;
		onmark?: (key: string, label: string) => void;
		onunmark?: (key: string, label: string) => void;
	} = $props();
	const groups = $derived(goalGroups(chips, objectives));
</script>

<div class="goals">
	{#each groups as g (g.role)}
		<span class="gk">{g.label}</span>
		<div class="gv">
			{#each g.goals as o (o.key)}
				{@const icon = iconFor(o.ref)}
				<div class="goalw">
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
					{#if o.ownerMarked}
						<span class="own7" title="Você marcou este objetivo como já feito, sem evidência"
							>marcado por você</span
						>
						{#if onunmark}<button
								class="btn btn-g btn-s"
								type="button"
								disabled={busy}
								title="Desfazer a marca deste objetivo"
								onclick={() => onunmark(o.key, o.name)}>Desfazer</button
							>{/if}
					{:else if o.cls !== 'met' && onmark}
						<button
							class="btn btn-g btn-s"
							type="button"
							disabled={busy}
							title="Marcar como já feito, para a conta toda"
							onclick={() => onmark(o.key, o.name)}>Já fiz</button
						>
					{/if}
				</div>
			{/each}
		</div>
	{/each}
</div>

<style>
	.goalw {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px 8px;
		max-width: 100%;
	}
	.own7 {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--done);
	}
	.goal7:disabled {
		cursor: default;
	}
	.gi {
		width: 22px;
		height: 22px;
	}
</style>
