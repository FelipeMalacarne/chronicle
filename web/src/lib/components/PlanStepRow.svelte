<script lang="ts">
	import type { StepVM } from '#lib/planner/types';

	type RowState = 'pending' | 'current' | 'done';
	let {
		step,
		state = 'pending',
		setsize,
		posinset,
		compact = false
	}: {
		step: StepVM;
		state?: RowState;
		setsize?: number;
		posinset?: number;
		compact?: boolean;
	} = $props();
	const labels: Record<RowState, string> = {
		pending: 'pendente',
		current: 'passo atual',
		done: 'concluído'
	};
</script>

<li
	class="row {state}"
	class:compact
	aria-setsize={setsize}
	aria-posinset={posinset}
	aria-current={state === 'current' ? 'step' : undefined}
>
	<span class="circle" role="img" aria-label={labels[state]}>{state === 'done' ? '✓' : ''}</span>
	<span class="data id" title={step.key}>{step.localId}</span>
	<span class="text">{step.text}</span>
	{#if step.milestone}<span class="tag accent">◆ marco</span>{/if}
	{#if step.bank}<span class="tag">banco</span>{/if}
	{#if step.location && !compact}<span class="loc">{step.location}</span>{/if}
</li>

<style>
	.row {
		height: 44px;
		box-sizing: border-box;
		padding: 0 var(--sp-3);
		border-radius: var(--r-button);
		background: var(--mantle);
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		min-width: 0;
		list-style: none;
	}
	.compact {
		height: 40px;
		gap: 10px;
		padding: 0 10px;
		border-radius: 9px;
		background: var(--surface);
		font-size: var(--fs-control);
	}
	.row.current {
		background: var(--accent-tint);
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.row.done {
		color: var(--muted);
	}
	.circle {
		width: 18px;
		height: 18px;
		border-radius: 9px;
		box-sizing: border-box;
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 700;
		border: 1.7px solid var(--subtle);
	}
	.done .circle {
		border: 0;
		background: var(--success);
		color: var(--accent-text);
	}
	.current .circle {
		border: 5px solid var(--accent);
	}
	.id {
		color: var(--subtle);
		width: 56px;
		flex: none;
	}
	.compact .id {
		width: 52px;
	}
	.text {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.tag {
		padding: 2px 8px;
		border-radius: var(--r-chip);
		background: var(--elevated);
		font-family: var(--font-mono);
		font-size: var(--fs-micro);
		color: var(--muted);
		flex: none;
	}
	.tag.accent {
		background: var(--accent-tint);
		color: var(--accent);
	}
	.loc {
		font-size: var(--fs-caption);
		color: var(--muted);
		max-width: 28%;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	@media (max-width: 719px) {
		.loc {
			display: none;
		}
	}
</style>
