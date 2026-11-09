<script lang="ts">
	import { microbotApi, MicrobotError, type GameView } from '#lib/api/microbot';
	import { itemIcon } from '#lib/progress/icons';
	import {
		bankMenu,
		loopScripts,
		machineRows,
		nearMenu,
		nearRows,
		qtyLabel,
		selectedNearMenu,
		type NearRow
	} from '#lib/progress/jogo';
	import Glyph from '#lib/v6/Glyph.svelte';
	import { gameMap, gameMenu, openMenu, runAction } from './game/menu.svelte';

	/*
	 * O que fica em volta do cliente no cockpit (doc 08): o aviso dos scripts de loop, o que está por
	 * perto (o mesmo "Choose Option" do painel lateral), andar até coordenadas, o script ativo e o
	 * banco aberto com Withdraw-1, 5, 10 e All. Inventário, equipamento, skills e chat ficam no
	 * cliente (SidePanel e Chatbox).
	 */
	let { view }: { view: GameView } = $props();

	const near = $derived(nearRows(view));
	const machines = $derived(machineRows(view));
	const heartbeats = $derived(view.observe?.scripts ?? []);
	const loops = $derived(loopScripts(view));
	const bank = $derived(view.bank);
	const me = $derived(view.observe?.player?.position);
	const short = (c: string) => c.split('.').pop() ?? c;
	const key = (r: NearRow) => `${r.kind}:${r.id}:${r.position?.x}:${r.position?.y}`;
	/** Com algo selecionado ("Use X ->", feitiço de NPC), o menu é o do alvo, como no jogo. */
	const menuOf = (r: NearRow) =>
		gameMenu.selected ? selectedNearMenu(gameMenu.selected, r) : nearMenu(r);

	let walkTo = $state({ x: '', y: '', plane: '0' });
	let pausing = $state(false);
	let armed = $state(false);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	async function pause() {
		pausing = true;
		try {
			await microbotApi.plugin({ path: 'scripts/pause' });
		} catch (e) {
			gameMenu.error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			pausing = false;
			gameMenu.onDone?.();
		}
	}
	function walk(e: SubmitEvent) {
		e.preventDefault();
		const x = Number(walkTo.x);
		const y = Number(walkTo.y);
		if (!x || !y) return;
		void runAction({
			label: 'Walk here',
			target: '',
			tone: 'object',
			command: 'walk',
			body: { x, y, plane: Number(walkTo.plane || 0) }
		});
	}
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

