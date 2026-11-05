<script lang="ts">
	import BankAhead from './BankAhead.svelte';
	import Glyph from './Glyph.svelte';
	import { itemIcon } from '#lib/progress/icons';
	import { bankColumns, freshnessLabel, loadoutItems, qtyLabel } from '#lib/progress/view';
	import type { Ahead, BankAction, Loadout, SegmentReadiness } from '#lib/progress/types';

	/**
	 * Setup do trecho em três colunas: com você / no banco / o guia manda (V4AgoraOziris,
	 * decisão 7 do v4); o estado de readiness vira frase, nunca badge (v6). Ações de banco são
	 * instrução e nunca viram passo feito; ausência só é afirmada com container live e baseline
	 * full (regra 8). Sem setup do guia, mostra "Levar do banco", a prévia pelos próximos passos
	 * (BankAhead).
	 */
	let {
		readiness,
		range,
		loadout,
		bankActions,
		firstRef,
		ahead = null
	}: {
		readiness: SegmentReadiness | null;
		range: string;
		loadout?: Loadout;
		bankActions: BankAction[];
		firstRef: string;
		ahead?: Ahead | null;
	} = $props();

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 15_000);
		return () => clearInterval(t);
	});

	const cols = $derived(readiness ? bankColumns(readiness) : null);
	const inv = $derived(freshnessLabel(readiness, 'inventory', now));
	const bank = $derived(freshnessLabel(readiness, 'bank', now));
	const actions = $derived(readiness?.bank_actions?.length ? readiness.bank_actions : bankActions);
	const fallback = $derived(loadoutItems(loadout));
	const partial = $derived((readiness?.loadout_status ?? loadout?.status) === 'partial');
</script>

<section class="panel">
	<div class="plaque">
		{#if ahead}Levar do banco<span class="r">até {ahead.until_step_id} · fim do trecho</span
			>{:else}Setup do trecho<span class="r">{range}</span>{/if}
	</div>
	{#if ahead}
		<BankAhead {ahead} bankActions={actions} {firstRef} bankLabel={bank.label} bankCls={bank.cls} />
	{:else}
		<div class="cols">
			<div class="col">
				<div class="h"><p class="ey">Com você</p></div>
				<span class="fr {inv.cls}"><span class="d"></span>{inv.label}</span>
				{#if cols && cols.total === 0}
					<span class="mu small">O guia não lista itens para este trecho.</span>
				{:else if cols}
					<span class="mu small"
						>{cols.carried} de {cols.total} itens do setup com você{partial
							? ', e o guia lista o setup só em parte'
							: ''}.</span
					>
				{:else}
					<span class="mu small"
						>Sem plugin, o que você carrega não é verificável. Confira à mão.</span
					>
				{/if}
			</div>
			<div class="col">
				<div class="h">
					<p class="ey">No banco</p>
					<span class="fr {bank.cls} push"><span class="d"></span>{bank.label}</span>
				</div>
				<div class="slots">
					{#if cols}
						{#each cols.slots as s, i (i)}
							{@const icon = itemIcon(s.itemId)}
							<span class="slot {s.cls}" title={s.suffix ? `${s.name} · ${s.suffix}` : s.name}>
								<span class="bx">
									{#if icon}<img class="sp" src={icon} alt="" />{/if}
									<span class="q">{s.qty}</span>
									{#if s.ok}<span class="ok"><Glyph name="check" size={10} /></span>{/if}
									{#if s.cls === 'slot-unk'}<span class="qm">?</span>{/if}
								</span>
								<span class="nm">{s.name}</span>
							</span>
						{/each}
					{:else}
						{#each fallback as it, i (i)}
							{@const icon = itemIcon(it.item_id ?? it.any_of?.[0])}
							<span class="slot slot-unk" title="{it.name} · não sabemos: sem leitura do plugin">
								<span class="bx">
									{#if icon}<img class="sp" src={icon} alt="" />{/if}
									<span class="q">{qtyLabel(it.qty)}</span><span class="qm">?</span>
								</span>
								<span class="nm">{it.name}</span>
							</span>
						{:else}
							<span class="su small">O guia não lista itens para este trecho.</span>
						{/each}
					{/if}
				</div>
				{#if cols && cols.total > 0}
					<span class="su tiny">
						{cols.inBank} no banco · {cols.missing} falta{#if cols.unresolved.length}
							· sem id: {cols.unresolved.map((u) => `"${u}"`).join(', ')}{/if}.
					</span>
				{/if}
			</div>
			<div class="col">
				<div class="h"><p class="ey">O guia manda</p></div>
				{#each actions as a, i (i)}
					<span class="quote">“{a.text}”</span>
				{:else}
					<span class="su small">Nenhuma ação de banco neste trecho.</span>
				{/each}
				{#if actions.length}
					<span class="su tiny">instrução do passo {firstRef} · nunca é marcada como feita</span>
				{/if}
			</div>
		</div>
	{/if}
</section>

<style>
	section {
		display: flex;
		flex-direction: column;
	}
	.cols {
		padding: 14px 18px 12px;
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 2fr) minmax(0, 1fr);
		gap: 16px;
	}
	@media (max-width: 760px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.fr {
		align-self: flex-start;
	}
	.push {
		margin-left: auto;
	}
	.badge {
		align-self: flex-start;
	}
	.slots {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 6px;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	.quote {
		font-size: 13px;
	}
</style>
