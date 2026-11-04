<script lang="ts">
	import { describeBank } from '#lib/planner/bank';
	import type { BankLeg, LoadoutItem, ReadinessStatus } from '#lib/planner/types';
	import BankBadge from './BankBadge.svelte';

	let { leg, now = Date.now() }: { leg: BankLeg; now?: number } = $props();
	const view = $derived(describeBank(leg.bank, now));
	const titleId = $derived(`leg-${leg.guideId}`);

	const READINESS: Record<ReadinessStatus, { text: string; tone: 'ok' | 'short' | 'unknown' }> = {
		ready: { text: 'Pronto para sair', tone: 'ok' },
		listed_ready: { text: 'Itens listados com você', tone: 'ok' },
		not_ready: { text: 'Falta item no inventário', tone: 'short' },
		unknown: { text: 'Prontidão não verificável', tone: 'unknown' },
		no_loadout: { text: 'Setup não listado no guia', tone: 'unknown' }
	};
	const readiness = $derived(leg.readiness ? READINESS[leg.readiness.status] : null);

	function itemState(it: LoadoutItem): {
		text: string;
		tone: 'ok' | 'short' | 'stale' | 'unknown';
	} {
		// Estar com o item (inventário/equipamento ao vivo) não é o mesmo que tê-lo no banco.
		if (it.carried === 'met') return { text: 'com você', tone: 'ok' };
		if (it.inBank === null) return { text: 'conferir', tone: 'unknown' };
		if (!view.trustworthy) return { text: `salvo: ${it.inBank}`, tone: 'stale' };
		return it.inBank >= it.qty
			? { text: `no banco ${it.inBank}`, tone: 'ok' }
			: { text: `falta ${it.qty - it.inBank}`, tone: 'short' };
	}
</script>

<section class="leg" data-freshness={view.freshness} aria-labelledby={titleId}>
	<div class="top">
		<span class="eyebrow">Próxima perna de banco</span>
		<BankBadge bank={leg.bank} {now} />
	</div>
	<h2 id={titleId} class="title">{leg.location}</h2>
	<span class="data muted">
		no passo {leg.atStepId} · depois {leg.stepsUntilNextBank} passos de campo
	</span>
	{#if readiness && leg.readiness}
		<p class="readiness">
			<span class="data state {readiness.tone}">{readiness.text}</span>
			{#if leg.readiness.detail}<span class="muted">{leg.readiness.detail}</span>{/if}
		</p>
	{/if}
	{#if !view.trustworthy}
		<p class="note" role="note">{view.detail}</p>
	{/if}

	{#if leg.withdraw.length}
		<div class="block">
			<span class="eyebrow">Sacar</span>
			<ul class="items">
				{#each leg.withdraw as it, i (`${it.name}-${i}`)}
					{@const s = itemState(it)}
					<li>
						<span class="data qty">{it.qtyStated === false ? '—' : `${it.qty}×`}</span>
						<span class="name"
							>{it.name}{#if it.optional}<span class="muted"> · opcional</span>{/if}</span
						>
						<span class="data state {s.tone}">{s.text}</span>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
	{#if leg.deposit.length}
		<div class="block">
			<span class="eyebrow">Depositar</span>
			<ul class="chips">
				{#each leg.deposit as d, i (`${d}-${i}`)}<li class="data">{d}</li>{/each}
			</ul>
		</div>
	{/if}
</section>

<style>
	.leg {
		background: var(--surface);
		border-radius: var(--r-panel);
		padding: var(--sp-6);
		display: flex;
		flex-direction: column;
		gap: var(--sp-3);
		border: 1px solid transparent;
	}
	.leg[data-freshness='restored'] {
		border: 1.5px dashed var(--warning);
	}
	.leg[data-freshness='unavailable'] {
		border-color: color-mix(in srgb, var(--error) 50%, transparent);
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-2);
		flex-wrap: wrap;
	}
	.title {
		font-size: 20px;
		font-weight: 400;
	}
	.readiness {
		display: flex;
		flex-direction: column;
		gap: 2px;
		font-size: var(--fs-label);
		line-height: 1.4;
	}
	.note {
		padding: 10px 12px;
		border-radius: var(--r-button);
		font-size: var(--fs-label);
		line-height: 1.4;
	}
	[data-freshness='restored'] .note {
		color: var(--warning);
		background: var(--warning-tint);
	}
	[data-freshness='unavailable'] .note {
		color: var(--error);
		background: var(--error-tint);
	}
	.block {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.items {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.items li {
		min-height: 36px;
		padding: 6px 10px;
		border-radius: 9px;
		background: var(--bg);
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}
	.qty {
		width: 44px;
		flex: none;
		color: var(--muted);
	}
	.name {
		flex: 1;
		min-width: 0;
		font-size: var(--fs-control);
	}
	.state.ok {
		color: var(--success);
	}
	.state.short {
		color: var(--warning);
	}
	.state.stale {
		color: var(--muted);
		font-style: italic;
	}
	.state.unknown {
		color: var(--muted);
	}
	.chips {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.chips li {
		padding: 4px 10px;
		border-radius: var(--r-chip);
		background: var(--bg);
	}
</style>
