<script lang="ts">
	import type { GameView } from '#lib/api/microbot';
	import { gameMenu, runAction } from './game/menu.svelte';

	/*
	 * Banco (V7Cliente1440): fechado, abre o banco mais perto; aberto, a grade aparece no lugar da
	 * tela (GameBank) e o cartão deposita o inventário (segundo clique) ou fecha.
	 */
	let { view }: { view: GameView } = $props();

	const bank = $derived(view.bank);
	let armed = $state(false);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	function depositAll() {
		if (!armed) {
			armed = true;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = false), 4000);
			return;
		}
		armed = false;
		void runAction({
			label: 'Deposit inventory',
			target: '',
			tone: 'item',
			command: 'bank_deposit',
			body: { all: true }
		});
	}
</script>

<section class="card">
	<div class="h">
		<h2>Banco</h2>
		<span>{bank?.open ? `aberto · ${bank.count ?? bank.items?.length ?? 0} itens` : 'fechado'}</span
		>
	</div>
	<div class="b">
		{#if bank?.open}
			<div class="acts">
				<button
					class="btn btn-s"
					class:btn-d={armed}
					type="button"
					disabled={gameMenu.busy}
					onclick={depositAll}>{armed ? 'Confirmar: depositar' : 'Depositar o inventário'}</button
				>
				<button
					class="btn btn-s btn-g"
					type="button"
					disabled={gameMenu.busy}
					onclick={() =>
						runAction({ label: 'Close', target: '', tone: 'object', command: 'bank_close' })}
					>Fechar</button
				>
			</div>
		{:else}
			<p class="mu">
				Aberto, o banco aparece no lugar da tela e o menu dos itens vira o de depósito.
			</p>
			<button
				class="btn btn-s self"
				type="button"
				disabled={gameMenu.busy}
				onclick={() =>
					runAction({ label: 'Bank', target: '', tone: 'object', command: 'bank_open' })}
				>Abrir o banco mais perto</button
			>
		{/if}
	</div>
</section>

<style>
	.b {
		gap: 8px;
	}
	.mu {
		font-size: 12px;
		line-height: 1.45;
	}
	.self {
		align-self: flex-start;
	}
	.acts {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
</style>
