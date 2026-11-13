<script lang="ts">
	import type { GameView } from '#lib/api/microbot';
	import { itemIcon } from '#lib/progress/icons';
	import { bankMenu, qtyLabel } from '#lib/progress/jogo';
	import { openMenu } from './menu.svelte';

	/*
	 * O banco aberto no lugar da tela do jogo (V7Cliente1440): a grade com as quantidades nas cores
	 * do jogo; cada item abre o menu de Withdraw-1, 5, 10, X e All.
	 */
	let { view }: { view: GameView } = $props();
</script>

<div class="gbank" role="grid" aria-label="Banco">
	{#each view.bank?.items ?? [] as i (i.id)}
		{@const icon = itemIcon(i.id)}
		{@const q = qtyLabel(i.quantity)}
		<button
			type="button"
			class="bs"
			title={`${i.name} ×${i.quantity.toLocaleString('pt-BR')}`}
			onclick={(e) => openMenu(e, bankMenu(i))}
			oncontextmenu={(e) => openMenu(e, bankMenu(i))}
			>{#if icon}<img src={icon} alt={i.name} />{:else}<span class="gab">{i.name.slice(0, 4)}</span
				>{/if}<span class="gq {q.tone}">{q.text}</span></button
		>
	{:else}
		<span class="none">Banco vazio.</span>
	{/each}
</div>

<style>
	.gbank {
		position: absolute;
		inset: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, 44px);
		grid-auto-rows: 38px;
		align-content: start;
		justify-content: center;
		gap: 2px;
		overflow: auto;
		padding: 36px 10px 10px;
		box-sizing: border-box;
		background: #2e2720;
	}
	.bs {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		background: transparent;
		border-radius: 3px;
		cursor: pointer;
		padding: 0;
	}
	.bs:hover {
		background: rgba(255, 255, 255, 0.08);
	}
	.bs img {
		max-width: 32px;
		max-height: 30px;
		image-rendering: pixelated;
	}
	.gab {
		font:
			500 9px 'JetBrains Mono',
			monospace;
		color: #b3a58a;
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
	.none {
		grid-column: 1 / -1;
		color: #b3a58a;
		font-size: 12px;
	}
</style>
