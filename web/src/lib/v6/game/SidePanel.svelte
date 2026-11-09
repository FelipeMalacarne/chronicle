<script lang="ts">
	import type { GameView } from '#lib/api/microbot';
	import { equipmentLayout } from '#lib/progress/conta';
	import { iconFor, itemIcon } from '#lib/progress/icons';
	import { equipItems, invSlots, itemMenu, qtyLabel, skillCells } from '#lib/progress/jogo';
	import { openMenu } from './menu.svelte';

	/*
	 * O painel lateral do cliente (doc 08, V6MicrobotCliente1440): as abas do jogo com os ícones do
	 * jogo. Itens abre o "Choose Option" com as ações reais do item (com o banco aberto, as de
	 * depósito); Equipado e Skills só mostram, porque o Agent Server não tem ação de equipamento.
	 * Oração e Magia esperam a ponte do Dev Bootstrap (V6MicrobotClienteEstados1440).
	 */
	let { view }: { view: GameView } = $props();

	type Tab = 'itens' | 'equipado' | 'skills' | 'oracao' | 'magia';
	let tab = $state<Tab>('itens');
	/** [aba, nome, ícone, disponível] */
	const TABS: [Tab, string, string | undefined, boolean][] = [
		['itens', 'Itens', itemIcon(995), true],
		['equipado', 'Equipado', itemIcon(1127), true],
		['skills', 'Skills', iconFor('skill:attack'), true],
		['oracao', 'Oração', iconFor('skill:prayer'), false],
		['magia', 'Magia', iconFor('skill:magic'), false]
	];

	const slots = $derived(invSlots(view));
	const equip = $derived(equipmentLayout(equipItems(view)));
	const skills = $derived(skillCells(view));
	const bankOpen = $derived(!!view.bank?.open);
	const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
</script>

