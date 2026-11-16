<script lang="ts">
	import { resolve } from '$app/paths';
	import { microbotApi, MicrobotError } from '#lib/api/microbot';
	import { cardStatus, findInstalled, type Card, type CardRow } from '#lib/progress/clientplugins';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * O cartão Plugins da aba Cliente (V7ClientePlugins1440, V7ClientePlugins640 e os estados de
	 * V7ClientePluginsEstados1440), no lugar do antigo cartão Script: liga, para e abre a
	 * configuração sem sair do jogo. O desvio do perfil aparece como selo na linha e como contagem no
	 * título; salvar ou descartar fica na gaveta. Ligar e parar pedem um segundo clique; Pausar todos
	 * é do Agent Server, que pausa todos os loops do cliente.
	 */
	let {
		card,
		client,
		paused,
		open = null,
		onopen,
		onchange,
		onstart,
		onlogin
	}: {
		card: Card | null;
		/** none: sem cliente; stopped: o cliente parado; login: na tela de login; game: no jogo. */
		client: 'none' | 'stopped' | 'login' | 'game';
		paused: boolean;
		/** A classe aberta na gaveta. */
		open?: string | null;
		onopen: (className: string) => void;
		onchange?: () => void;
		onstart?: () => void;
		onlogin?: () => void;
	} = $props();

	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let starting = $state<string | null>(null);
	let error = $state<string | null>(null);
	let q = $state('');
	let alwaysOpen = $state(false);
	let showClient = $state(false);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	const status = $derived(card ? cardStatus(card, paused) : { text: '' });
	const found = $derived(card ? findInstalled(card, q) : []);
	const inGame = $derived(client === 'game');
	/** Sempre ligados abre sozinho quando um deles tem desvio. */
	const alwaysShown = $derived(alwaysOpen || !!card?.always.some((r) => r.badges.length));

	// "ligando…" até o plugin aparecer ativo, ou 30 s.
	$effect(() => {
		const s = starting;
		if (!s) return;
		if (
			card &&
			[...card.plays, ...card.routines, ...card.always, ...card.client].some(
				(r) => r.className === s && r.active
			)
		) {
			starting = null;
			return;
		}
		const t = setTimeout(() => (starting = null), 30_000);
		return () => clearTimeout(t);
	});

	async function act(key: string, run: () => Promise<unknown>, confirm = false) {
		if (busy) return;
		if (confirm && armed !== key) {
			armed = key;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = key;
		error = null;
		try {
			await run();
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			onchange?.();
		}
	}
	const start = (r: CardRow) =>
		act(
			`start:${r.className}`,
			async () => {
				await microbotApi.plugin({ path: 'scripts/start', body: { className: r.className } });
				starting = r.className;
				q = '';
			},
			true
		);
	const stop = (r: CardRow) =>
		act(
			`stop:${r.className}`,
			() => microbotApi.plugin({ path: 'scripts/stop', body: { className: r.className } }),
			true
		);
	const pauseAll = () =>
		act('pause', () => microbotApi.plugin({ path: paused ? 'scripts/resume' : 'scripts/pause' }));
</script>

{#snippet row(r: CardRow)}
	{@const startKey = `start:${r.className}`}
	{#if armed === startKey}
		<div class="pr on arm">
			<span class="lamp off"></span>
			<span class="x"><b>{r.name}</b><span>age no jogo com a conta logada</span></span>
			<span class="ac">
				<button class="btn btn-s btn-p" type="button" onclick={() => start(r)}
					><Glyph name="play" size={12} />Confirmar: ligar</button
				>
				<button class="btn btn-s btn-g" type="button" onclick={() => (armed = null)}
					>Cancelar</button
				>
			</span>
		</div>
	{:else}
		{@const isStarting = starting === r.className && !r.active}
		<div class="pr" class:on={open === r.className} class:dim={r.protected}>
			{#if r.protected}
				<span class="lock" title="protegido"><Glyph name="lock" size={14} /></span>
			{:else}
				<span class="lamp {isStarting ? 'up' : r.lamp}"></span>
			{/if}
			<span class="x"
				><b>{r.name}</b><span class:bad={r.tone === 'bad'} class:up={isStarting}
					>{isStarting ? 'ligando…' : r.line}</span
				>{#if r.badges.length}<span class="bd"
						>{#each r.badges as b (b.text)}<span class="badge badge-stale" title={b.title}
								>{b.text}</span
							>{/each}</span
					>{/if}</span
			>
			{#if !r.protected}
				<span class="ac">
					<button
						class="ib"
						type="button"
						aria-label="Configurar {r.name}"
						title="Configurar"
						aria-pressed={open === r.className}
						disabled={client === 'none' || client === 'stopped'}
						onclick={() => onopen(r.className)}><Glyph name="config" size={14} /></button
					>
					{#if r.active || isStarting}
						{@const stopKey = `stop:${r.className}`}
						{#if armed === stopKey}
							<button class="btn btn-s btn-d" type="button" onclick={() => stop(r)}
								>Confirmar: parar</button
							>
						{:else}
							<button
								class="ib ib-d"
								type="button"
								aria-label="Parar {r.name}"
								title="Parar (pede um segundo clique)"
								disabled={!!busy}
								onclick={() => stop(r)}><Glyph name="stop" size={12} /></button
							>
						{/if}
					{:else}
						<button
							class="ib"
							type="button"
							aria-label="Ligar {r.name}"
							title={inGame ? 'Ligar (pede um segundo clique)' : 'Entre no jogo primeiro'}
							disabled={!!busy || !inGame || !!starting}
							onclick={() => start(r)}><Glyph name="play" size={12} /></button
						>
					{/if}
				</span>
			{/if}
		</div>
	{/if}
{/snippet}

<section class="card pc" aria-label="Plugins do cliente">
	<div class="h">
		<h2>Plugins</h2>
		{#if status.text}<span class:dv={status.tone === 'stale'} class:red={status.tone === 'bad'}
				>{status.text}</span
			>{/if}
	</div>

	{#if client === 'none'}
		<div class="msg">
			<p><b>Nenhum cliente ligado a esta conta.</b></p>
			<p class="s">Sem cliente não há plugins. O worker sobe pela Visão geral.</p>
		</div>
	{:else if client === 'stopped'}
		<div class="msg">
			<p><b>O cliente está parado.</b></p>
			<p class="s">
				Ao iniciar, ele liga o que o perfil da conta pede. Configurar pede o cliente rodando.
			</p>
			<div class="a">
				{#if onstart}<button class="btn btn-s" type="button" onclick={onstart}
						><Glyph name="play" size={12} />Iniciar o cliente</button
					>{/if}
				<a class="lnk" href={resolve('/plugins')}>ver o perfil em Plugins</a>
			</div>
		</div>
	{:else}
		{#if client === 'login'}
			<div class="msg">
				<p><b>O cliente está na tela de login.</b></p>
				<p class="s">Ligar um plugin pede a conta no jogo. A configuração dá para mudar.</p>
				{#if onlogin}<div class="a">
						<button class="btn btn-s" type="button" onclick={onlogin}>Entrar no jogo</button>
					</div>{/if}
			</div>
		{/if}
		{#if paused && inGame}
			<div class="paused">
				<span><b>Pausados</b></span>
				<button class="btn btn-s" type="button" disabled={!!busy} onclick={pauseAll}
					><Glyph name="play" size={12} />{busy === 'pause'
						? 'Retomando…'
						: 'Retomar todos'}</button
				>
			</div>
		{/if}
		<div class="tool">
			<label class="fs" class:focus={!!q.trim()}>
				<Glyph name="search" size={14} />
				<input
					type="text"
					placeholder="ligar outro instalado"
					aria-label="Ligar outro plugin instalado"
					bind:value={q}
				/>
			</label>
			{#if !(paused && inGame)}
				<button
					class="btn btn-s"
					type="button"
					title="O Agent Server pausa todos os loops do cliente"
					disabled={!!busy || !inGame || !card?.plays.some((r) => r.active)}
					onclick={pauseAll}><Glyph name="pause" size={12} />Pausar todos</button
				>
			{/if}
			{#if q.trim()}
				<div class="res" role="listbox" aria-label="Instalados">
					{#each found as r (r.className)}
						{@render row(r)}
					{:else}
						<p class="none">Nenhum instalado com “{q.trim()}”.</p>
					{/each}
					<div class="rf">Instalar outro: <a href={resolve('/plugins')}>Plugin Hub</a></div>
				</div>
			{/if}
		</div>

		{#if error}<p class="err" role="alert">{error}</p>{/if}

		{#if card}
			{#if card.plays.length}
				<div class="gh">Jogam</div>
				{#each card.plays as r (r.className)}{@render row(r)}{/each}
			{/if}
			{#if card.routines.length}
				<div class="gh">Rotinas usam</div>
				{#each card.routines as r (r.className)}{@render row(r)}{/each}
			{/if}
			{#if card.always.length}
				{@const on = card.always.filter((r) => r.active).length}
				{@const drift = card.always.filter((r) => r.badges.length).length}
				<button
					class="gh fold"
					type="button"
					aria-expanded={alwaysShown}
					onclick={() => (alwaysOpen = !alwaysShown)}
					><span class="car" class:on={alwaysShown}><Glyph name="chevron" size={12} /></span>Sempre
					ligados · {card.always.length}<span class="r"
						>{#if drift}<span class="badge badge-stale"
								>{drift} {drift === 1 ? 'desvio' : 'desvios'}</span
							>{:else}{#each card.always.filter((r) => r.active) as r (r.className)}<span
									class="lamp ok"
								></span>{/each}
							{on ? 'ligados' : ''}{/if}</span
					></button
				>
				{#if alwaysShown}
					{#each card.always as r (r.className)}{@render row(r)}{/each}
				{/if}
			{/if}
			{#if showClient && card.client.length}
				<div class="gh">Do cliente</div>
				{#each card.client as r (r.className)}{@render row(r)}{/each}
			{/if}
			<div class="cf">
				{#if card.client.length}
					<button
						class="tog"
						type="button"
						aria-pressed={showClient}
						title="Os plugins que vêm com o cliente ficam escondidos"
						onclick={() => (showClient = !showClient)}
						>{showClient ? 'Esconder' : 'Mostrar'} os do cliente · {card.client.length}</button
					>
				{/if}
				{#if card.unchecked || card.missing.length}
					<span
						class="mono su tiny"
						title="Do perfil: plugins do Hub que o cliente não tem, e plugins do RuneLite que o servidor não consegue conferir"
						>{[
							card.missing.length ? `${card.missing.length} do perfil sem instalar` : '',
							card.unchecked ? `${card.unchecked} do RuneLite sem conferir` : ''
						]
							.filter(Boolean)
							.join(' · ')}</span
					>
				{/if}
				<a class="lnk" href={resolve('/plugins')}>Hub e versões</a>
			</div>
		{:else}
			<p class="mu wait" aria-busy="true">Lendo os plugins…</p>
		{/if}
	{/if}
</section>

<style>
	.pc > :global(.h) {
		padding-bottom: 10px;
	}
	.pc .h .dv {
		color: var(--stale);
		font-weight: 500;
	}
	.pc .h .red {
		color: var(--not-started);
		font-weight: 500;
	}
	.tool {
		position: relative;
		display: flex;
		gap: 6px;
		align-items: center;
		padding: 0 14px 12px;
	}
	.fs {
		flex: 1 1 auto;
		display: flex;
		align-items: center;
		gap: 8px;
		height: 32px;
		min-width: 0;
		padding: 0 10px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		box-sizing: border-box;
		color: var(--ink-subtle);
	}
	.fs.focus {
		border-color: var(--gold-ink);
		box-shadow:
			var(--bevel),
			0 0 0 2px var(--gold-tint);
	}
	.fs input {
		flex: 1;
		min-width: 0;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--ink);
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
	}
	.res {
		position: absolute;
		left: 14px;
		right: 14px;
		top: 38px;
		z-index: 3;
		display: flex;
		flex-direction: column;
		border-radius: 5px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: 0 6px 16px rgba(43, 33, 20, 0.16);
	}
	.res .pr {
		padding-left: 12px;
	}
	.res .none {
		margin: 0;
		padding: 10px 12px;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.res .rf {
		padding: 8px 12px;
		border-top: 1px solid var(--hairline);
		font-size: 12px;
		color: var(--ink-muted);
	}
	.gh {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 30px;
		padding: 6px 14px 2px;
		border: 0;
		border-top: 1px solid var(--hairline);
		background: transparent;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		text-align: left;
		width: 100%;
		box-sizing: border-box;
	}
	.gh.fold {
		min-height: 44px;
		padding-block: 0;
		cursor: pointer;
	}
	.gh .car {
		display: inline-flex;
	}
	.gh .car.on {
		transform: rotate(90deg);
	}
	.gh .r {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 5px;
		text-transform: none;
		letter-spacing: 0;
		font-size: 11px;
	}
	.pr {
		display: grid;
		grid-template-columns: 9px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		min-height: 52px;
		padding: 6px 10px 6px 14px;
		font-size: 13px;
	}
	.pr + .pr {
		border-top: 1px solid var(--hairline);
	}
	.pr > .lamp,
	.pr > .lock {
		align-self: start;
		margin-top: 4px;
	}
	.pr > .lock {
		margin-top: 2px;
		color: var(--ink-subtle);
		display: inline-flex;
	}
	.pr .x {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
		line-height: 1.3;
	}
	.pr .x b {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pr .x > span {
		font-size: 12px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pr .x > span.bad {
		color: var(--not-started);
		font-weight: 500;
	}
	.pr .x > span.up {
		color: var(--auto);
	}
	.pr .x > .bd {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
		padding-top: 4px;
	}
	.pr .ac {
		display: flex;
		gap: 4px;
		align-items: center;
	}
	.pr.on {
		background: var(--gold-tint);
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.pr.dim .x b {
		color: var(--ink-muted);
	}
	.pr.arm {
		grid-template-columns: 9px minmax(0, 1fr);
		row-gap: 8px;
		padding-bottom: 12px;
	}
	.pr.arm .ac {
		grid-column: 2;
	}
	.ib-d {
		color: var(--not-started);
	}
	.ib:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.ib[aria-pressed='true'] {
		border-color: var(--gold-ink);
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.cf {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		padding: 8px 14px 12px;
		border-top: 1px solid var(--hairline);
	}
	.cf .tog {
		min-height: 32px;
		padding: 0 4px;
	}
	.cf .lnk {
		margin-left: auto;
	}
	.lnk {
		font-size: 12px;
		color: var(--gold-ink);
	}
	.tiny {
		font-size: 11px;
	}
	.msg {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 0 14px 14px;
		font-size: 13px;
		line-height: 1.45;
	}
	.msg p {
		margin: 0;
	}
	.msg b {
		font-weight: 600;
	}
	.msg .s {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.msg .a {
		display: flex;
		gap: 10px;
		align-items: center;
		flex-wrap: wrap;
	}
	.paused {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		margin: 0 14px 10px;
		padding: 8px 10px;
		border-radius: 4px;
		background: var(--gold-tint);
		border: 1px solid var(--gold-ink);
		font-size: 12px;
	}
	.paused b {
		font-weight: 600;
	}
	.paused .btn {
		margin-left: auto;
	}
	.err {
		margin: 0;
		padding: 0 14px 8px;
		font-size: 12px;
		color: var(--not-started);
	}
	.wait {
		margin: 0;
		padding: 0 14px 14px;
		font-size: 12px;
	}
</style>
