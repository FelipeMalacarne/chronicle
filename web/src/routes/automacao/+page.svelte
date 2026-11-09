<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		microbotApi,
		MicrobotError,
		type AuditEntry,
		type MicrobotAction,
		type MicrobotStatus
	} from '#lib/api/microbot';
	import { auditLine, auditWho, cards, stateRows } from '#lib/progress/automacao';
	import Glyph from '#lib/v6/Glyph.svelte';
	import VncView, { type VncPhase } from '#lib/v6/VncView.svelte';
	import JogoView from '#lib/v6/JogoView.svelte';
	import SidePanel from '#lib/v6/game/SidePanel.svelte';
	import Chatbox from '#lib/v6/game/Chatbox.svelte';
	import GameMenu from '#lib/v6/game/GameMenu.svelte';
	import WorldMap from '#lib/v6/game/WorldMap.svelte';
	import { cancelSelection, gameMap, gameMenu } from '#lib/v6/game/menu.svelte';
	import { selectedLabel } from '#lib/progress/jogo';
	import { miscIcon } from '#lib/progress/icons';
	import ClientLog from '#lib/v6/ClientLog.svelte';
	import ClientAccount from '#lib/v6/ClientAccount.svelte';
	import Relogios from '#lib/v6/Relogios.svelte';
	import type { GameView } from '#lib/api/microbot';

	/*
	 * Automação › Cockpit (V6Microbot1440; no estreito, o modo só dashboard do V6Microbot640), doc 08:
	 * o estado do cliente pelo agent, a tela por screenshot a cada 5 s ou ao vivo por VNC (fase 2, só
	 * no largo e com o cliente headless), iniciar e parar o cliente, login e logout, e a auditoria. Os
	 * plugins (fase 3) têm a página deles; Rotinas ficam para depois das receitas.
	 */
	let status = $state<MicrobotStatus | null>(null);
	let off = $state(false);
	let loadError = $state<string | null>(null);
	let audit = $state<AuditEntry[]>([]);
	let shotAt = $state(0);
	let shotOk = $state(false);
	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let world = $state('');
	let game = $state<GameView | null>(null);
	let live = $state(false);
	let intervene = $state(false);
	let vnc = $state<{ phase: VncPhase; reason?: string }>({ phase: 'closed' });
	let wide = $state(true);

	const client = $derived(status?.client);
	const running = $derived(!!client?.running);
	const loggedIn = $derived(!!client?.state?.loggedIn);
	const facts = $derived(cards(status));
	const rows = $derived(stateRows(status));
	const pos = $derived(client?.state?.player?.position);
	const dev = $derived(status?.dev ?? true);
	/** O VNC só existe com o x11vnc do headless e fora do celular (V6Microbot640: sem websocket). */
	const vncReady = $derived(running && !!client?.vnc && wide);
	const showLive = $derived(live && vncReady);
	const vncURL = () =>
		`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/microbot/vnc`;
	function setLive(on: boolean) {
		live = on;
		intervene = false;
	}
	function onVnc(phase: VncPhase, reason?: string) {
		vnc = { phase, reason };
		if (phase === 'closed') intervene = false;
		if (phase !== 'connecting') void refreshAudit();
	}
	$effect(() => {
		const mq = matchMedia('(min-width: 700px)');
		wide = mq.matches;
		const on = () => (wide = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});

	async function refresh() {
		try {
			const s = await microbotApi.status();
			if (s === null) {
				off = true;
				return;
			}
			status = s;
			loadError = null;
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		}
	}
	async function refreshAudit() {
		try {
			audit = (await microbotApi.audit(30)).entries;
		} catch {
			/* a auditoria some se o servidor cair; o estado já mostra o erro */
		}
	}
	function shoot() {
		if (running) shotAt = Date.now();
	}

	$effect(() => {
		void refresh();
		void refreshAudit();
		const st = setInterval(() => {
			if (!document.hidden) void refresh();
		}, 3000);
		const au = setInterval(() => {
			if (!document.hidden) void refreshAudit();
		}, 30000);
		return () => {
			clearInterval(st);
			clearInterval(au);
		};
	});
	// O jogo (inventário, skills, por perto, diálogo...) a cada 3 s enquanto a conta está logada.
	// Uma leitura leva alguns segundos (várias do Agent Server e da ponte): uma por vez, senão as
	// leituras se acumulam e esgotam as conexões do navegador com o servidor.
	let gameBusy = false;
	async function refreshGame() {
		if (gameBusy) return;
		gameBusy = true;
		try {
			game = await microbotApi.game();
		} catch {
			/* o estado do cliente já mostra o erro */
		} finally {
			gameBusy = false;
		}
	}
	// Depois de cada ação do menu, o jogo e a auditoria são relidos.
	$effect(() => {
		gameMenu.onDone = () => {
			void refreshGame();
			void refreshAudit();
		};
		return () => {
			gameMenu.onDone = null;
		};
	});
	const inGame = $derived(loggedIn && !!game);
	$effect(() => {
		if (!running || !loggedIn) {
			game = null;
			return;
		}
		void refreshGame();
		const t = setInterval(() => {
			if (!document.hidden) void refreshGame();
		}, 3000);
		return () => clearInterval(t);
	});

	// Screenshot a cada 5 s enquanto o cliente roda, a aba está à vista e o VNC não está aberto.
	$effect(() => {
		if (!running || showLive) return;
		shoot();
		const t = setInterval(() => {
			if (!document.hidden) shoot();
		}, 5000);
		return () => clearInterval(t);
	});

	const LABEL: Record<MicrobotAction, string> = {
		'client/start': 'Iniciar cliente',
		'client/stop': 'Parar cliente',
		login: 'Login',
		logout: 'Logout',
		world: 'Trocar de mundo'
	};
	/** Ações que mexem na conta pedem um segundo clique em até 4 s. */
	const CONFIRM: MicrobotAction[] = ['client/stop', 'login', 'logout', 'world'];
	let disarm: ReturnType<typeof setTimeout> | undefined;

	async function act(a: MicrobotAction) {
		if (busy) return;
		if (a === 'world' && !(Number(world) > 0)) {
			failure = 'Escreva o mundo para onde trocar.';
			return;
		}
		if (CONFIRM.includes(a) && armed !== a) {
			armed = a;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = a;
		failure = null;
		try {
			const w = Number(world);
			await microbotApi.action(
				a,
				(a === 'login' || a === 'world') && w > 0 ? { world: w } : undefined
			);
		} catch (e) {
			failure = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			await refresh();
			await refreshAudit();
		}
	}
	const label = (a: MicrobotAction) =>
		busy === a ? `${LABEL[a]}…` : armed === a ? `Confirmar ${LABEL[a].toLowerCase()}` : LABEL[a];
	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
</script>

<svelte:head>
	<title>Automação · Chronicle</title>
</svelte:head>

{#snippet button(a: MicrobotAction, danger = false)}
	<button
		class="btn btn-s"
		class:btn-d={danger || armed === a}
		type="button"
		disabled={!!busy}
		onclick={() => act(a)}>{label(a)}</button
	>
{/snippet}

<header class="mh">
	<div class="ttl">
		<p class="ey">Microbot · controle do cliente · doc 08</p>
		<h1 class="sr">Cockpit</h1>
	</div>
	<div class="seg" role="tablist" aria-label="Automação">
		<a class="on" href={resolve('/automacao')} aria-current="page">Cockpit</a>
		<a href={resolve('/automacao/plugins')}>Plugins</a>
		<a href="#auditoria">Auditoria</a>
	</div>
	<div class="seg modes" role="group" aria-label="Modo">
		<button
			type="button"
			class:on={!showLive}
			aria-pressed={!showLive}
			onclick={() => setLive(false)}>só dashboard</button
		>
		<button
			type="button"
			class:on={showLive}
			aria-pressed={showLive}
			disabled={!vncReady}
			title={vncReady
				? 'A tela ao vivo pelo agent; mouse e teclado só depois de "Intervir"'
				: !wide
					? 'Sem VNC no celular: só dashboard'
					: 'O VNC precisa do cliente headless rodando no zaros'}
			onclick={() => setLive(true)}>VNC ao vivo</button
		>
	</div>
	<span
		class="badge badge-gold"
		title={dev
			? 'Em AUTH_MODE=dev o controle só responde a esta máquina'
			: 'O papel microbot:control do Zitadel libera esta tela'}
		>{dev ? 'dev · só desta máquina' : 'papel microbot:control'}</span
	>
</header>

{#if off}
	<section class="panel pad">
		<p class="sm0">
			O controle do Microbot está desligado neste servidor. Ligue com <code
				>MICROBOT_CONTROL=on</code
			>
			no <code>serve</code> e rode <code>ironman agent</code> na máquina do jogo.
		</p>
	</section>
{:else}
	<div class="panel strip">
		{#each facts as c (c.k)}
			<div class="kv" title={c.title}>
				<span class="k">{c.k}</span><span class="val"
					><span class="dot" class:dot-w={c.tone === 'warn'} class:dot-off={c.tone === 'off'}
					></span>{c.v}</span
				>
			</div>
		{:else}
			<span class="su sm0" aria-busy="true">{loadError ?? 'Carregando…'}</span>
		{/each}
		<div class="acts">
			{#if status?.agent.connected}
				<button class="btn btn-s" type="button" disabled={!running} onclick={shoot}
					>Screenshot</button
				>
				{#if running}
					<label class="wl"
						><span class="vh">Mundo</span><input
							class="world"
							inputmode="numeric"
							placeholder="mundo"
							bind:value={world}
						/></label
					>
					{#if loggedIn}
						{@render button('world')}
						{@render button('logout')}
					{:else}
						{@render button('login')}
					{/if}
					{@render button('client/stop', true)}
				{:else}
					{@render button('client/start')}
				{/if}
			{/if}
		</div>
	</div>

	{#if failure}
		<div class="band" role="alert"><Glyph name="alert" /><span>{failure}</span></div>
	{/if}

	<div class="cols">
		<div class="main">
			<section class="panel">
				<div class="plaque">
					{inGame ? 'Jogo' : showLive ? 'Tela ao vivo · VNC' : 'Tela do jogo'}<span class="r"
						>{showLive
							? vnc.phase === 'connected'
								? intervene
									? 'você no controle'
									: 'só olhando'
								: vnc.phase === 'connecting'
									? 'conectando…'
									: (vnc.reason ?? 'desconectado')
							: running
								? 'screenshot a cada 5 s'
								: 'cliente parado'}{client?.headless ? ' · headless' : ''}</span
					>
				</div>
				<div class="pad">
					<div class="client" class:game={inGame}>
						<div class="vp">
							<div class="shot">
								{#if showLive}
									<VncView url={vncURL()} viewOnly={!intervene} onphase={onVnc} />
									{#if vnc.phase !== 'connected'}
										<div class="ph">
											<Glyph name="automacao" size={28} />
											<span
												>{vnc.phase === 'connecting'
													? 'Abrindo o VNC pelo agent…'
													: `VNC fechado${vnc.reason ? `: ${vnc.reason}` : ''}`}</span
											>
										</div>
									{/if}
								{:else if running && shotAt}
									<img
										src={microbotApi.screenshotURL(shotAt)}
										alt="Tela do cliente do Microbot"
										onload={() => (shotOk = true)}
										onerror={() => (shotOk = false)}
									/>
									<div class="hud" class:hidden={!!gameMenu.selected}>
										{#if client?.state?.player?.name}<span>{client.state.player.name}</span>{/if}
										{#if client?.login?.currentWorld}<span>mundo {client.login.currentWorld}</span
											>{/if}
										{#if pos}<span>{pos.x}, {pos.y}, {pos.plane}</span>{/if}
										<span>só dashboard</span>
									</div>
								{/if}
								{#if !showLive && (!running || !shotOk)}
									<div class="ph">
										<Glyph name="automacao" size={28} />
										<span
											>{!status?.agent.connected
												? 'Sem agent: rode `ironman agent` no zaros'
												: running
													? 'Carregando a tela…'
													: 'Cliente parado: “Iniciar cliente” sobe o headless no zaros'}</span
										>
									</div>
								{/if}
								{#if inGame && game}
									{#if gameMenu.selected}
										<!-- O texto do topo do jogo enquanto algo está selecionado. -->
										<div class="uptext" role="status">
											<span>{selectedLabel(gameMenu.selected)}</span>
											<button type="button" onclick={cancelSelection}>Cancel</button>
										</div>
									{/if}
									<button
										class="orb"
										type="button"
										title="Mapa do mundo: clique num lugar para andar até ele"
										aria-label="Mapa do mundo"
										onclick={() => (gameMap.open = true)}
										><img src={miscIcon('world_map')} alt="" /></button
									>
								{/if}
							</div>
							{#if inGame && game}<Chatbox view={game} />{/if}
						</div>
						{#if inGame && game}<SidePanel view={game} />{/if}
					</div>
					{#if showLive}
						<div class="under">
							<span class="su tiny"
								>{intervene
									? 'Mouse e teclado vão para o jogo. Um script rodando disputa o input com você.'
									: 'Abrir o VNC fica na auditoria.'}</span
							>
							<button
								class="btn btn-s"
								class:btn-g={!intervene}
								type="button"
								disabled={vnc.phase !== 'connected'}
								onclick={() => (intervene = !intervene)}
								>{intervene ? 'Devolver: só olhar' : 'Intervir com mouse e teclado'}</button
							>
						</div>
					{/if}
				</div>
			</section>

			{#if inGame && game}
				<JogoView view={game} />
			{/if}

			{#if status?.agent.connected}
				<ClientLog />
			{/if}

			<section class="panel">
				<div class="plaque">Estado · GET /state<span class="r">a cada 3 s</span></div>
				<div class="pad grid2">
					{#each rows as r (r.k)}
						<div class="kv"><span class="k">{r.k}</span><span class="val mono">{r.v}</span></div>
					{:else}
						<span class="su sm0"
							>{running ? 'Sem estado ainda.' : 'O cliente não está rodando.'}</span
						>
					{/each}
				</div>
			</section>
		</div>

		<aside class="side-col">
			{#if status?.agent.connected}
				<Relogios {running} />
			{/if}
			<section class="panel" id="auditoria">
				<div class="plaque">Auditoria<span class="r">quem · quando · o quê</span></div>
				<div class="pad">
					{#each audit as e (e.id)}
						<div class="log" class:fail={!e.ok}>
							<span class="t">{hhmm(e.at)}</span><span class="who">{auditWho(e)}</span><span
								>{auditLine(e)}</span
							>
						</div>
					{:else}
						<span class="su sm0">Nenhuma ação ainda.</span>
					{/each}
				</div>
			</section>
			{#if status?.agent.connected}
				<ClientAccount {running} onchange={refreshAudit} />
			{/if}
			<p class="su tiny">
				Cliente de terceiros, fora do planner. Parar tudo: Ctrl+C no <code>ironman agent</code> no zaros.
				Login derruba o mesmo personagem de outro cliente.
			</p>
		</aside>
	</div>
{/if}

<GameMenu />
{#if gameMap.open}<WorldMap me={game?.observe?.player?.position ?? pos} />{/if}

<style>
	.mh {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: 6px;
	}
	.ttl h1 {
		font-size: 24px;
	}
	.modes {
		margin-left: auto;
	}
	.seg a {
		display: inline-flex;
		align-items: center;
		height: 32px;
		padding: 0 10px;
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		color: var(--ink-muted);
		text-decoration: none;
		background: var(--elevated);
	}
	.seg a + a {
		border-left: 1px solid var(--border);
	}
	.seg a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.seg button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.kv {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.kv .k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.kv .val {
		font-size: 13px;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
	}
	.kv .val.mono {
		font-size: 12px;
	}
	.strip {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 24px;
		padding: 10px 16px;
	}
	.strip .dot {
		display: inline-block;
		margin-right: 6px;
	}
	.acts {
		margin-left: auto;
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		align-items: center;
	}
	.world {
		width: 76px;
		height: 32px;
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
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.main {
		flex: 999 1 520px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.side-col {
		flex: 1 1 300px;
		max-width: 360px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.pad {
		padding: 12px;
	}
	.grid2 {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 10px 16px;
	}
	.client {
		display: flex;
		gap: 10px;
		align-items: stretch;
	}
	.vp {
		flex: 1 1 auto;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	@media (max-width: 900px) {
		.client.game {
			flex-direction: column;
		}
	}
	.shot {
		position: relative;
		aspect-ratio: 765 / 503;
		background: var(--frame);
		border-radius: 4px;
		overflow: hidden;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.shot img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: contain;
	}
	.under {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
		justify-content: space-between;
		padding-top: 10px;
	}
	.shot .ph {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		color: var(--on-frame-muted);
		font-size: 12px;
		text-align: center;
		padding: 0 16px;
	}
	.shot .hud {
		position: absolute;
		left: 10px;
		top: 10px;
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.shot .hud.hidden {
		display: none;
	}
	/* O texto do topo do jogo: "Use Lobster ->" em branco sobre a tela, com sombra preta. */
	.uptext {
		position: absolute;
		left: 8px;
		top: 6px;
		display: flex;
		align-items: baseline;
		gap: 10px;
		font:
			700 13px/1.2 'JetBrains Mono',
			monospace;
		color: #fff;
		text-shadow: 1px 1px 0 #000;
	}
	.uptext button {
		border: 0;
		background: none;
		padding: 0;
		font: inherit;
		color: #ff9040;
		text-shadow: 1px 1px 0 #000;
		text-decoration: underline;
		cursor: pointer;
	}
	/* O orb do mapa do mundo, no canto do minimapa do jogo. */
	.orb {
		position: absolute;
		right: 8px;
		top: 8px;
		width: 38px;
		height: 38px;
		border-radius: 50%;
		border: 2px solid #3e3529;
		background: radial-gradient(circle at 40% 35%, #5a4d3a, #2a2218);
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.6);
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		padding: 0;
	}
	.orb:hover {
		border-color: #c8a14a;
	}
	.orb img {
		max-width: 24px;
		max-height: 24px;
		image-rendering: pixelated;
	}
	.shot .hud span {
		background: rgba(0, 0, 0, 0.55);
		color: var(--on-frame);
		font:
			500 11px 'JetBrains Mono',
			monospace;
		padding: 3px 7px;
		border-radius: 2px;
	}
	.log {
		display: flex;
		gap: 10px;
		align-items: flex-start;
		padding: 6px 0;
		border-bottom: 1px solid var(--hairline);
		font-size: 12px;
	}
	.log .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 46px;
		flex: 0 0 auto;
		padding-top: 1px;
	}
	.log .who {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		width: 54px;
		flex: 0 0 auto;
		padding-top: 2px;
	}
	.log.fail span:last-child {
		color: var(--not-started);
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
	.tiny {
		font-size: 11px;
		margin: 0;
	}
	@media (max-width: 900px) {
		.modes {
			margin-left: 0;
		}
		.side-col {
			max-width: none;
		}
	}
</style>