<aside class="gside" aria-label="Painel do jogo">
	<div class="gtabs" role="tablist">
		{#each TABS as [k, label, icon, ok] (k)}
			<button
				type="button"
				role="tab"
				class:on={tab === k}
				aria-selected={tab === k}
				aria-label={label}
				disabled={!ok}
				title={ok ? label : `${label}: precisa da ponte do Dev Bootstrap (próxima entrega)`}
				onclick={() => (tab = k)}
				>{#if icon}<img src={icon} alt="" />{:else}{label}{/if}</button
			>
		{/each}
	</div>

	{#if tab === 'itens'}
		<div class="ginv" role="grid" aria-label="Inventário">
			{#each slots as it, n (n)}
				{#if it}
					{@const icon = itemIcon(it.id)}
					{@const q = it.quantity > 1 || it.stackable ? qtyLabel(it.quantity) : null}
					<button
						type="button"
						class="gslot"
						title={it.name}
						onclick={(e) => openMenu(e, itemMenu(it, bankOpen))}
						oncontextmenu={(e) => openMenu(e, itemMenu(it, bankOpen))}
						>{#if icon}<img src={icon} alt={it.name} />{:else}<span class="gab"
								>{it.name.slice(0, 4)}</span
							>{/if}{#if q}<span class="gq {q.tone}">{q.text}</span>{/if}</button
					>
				{:else}
					<span class="gslot e" aria-hidden="true"></span>
				{/if}
			{/each}
		</div>
		{#if bankOpen}<p class="ghint">Banco aberto: o menu dos itens deposita.</p>{/if}
	{:else if tab === 'equipado'}
		<div class="geqw">
			<div class="geq">
				{#each equip.cells as c (c.slot)}
					<div
						class="ges"
						style="grid-row: {c.row}; grid-column: {c.col};"
						title={c.item ? c.item.name : `${c.label}: vazio`}
					>
						{#if c.item}
							{@const icon = itemIcon(c.item.id)}
							{#if icon}<img src={icon} alt={c.item.name} />{:else}<span class="gab"
									>{c.item.name.slice(0, 4)}</span
								>{/if}
							{#if c.item.qty > 1}<span class="gq y">{qtyLabel(c.item.qty).text}</span>{/if}
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{:else}
		<div class="gsk">
			{#each skills as s (s.skill)}
				{@const icon = iconFor(`skill:${s.skill}`)}
				<span class="gsc" title="{cap(s.skill)} · {s.xp.toLocaleString('pt-BR')} xp"
					>{#if icon}<img src={icon} alt="" />{/if}<span
						class="glv"
						class:boost={s.boosted !== s.level}>{s.boosted}<i>/</i>{s.level}</span
					></span
				>
			{/each}
		</div>
		<div class="gtot">Total level: {view.skills?.totalLevel ?? '—'}</div>
	{/if}
</aside>

<style>
	/* A pele do jogo dentro do cliente (decisão do dono, página 6 do canvas). */
	.gside {
		width: 226px;
		flex: 0 0 auto;
		align-self: stretch;
		display: flex;
		flex-direction: column;
		box-sizing: border-box;
		background: #3e3529;
		border: 2px solid #1c160d;
		box-shadow: inset 1px 1px 0 #6a5a3c;
		border-radius: 4px;
		padding: 6px;
		color: var(--on-frame);
	}
	.gtabs {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: 2px;
		margin-bottom: 6px;
	}
	.gtabs button {
		height: 34px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 1px solid #000;
		border-radius: 3px 3px 0 0;
		background: #2e2720;
		color: var(--on-frame-muted);
		font:
			500 10px Inter,
			system-ui,
			sans-serif;
		cursor: pointer;
		padding: 0;
	}
	.gtabs button img {
		max-width: 22px;
		max-height: 22px;
	}
	.gtabs button.on {
		background: #6a3b26;
		box-shadow: inset 0 0 0 1px #9c5a32;
	}
	.gtabs button:disabled {
		cursor: not-allowed;
	}
	.gtabs button:disabled img {
		filter: grayscale(1) brightness(0.5);
	}
	.ginv {
		min-height: 264px;
		align-content: start;
		box-sizing: border-box;
		border: 1px solid #000;
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		grid-auto-rows: 36px;
		gap: 2px 4px;
		padding: 4px 2px;
		background: #2e2720;
		border-radius: 3px;
	}
	.gslot {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		border-radius: 3px;
		background: transparent;
		padding: 0;
		cursor: pointer;
	}
	.gslot:hover {
		background: rgba(255, 255, 255, 0.08);
	}
	.gslot:focus-visible {
		outline: 1px solid #fff;
	}
	.gslot.e {
		cursor: default;
	}
	.gslot img,
	.ges img {
		max-width: 32px;
		max-height: 30px;
		image-rendering: pixelated;
	}
	.gab {
		font:
			500 9px 'JetBrains Mono',
			monospace;
		color: var(--on-frame-muted);
	}
	/* Quantidades com as cores do jogo: amarelo, branco em K, verde em M. */
	.gq {
		position: absolute;
		left: 1px;
		top: -1px;
		font:
			600 11px/1 'JetBrains Mono',
			monospace;
		text-shadow:
			1px 1px 0 #000,
			-1px 0 0 #000;
	}
	.gq.y {
		color: #ffff00;
	}
	.gq.w {
		color: #fff;
	}
	.gq.g {
		color: #00ff80;
	}
	.ghint {
		margin: 6px 2px 0;
		font-size: 11px;
		color: var(--gold);
	}
	.geqw {
		display: flex;
		justify-content: center;
		padding: 8px 0;
		background: #2e2720;
		border-radius: 3px;
	}
	.geq {
		display: grid;
		grid-template-columns: repeat(3, 40px);
		grid-template-rows: repeat(5, 40px);
		gap: 6px;
	}
	.ges {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #2a241c;
		border: 1px solid #000;
		border-radius: 3px;
	}
	.gsk {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 2px;
		background: #2e2720;
		border-radius: 3px;
		padding: 3px;
	}
	.gsc {
		display: flex;
		align-items: center;
		gap: 4px;
		height: 26px;
		padding: 0 4px;
		background: #3a3226;
		border: 1px solid #000;
		border-radius: 2px;
	}
	.gsc img {
		width: 18px;
		height: 18px;
		object-fit: contain;
	}
	.glv {
		margin-left: auto;
		font:
			600 11px 'JetBrains Mono',
			monospace;
		color: #ffff00;
		text-shadow: 1px 1px 0 #000;
	}
	.glv i {
		font-style: normal;
		opacity: 0.6;
		margin: 0 1px;
	}
	.glv.boost {
		color: #00ff80;
	}
	.gtot {
		margin-top: 4px;
		text-align: center;
		font:
			600 11px 'JetBrains Mono',
			monospace;
		color: #ffff00;
	}
</style>
