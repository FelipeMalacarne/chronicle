<script lang="ts">
	import { describeBank } from '#lib/planner/bank';
	import type { BankStatus } from '#lib/planner/types';

	let { bank, now = Date.now() }: { bank: BankStatus; now?: number } = $props();
	const view = $derived(describeBank(bank, now));
</script>

<span class="bank" data-freshness={view.freshness} title={view.detail}>
	<span class="dot" aria-hidden="true"></span>
	{view.label}
</span>

<style>
	.bank {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 22px;
		padding: 0 8px;
		border-radius: 6px;
		box-sizing: border-box;
		font-family: var(--font-mono);
		font-size: var(--fs-micro);
		letter-spacing: 0.6px;
		text-transform: uppercase;
		white-space: nowrap;
	}
	.dot {
		width: 6px;
		height: 6px;
		border-radius: 3px;
		box-sizing: border-box;
		background: currentColor;
	}
	[data-freshness='live'] {
		color: var(--success);
		background: var(--success-tint);
	}
	[data-freshness='restored'] {
		color: var(--warning);
		background: var(--warning-tint);
		border: 1px dashed var(--warning);
	}
	[data-freshness='unavailable'] {
		color: var(--error);
		background: var(--error-tint);
	}
	[data-freshness='unavailable'] .dot {
		background: transparent;
		border: 1px solid currentColor;
	}
</style>
