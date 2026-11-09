<script lang="ts">
	import { cancelSelection, closeMenu, gameMenu, pick } from './menu.svelte';

	/*
	 * O "Choose Option" do jogo: fundo e cores do menu do OSRS (ação em branco, alvo na cor do tipo:
	 * laranja item, amarelo NPC, ciano objeto), aberto no ponto do clique e preso à janela.
	 */
	let box = $state<HTMLDivElement>();
	let typed = $state('');
	let askAt = $state(-1);

	const pos = $derived.by(() => {
		const w = box?.offsetWidth ?? 180;
		const h = box?.offsetHeight ?? 120;
		return {
			x: Math.max(4, Math.min(gameMenu.x - 10, window.innerWidth - w - 4)),
			y: Math.max(4, Math.min(gameMenu.y - 8, window.innerHeight - h - 4))
		};
	});

	$effect(() => {
		if (!gameMenu.open) return;
		askAt = -1;
		typed = '';
		const away = (e: Event) => {
			if (box && !box.contains(e.target as Node)) closeMenu();
		};
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && closeMenu();
		// Só a página rolando tira o menu do lugar; o chat e o log rolam sozinhos a cada leitura.
		const scrolled = (e: Event) => e.target === document && closeMenu();
		window.addEventListener('pointerdown', away, true);
		window.addEventListener('keydown', esc);
		window.addEventListener('scroll', scrolled, true);
		return () => {
			window.removeEventListener('pointerdown', away, true);
			window.removeEventListener('keydown', esc);
			window.removeEventListener('scroll', scrolled, true);
		};
	});

	// Com algo selecionado e o menu fechado, Esc desfaz a seleção.
	$effect(() => {
		if (!gameMenu.selected || gameMenu.open) return;
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && cancelSelection();
		window.addEventListener('keydown', esc);
		return () => window.removeEventListener('keydown', esc);
	});

	function choose(i: number) {
		if (gameMenu.options[i]?.ask) {
			askAt = i;
			return;
		}
		void pick(i);
	}
</script>

{#if gameMenu.open}
	<div
		class="menu"
		role="menu"
		tabindex="-1"
		bind:this={box}
		style="left: {pos.x}px; top: {pos.y}px;"
		oncontextmenu={(e) => e.preventDefault()}
	>
		<div class="mhd">Choose Option</div>
		{#each gameMenu.options as o, i (i)}
			{#if askAt === i}
				<form
					class="mask"
					onsubmit={(e) => {
						e.preventDefault();
						void pick(i, typed);
					}}
				>
					<!-- svelte-ignore a11y_autofocus -->
					<input
						autofocus
						inputmode={o.ask === 'number' ? 'numeric' : 'text'}
						placeholder={o.ask === 'number' ? 'quantidade (ex. 14, 2k)' : 'ação do menu do jogo'}
						aria-label="{o.label} em {o.target}"
						bind:value={typed}
					/>
				</form>
			{:else}
				<button class="mop" role="menuitem" type="button" onclick={() => choose(i)}
					><span class="ma" class:arm={gameMenu.armed === i}
						>{gameMenu.armed === i ? `Confirmar ${o.label}` : o.label}</span
					>{#if o.target}
						<span class="mt {o.tone}">{o.target}</span>{/if}</button
				>
			{/if}
		{/each}
		<!-- Cancel fecha o menu e, como no jogo, desfaz o item ou feitiço selecionado. -->
		<button class="mop" role="menuitem" type="button" onclick={cancelSelection}
			><span class="ma">Cancel</span></button
		>
	</div>
{/if}

<style>
	/* Cores do menu do jogo (replicação do cliente, fora dos tokens do Pergaminho de propósito). */
	.menu {
		position: fixed;
		z-index: 60;
		min-width: 150px;
		background: #5d5447;
		border: 1px solid #000;
		box-shadow: 0 4px 14px rgba(0, 0, 0, 0.45);
		padding: 0 0 3px;
		font:
			400 13px/1.25 'RuneScape Bold',
			Inter,
			system-ui,
			sans-serif;
	}
	.mhd {
		background: #000;
		color: #5d5447;
		padding: 3px 6px;
		font-weight: 600;
		border: 1px solid #5d5447;
		margin: 1px;
	}
	.mop {
		display: block;
		width: 100%;
		text-align: left;
		border: 0;
		background: transparent;
		padding: 3px 8px;
		color: #fff;
		font: inherit;
		cursor: pointer;
		white-space: nowrap;
	}
	.mop:hover,
	.mop:focus-visible {
		background: rgba(0, 0, 0, 0.35);
		outline: none;
	}
	.ma.arm {
		color: #ff5c5c;
	}
	.mt {
		margin-left: 4px;
	}
	.mt.item {
		color: #ff9040;
	}
	.mt.npc {
		color: #ffff00;
	}
	.mt.object {
		color: #00ffff;
	}
	.mask {
		padding: 3px 6px;
	}
	.mask input {
		width: 100%;
		box-sizing: border-box;
		height: 24px;
		border: 1px solid #000;
		background: #3e3529;
		color: #fff;
		padding: 0 6px;
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
</style>
