<script lang="ts">
	import type { Step, StepStatus } from '#lib/api/types';
	import StatusCircle from './StatusCircle.svelte';

	let {
		step,
		status = 'pending',
		current = false,
		href
	}: { step: Step; status?: StepStatus; current?: boolean; href?: string } = $props();

	const circle = $derived(
		current
			? 'current'
			: status === 'skipped'
				? 'skipped'
				: status === 'pending'
					? 'pending'
					: 'done'
	);
</script>

<svelte:element this={href ? 'a' : 'div'} class="row" class:current {href}>
	<StatusCircle state={circle} />
	<span class="data id">{step.id}</span>
	<span class="text">{step.text}</span>
	{#if step.layer === 'enhanced'}
		<span class="badge badge-accent">enhanced</span>
	{/if}
	{#if step.location}
		<span class="caption loc">{step.location}</span>
	{/if}
</svelte:element>

<style>
	.row {
		min-width: 0;
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		min-height: var(--h-row);
		padding: 0 var(--sp-4);
		border-radius: var(--r-row);
		color: var(--text);
		border: 1px solid transparent;
	}
	a.row:hover {
		text-decoration: none;
		background: var(--elevated);
	}
	.current {
		box-shadow: 0 0 0 2px var(--accent);
		background: var(--accent-tint);
	}
	.id {
		color: var(--subtle);
		flex: 0 0 auto;
		min-width: 46px;
	}
	.text {
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.loc {
		flex: 0 0 auto;
		max-width: 30%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	@media (max-width: 719px) {
		.loc {
			display: none;
		}
	}
</style>
