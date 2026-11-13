<script lang="ts">
	import type { GameView } from '#lib/api/microbot';
	import { nearMenu, nearRows, selectedNearMenu, type NearRow } from '#lib/progress/jogo';
	import Glyph from '#lib/v6/Glyph.svelte';
	import { gameMap, gameMenu, openMenu, runAction } from './game/menu.svelte';

	/*
	 * Por perto (V7Cliente1440 e V7Cliente640): NPCs, objetos e itens do chão do mais perto ao mais
	 * longe, cada um com o "Choose Option" do jogo; embaixo, andar até coordenadas ou pelo mapa.
	 * Com algo selecionado ("Use X ->", feitiço de NPC), o menu é o do alvo, como no jogo.
	 */
	let { view, walk = true }: { view: GameView; walk?: boolean } = $props();

	const LIMIT = 8;
	let all = $state(false);
	const rows = $derived(nearRows(view));
	const shown = $derived(all ? rows : rows.slice(0, LIMIT));
	const me = $derived(view.observe?.player?.position);
	const KIND: Record<NearRow['kind'], string> = { npc: 'npc', objeto: 'objeto', chão: 'chão' };
	const key = (r: NearRow) => `${r.kind}:${r.id}:${r.position?.x}:${r.position?.y}`;
	const menuOf = (r: NearRow) =>
		gameMenu.selected ? selectedNearMenu(gameMenu.selected, r) : nearMenu(r);

	let to = $state({ x: '', y: '' });
	function go(e: SubmitEvent) {
		e.preventDefault();
		const x = Number(to.x);
		const y = Number(to.y);
		if (!x || !y) return;
		void runAction({
			label: 'Walk here',
			target: '',
			tone: 'object',
			command: 'walk',
			body: { x, y, plane: me?.plane ?? 0 }
		});
	}
</script>

<section class="card">
	<div class="h hb">
		<h2>Por perto</h2>
		<span>o menu de cada um</span>
	</div>
	{#each shown as r (key(r))}
		<div
			class="near"
			title={[r.detail, r.position ? `${r.position.x}, ${r.position.y}` : '']
				.filter(Boolean)
				.join(' · ') || undefined}
		>
			<span class="kd">{KIND[r.kind]}</span><b>{r.name}</b><span class="ds"
				>{r.distance ?? '?'}</span
			><button
				class="ib ib-g"
				type="button"
				aria-label="Menu de {r.name}"
				onclick={(e) => openMenu(e, menuOf(r))}
				oncontextmenu={(e) => openMenu(e, menuOf(r))}><Glyph name="more" size={13} /></button
			>
		</div>
	{:else}
		<p class="none mu">Nada por perto.</p>
	{/each}
	{#if rows.length > LIMIT}
		<button class="lnk more" type="button" onclick={() => (all = !all)}
			>{all ? 'mostrar menos' : `mais ${rows.length - LIMIT} por perto`}</button
		>
	{/if}
	{#if walk}
		<form class="walk" onsubmit={go}>
			<span class="lbl">Andar até</span>
			<label class="vh" for="walk-x">x</label>
			<input id="walk-x" class="in7 w" inputmode="numeric" placeholder="x" bind:value={to.x} />
			<label class="vh" for="walk-y">y</label>
			<input id="walk-y" class="in7 w" inputmode="numeric" placeholder="y" bind:value={to.y} />
			<button class="btn btn-s" type="submit" disabled={gameMenu.busy || !to.x || !to.y}
				>Andar</button
			>
			<button class="btn btn-s btn-g" type="button" onclick={() => (gameMap.open = true)}
				>Mapa</button
			>
		</form>
	{/if}
</section>

<style>
	.hb {
		padding-bottom: 10px;
	}
	.near {
		cursor: default;
	}
	.none {
		margin: 0;
		padding: 0 16px 14px;
		font-size: 13px;
	}
	.more {
		align-self: flex-start;
		margin: 0 14px 8px;
		min-height: 32px;
		padding: 0;
		border: 0;
		background: none;
		cursor: pointer;
		font-size: 12px;
	}
	.walk {
		display: flex;
		gap: 6px;
		align-items: center;
		flex-wrap: wrap;
		padding: 10px 14px 14px;
		border-top: 1px solid var(--hairline);
	}
	.walk .lbl {
		font-size: 12px;
		font-weight: 600;
		color: var(--ink-subtle);
	}
	.w {
		width: 60px;
	}
</style>