{#if gameMenu.error}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>{gameMenu.error}</span>
		<button class="btn btn-g btn-s" type="button" onclick={() => (gameMenu.error = null)}
			>Fechar</button
		>
	</div>
{/if}
{#if loops.length}
	<div class="band" role="status" title={loops.join(', ')}>
		<Glyph name="alert" /><span
			>Scripts de loop ativos ({loops.length}). Se algum estiver jogando, ele disputa o mouse com as
			ações daqui.</span
		>
		<button class="btn btn-s" type="button" disabled={pausing} onclick={pause}
			>{pausing ? 'Pausando…' : 'Pausar todos os scripts'}</button
		>
	</div>
{/if}

<div class="jrow">
	<section class="panel">
		<div class="plaque">Por perto<span class="r">clique para o menu</span></div>
		<div class="list">
			{#each near as r (key(r))}
				<button
					class="nr"
					type="button"
					onclick={(e) => openMenu(e, menuOf(r))}
					oncontextmenu={(e) => openMenu(e, menuOf(r))}
					title={r.position ? `${r.position.x}, ${r.position.y}, ${r.position.plane}` : undefined}
				>
					<span class="kind">{r.kind}</span><span class="nm">{r.name}</span><span class="d"
						>{r.detail}</span
					><span class="dist">{r.distance ?? '?'}</span>
				</button>
			{:else}
				<p class="su sm0 pad">Nada por perto.</p>
			{/each}
		</div>
		<form class="walk" onsubmit={walk}>
			<span class="k">andar até</span>
			<input class="wnum" placeholder="x" aria-label="x" bind:value={walkTo.x} />
			<input class="wnum" placeholder="y" aria-label="y" bind:value={walkTo.y} />
			<input class="wnum sm" placeholder="0" aria-label="plano" bind:value={walkTo.plane} />
			<button class="btn btn-s" type="submit" disabled={gameMenu.busy || !walkTo.x || !walkTo.y}
				>Andar</button
			>
			<button class="btn btn-s btn-g" type="button" onclick={() => (gameMap.open = true)}
				>Mapa</button
			>
			{#if me}<span class="su tiny">você: {me.x}, {me.y}, {me.plane}</span>{/if}
		</form>
	</section>

	<section class="panel">
		<div class="plaque">Script ativo<span class="r">state machines</span></div>
		<div class="pad stack">
			{#each machines as m (m.name)}
				<div class="kv">
					<span class="k">{m.name}</span><span class="val mono">{m.state}</span><span
						class="su tiny">{m.detail}</span
					>
				</div>
			{/each}
			{#each heartbeats as h (h.scriptClassName)}
				<div class="kv">
					<span class="k">{short(h.scriptClassName)}</span><span class="val"
						>{h.status === 'NO_HEARTBEAT'
							? 'sem heartbeat'
							: (h.status ?? 'ok').toLowerCase()}{h.loopCount
							? ` · ${h.loopCount} loops`
							: ''}</span
					>
				</div>
			{/each}
			{#if !machines.length && !heartbeats.length}
				<span class="su sm0">Nenhum script de loop rodando.</span>
			{/if}
		</div>
	</section>
	<section class="panel">
		<div class="plaque">
			Banco<span class="r"
				>{bank?.open ? `aberto · ${bank.count ?? bank.items?.length ?? 0} itens` : 'fechado'}</span
			>
		</div>
		{#if bank?.open}
			<div class="bank">
				{#each bank.items ?? [] as i (i.id)}
					{@const icon = itemIcon(i.id)}
					{@const q = qtyLabel(i.quantity)}
					<button
						type="button"
						class="bs"
						title={`${i.name} ×${i.quantity.toLocaleString('pt-BR')}`}
						onclick={(e) => openMenu(e, bankMenu(i))}
						oncontextmenu={(e) => openMenu(e, bankMenu(i))}
						>{#if icon}<img src={icon} alt={i.name} />{:else}<span class="gab"
								>{i.name.slice(0, 4)}</span
							>{/if}<span class="gq {q.tone}">{q.text}</span></button
					>
				{/each}
			</div>
			<div class="bbar">
				<button
					class="btn btn-s"
					class:btn-d={armed}
					type="button"
					disabled={gameMenu.busy}
					onclick={depositAll}
					>{armed ? 'Confirmar: depositar o inventário' : 'Depositar o inventário'}</button
				>
				<button
					class="btn btn-g btn-s"
					type="button"
					disabled={gameMenu.busy}
					onclick={() =>
						runAction({ label: 'Close', target: '', tone: 'object', command: 'bank_close' })}
					>Fechar</button
				>
			</div>
		{:else}
			<div class="pad stack">
				<button
					class="btn btn-s self"
					type="button"
					disabled={gameMenu.busy}
					onclick={() =>
						runAction({ label: 'Bank', target: '', tone: 'object', command: 'bank_open' })}
					>Abrir o banco mais perto</button
				>
				<span class="su tiny"
					>Aberto, a grade do banco aparece aqui e o menu dos itens do inventário vira o de
					depósito.</span
				>
			</div>
		{/if}
	</section>
</div>

<style>
	.jrow {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
		gap: 14px;
		align-items: start;
	}
	.pad {
		padding: 12px;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.self {
		align-self: flex-start;
	}
	.bank {
		display: grid;
		grid-template-columns: repeat(auto-fill, 44px);
		grid-auto-rows: 38px;
		gap: 2px;
		max-height: 280px;
		overflow: auto;
		margin: 10px 12px 0;
		padding: 6px;
		background: var(--frame);
		border-radius: 3px;
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
	.bbar {
		display: flex;
		gap: 8px;
		padding: 10px 12px 12px;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.list {
		max-height: 260px;
		overflow: auto;
	}
	.nr {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		box-sizing: border-box;
		padding: 6px 12px;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-size: 12px;
		text-align: left;
		cursor: pointer;
	}
	.nr:hover {
		background: var(--gold-tint);
	}
	.nr .kind {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-subtle);
		width: 46px;
		flex: 0 0 auto;
	}
	.nr .nm {
		font-weight: 600;
	}
	.nr .d {
		flex: 1;
		min-width: 0;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nr .dist {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.walk {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
		padding: 10px 12px;
	}
	.wnum {
		width: 64px;
		height: 30px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.wnum.sm {
		width: 36px;
	}
	.kv {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.val {
		font-size: 13px;
		font-weight: 500;
	}
	.mono {
		font-family: 'JetBrains Mono', monospace;
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
	.tiny {
		font-size: 11px;
		margin: 0;
	}
</style>
