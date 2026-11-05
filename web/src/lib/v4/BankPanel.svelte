<script lang="ts">
	import BankAhead from './BankAhead.svelte';
	import Glyph from './Glyph.svelte';
	import { itemIcon } from '#lib/progress/icons';
	import {
		bankColumns,
		bankSummary,
		freshnessLabel,
		loadoutItems,
		qtyLabel
	} from '#lib/progress/view';
	import type { Ahead, BankAction, Loadout, SegmentReadiness } from '#lib/progress/types';

	/**
	 * Banco do trecho em três colunas: com você / no banco / o guia manda (V4AgoraOziris,
	 * decisão 7 do v4). Ações de banco são instrução e nunca viram passo feito; ausência só é
	 * afirmada com container live e baseline full (regra 8). Sem setup do guia, mostra a prévia
	 * pelos próximos passos (BankAhead).
	 * v5 limpeza: collapsed by default to one line plus the missing items; the choice is
	 * remembered in this browser only.
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
	const STATUS_CLS: Record<string, string> = {
		ready: 'badge-done',
		listed_ready: 'badge-done',
		not_ready: 'badge-red',
		unknown: 'badge-unk',
		no_loadout: ''
	};
	const partial = $derived((readiness?.loadout_status ?? loadout?.status) === 'partial');

	const KEY = 'ironman.bank.open';
	let open = $state(readOpen());
	function readOpen(): boolean {
		try {
			return localStorage.getItem(KEY) === '1';
		} catch {
			return false;
		}
	}
	function toggle() {
		open = !open;
		try {
			localStorage.setItem(KEY, open ? '1' : '0');
		} catch {
			/* local convenience only */
		}
	}
	const summary = $derived(
		cols
			? bankSummary({ total: cols.total, miss: cols.missing, unk: cols.unverifiable })
			: bankSummary({ total: fallback.length, miss: 0, unk: fallback.length })
	);
	const missingSlots = $derived(cols ? cols.slots.filter((s) => s.cls === 'slot-miss') : []);
</script>

<section class="panel">
	<div class="plaque">
		Banco · trecho {range}
		{#if open && !ahead}
			<span class="r">
				{#if readiness}readiness: {readiness.status}{:else}sem leitura do plugin{/if}{partial
					? ' · o guia lista o setup só em parte'
					: ''}
			</span>
		{/if}
	</div>
	{#if ahead}
		<BankAhead
			{ahead}
			bankActions={actions}
			{firstRef}
			bankLabel={bank.label}
			bankCls={bank.cls}
			{open}
			ontoggle={toggle}
		/>
	{:else if !open}
		<div class="top">
			<button class="disc sum-btn" type="button" aria-expanded={open} onclick={toggle}
				><Glyph name="chevron" size={12} /><span class:has-miss={summary.missing > 0}
					>{summary.text}</span
				></button
			>
		</div>
		{#if missingSlots.length}
			<div class="miss" aria-label="Faltam">
				{#each missingSlots as s, i (i)}
					{@const icon = itemIcon(s.itemId)}
					<span class="slot {s.cls}">
						<span class="bx">
							{#if icon}<img class="sp" src={icon} alt="" />{/if}
							<span class="q">{s.qty}</span>
						</span>
						<span class="nm">{s.name} · falta</span>
					</span>
				{/each}
			</div>
		{/if}
	{:else}
		<div class="top">
			<button class="disc sum-btn" type="button" aria-expanded={open} onclick={toggle}
				><Glyph name="chevron" size={12} /><span class:has-miss={summary.missing > 0}
					>{summary.text}</span
				></button
			>
		</div>
		<div class="cols">
			<div class="col">
				<div class="h"><p class="ey">Com você</p></div>
				<span class="fr {inv.cls}"><span class="d"></span>{inv.label}</span>
				{#if cols && cols.total === 0}
					<span class="mu small">O guia não lista itens para este trecho.</span>
				{:else if cols}
					<span class="mu small">{cols.carried} de {cols.total} itens do setup carregados.</span>
					<span class="badge {STATUS_CLS[readiness?.status ?? ''] ?? ''}">{readiness?.status}</span>
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
							<span class="slot {s.cls}">
								<span class="bx">
									{#if icon}<img class="sp" src={icon} alt="" />{/if}
									<span class="q">{s.qty}</span>
									{#if s.ok}<span class="ok"><Glyph name="check" size={10} /></span>{/if}
									{#if s.cls === 'slot-unk'}<span class="qm">?</span>{/if}
								</span>
								<span class="nm">{s.name}{s.suffix ? ` · ${s.suffix}` : ''}</span>
							</span>
						{/each}
					{:else}
						{#each fallback as it, i (i)}
							{@const icon = itemIcon(it.item_id ?? it.any_of?.[0])}
							<span class="slot slot-unk">
								<span class="bx">
									{#if icon}<img class="sp" src={icon} alt="" />{/if}
									<span class="q">{qtyLabel(it.qty)}</span><span class="qm">?</span>
								</span>
								<span class="nm">{it.name} · não sabemos</span>
							</span>
						{:else}
							<span class="su small">O guia não lista itens para este trecho.</span>
						{/each}
					{/if}
				</div>
				{#if cols && cols.total > 0}
					<span class="su tiny">
						{cols.inBank} no banco · {cols.missing} falta · {cols.unverifiable} não verificáveis{#if cols.unresolved.length}
							(o catálogo não resolveu {cols.unresolved.map((u) => `"${u}"`).join(' e ')}){/if}.
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
	.top {
		padding: 8px 12px 4px;
		display: flex;
	}
	.sum-btn {
		flex: 1 1 auto;
		justify-content: flex-start;
		font-size: 13px;
		color: var(--ink);
	}
	.has-miss {
		font-weight: 600;
	}
	.miss {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 6px;
		padding: 4px 18px 12px;
	}
	.cols {
		padding: 6px 18px 12px;
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
