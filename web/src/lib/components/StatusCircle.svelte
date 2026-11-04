<script lang="ts">
	import Icon from './Icon.svelte';
	type State = 'pending' | 'current' | 'done' | 'skipped';
	let { state = 'pending', size = 20 }: { state?: State; size?: number } = $props();
	const labels: Record<State, string> = {
		pending: 'pendente',
		current: 'passo atual',
		done: 'concluído',
		skipped: 'pulado'
	};
</script>

<span
	class="circle {state}"
	style:width="{size}px"
	style:height="{size}px"
	role="img"
	aria-label={labels[state]}
>
	{#if state === 'done'}
		<Icon name="check" size={size - 8} />
	{:else if state === 'skipped'}
		<Icon name="skip" size={size - 9} />
	{/if}
</span>

<style>
	.circle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex: 0 0 auto;
		border-radius: 50%;
		border: 1.5px solid var(--overlay);
		color: var(--accent-text);
		background: transparent;
	}
	.current {
		border-color: var(--accent);
		box-shadow:
			inset 0 0 0 3px var(--surface),
			inset 0 0 0 20px var(--accent);
	}
	.done {
		border-color: var(--success);
		background: var(--success);
	}
	.skipped {
		border-color: var(--subtle);
		background: var(--elevated);
		color: var(--muted);
	}
</style>
