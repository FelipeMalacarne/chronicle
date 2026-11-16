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
	 * (marca de objetivo, docs/09 seção 5; fora do canvas, versão mínima). Com `current` (o passo
	 * atual), o de item ganha "Já tenho" (decisão do dono, 2026-11-15).
	 */
	let {
		chips,
		objectives,
		busy = false,
		current = false,
		onmark,
		onunmark
	}: {
		chips: Chip[];
		objectives: Pick<BoardObjective, 'key' | 'role' | 'observed'>[] | undefined;
		busy?: boolean;
		current?: boolean;
		onmark?: (key: string, label: string) => void;
		onunmark?: (key: string, label: string) => void;
	} = $props();
	const groups = $derived(goalGroups(chips, objectives));
	const have = (o: { item: boolean }) => current && o.item;
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
						<span
							class="own7"
							title={have(o)
								? 'Você marcou que já tem este item, sem evidência'
								: 'Você marcou este objetivo como já feito, sem evidência'}>marcado por você</span
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
							class="btn btn-s"
							type="button"
							disabled={busy}
							title={have(o)
								? 'Marcar como já tenho, para a conta toda'
								: 'Marcar como já feito, para a conta toda'}
							onclick={() => onmark(o.key, o.name)}>{have(o) ? 'Já tenho' : 'Já fiz'}</button
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
