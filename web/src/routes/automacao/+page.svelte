<script lang="ts">
	import {
		driftApi,
		fleetPluginsApi,
		microbotApi,
		MicrobotError,
		type ClientDrift,
		type GameView,
		type MicrobotAction,
		type MicrobotStatus,
		type PluginsView,
		type RecurringState,
		type Trigger
	} from '#lib/api/microbot';
	import type { ProfilePlugin } from '#lib/progress/fleetplugins';
	import { clientCard } from '#lib/progress/clientplugins';
	import Glyph from '#lib/v6/Glyph.svelte';
	import { cockpit } from '#lib/progress/microbot.svelte';
	import VncView, { type VncPhase } from '#lib/v6/VncView.svelte';
	import SidePanel from '#lib/v6/game/SidePanel.svelte';
	import Chatbox from '#lib/v6/game/Chatbox.svelte';
	import GameBank from '#lib/v6/game/GameBank.svelte';
	import GameMenu from '#lib/v6/game/GameMenu.svelte';
	import WorldMap from '#lib/v6/game/WorldMap.svelte';
	import { cancelSelection, gameMap, gameMenu } from '#lib/v6/game/menu.svelte';
	import { scriptRows, selectedLabel } from '#lib/progress/jogo';
	import { miscIcon, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import PluginsCard from '#lib/v6/PluginsCard.svelte';
	import PluginDrawer from '#lib/v6/PluginDrawer.svelte';
	import NearCard from '#lib/v6/NearCard.svelte';
	import BankCard from '#lib/v6/BankCard.svelte';

	/*
	 * Conta › Cliente (página 8 do canvas, V7Cliente1440 e V7Cliente640; fatia V3 de
	 * decisoes-canvas.md, 7.5): o jogo no centro (tela por imagem a cada 5 s ou ao vivo por VNC, o
	 * chat e o painel de itens com o menu do jogo), e ao lado os plugins, o que está por perto e o
	 * banco. Relógios, Fazer agora, execuções, auditoria, log e conta do cliente foram para a Visão
	 * geral. A 640 o jogo e Por perto ficam empilhados, sem ao vivo, e Plugins vem embaixo.
	 *
	 * Os plugins moram aqui desde a rodada 4 do canvas (V7ClientePlugins1440 e V7ClienteGaveta1440,
	 * seção 13 do brief): o cartão Plugins liga, para e abre a configuração numa gaveta sobre a
	 * coluna da direita, sem sair do jogo; a manutenção (Hub, versões, remover) fica em /plugins.
	 */
	let status = $state<MicrobotStatus | null>(null);
	let off = $state(false);
	let loadError = $state<string | null>(null);
	let shotAt = $state(0);
	let shotOk = $state(false);
	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let world = $state('');
	let game = $state<GameView | null>(null);
	let plugins = $state<PluginsView | null>(null);
	let drift = $state<ClientDrift | null>(null);
	let triggers = $state<Trigger[]>([]);
	let recurring = $state<RecurringState[]>([]);
	let profilePlugins = $state<ProfilePlugin[]>([]);
	let hasProfile = $state(false);
	/** O grupo de config de cada plugin (do esquema), lido uma vez por classe. */
	let groups = $state<Record<string, string>>({});
	let startedAt = $state<Record<string, number>>({});
	let openPlugin = $state<string | null>(null);
	let live = $state(false);
	let intervene = $state(false);
	let vnc = $state<{ phase: VncPhase; reason?: string }>({ phase: 'closed' });
	let wide = $state(true);

	const client = $derived(status?.client);
	const agent = $derived(!!status?.agent.connected);
	const running = $derived(!!client?.running);
	const loggedIn = $derived(!!client?.state?.loggedIn);
	const paused = $derived(!!client?.state?.scriptsPaused);
	const pos = $derived(game?.observe?.player?.position ?? client?.state?.player?.position);
	const currentWorld = $derived(client?.login?.currentWorld);
	/** O VNC só existe com o x11vnc do headless e fora do celular (V7Cliente640: sem ao vivo). */
	const vncReady = $derived(running && !!client?.vnc && wide);
	const showLive = $derived(live && vncReady);
	const inGame = $derived(loggedIn && !!game);
	const bankOpen = $derived(inGame && !!game?.bank?.open && !showLive);
	const scripts = $derived(plugins?.scripts.scripts ?? []);
	const playing = $derived(scriptRows(scripts, game));
	// Lê a conta escolhida para o VNC reabrir quando ela muda.
	const vncURL = () => (cockpit.account, microbotApi.vncURL());
	const account = $derived(cockpit.account || status?.agent.account || '');
	const clientState = $derived(
		!agent ? 'none' : !running ? 'stopped' : loggedIn ? 'game' : 'login'
	);
	const card = $derived(
		plugins
			? clientCard({
					view: plugins,
					game,
					paused,
					profile: profilePlugins,
					triggers,
					recurring,
					drift,
					groups,
					startedAt
				})
			: null
	);
	const cardRows = $derived(
		card ? [...card.plays, ...card.routines, ...card.always, ...card.client] : []
	);
	const openRow = $derived(cardRows.find((r) => r.className === openPlugin) ?? null);
	const drawerGroups = $derived(
		card
			? [
					{ label: 'Jogam', items: card.plays },
					{ label: 'Rotinas usam', items: card.routines },
					{ label: 'Sempre ligados', items: card.always }
				]
			: []
	);

	const tone = $derived(
		!agent ? 'off' : loggedIn ? 'ok' : running ? 'up' : client?.error ? 'bad' : 'off'
	);
	const stateText = $derived(
		!status
			? (loadError ?? 'Lendo o cliente…')
			: !agent
				? 'Sem cliente'
				: loggedIn
					? 'No jogo'
					: running
						? 'Na tela de login'
						: 'Cliente parado'
	);
	const stateDetail = $derived(
		[
			loggedIn && currentWorld ? `mundo ${currentWorld}` : '',
			running && !loggedIn ? (client?.login?.loginError ?? '') : '',
			paused && loggedIn ? 'scripts pausados' : '',
			client?.error ?? ''
		]
			.filter(Boolean)
			.join(' · ')
	);

	function setLive(on: boolean) {
		live = on;
		intervene = false;
	}
	function onVnc(phase: VncPhase, reason?: string) {
		vnc = { phase, reason };
		if (phase === 'closed') intervene = false;
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
	async function refreshScripts() {
		try {
			plugins = await microbotApi.plugins();
		} catch {
			/* sem a lista, o cartão Plugins espera a próxima leitura */
		}
	}
	async function refreshDrift() {
		if (!account || !running) {
			drift = null;
			return;
		}
		try {
			drift = await driftApi.get(account);
		} catch {
			drift = null;
		}
	}
	/** Os grupos do cartão saem do uso: o que uma rotina roda e o que o perfil liga no boot. */
	async function refreshUse() {
		try {
			triggers = (await microbotApi.triggers()).triggers;
		} catch {
			triggers = [];
		}
		try {
			recurring = (await microbotApi.recurringOf(account)).recurring;
		} catch {
			recurring = [];
		}
		if (!account) return;
		try {
			const ac = await fleetPluginsApi.accountClient(account);
			profilePlugins = ac.effective.plugins;
			hasProfile = !!ac.profile;
		} catch {
			profilePlugins = [];
		}
	}
	function refreshPlugins() {
		void refresh();
		void refreshScripts();
		void refreshDrift();
	}
	$effect(() => {
		void refresh();
		const st = setInterval(() => {
			if (!document.hidden) void refresh();
		}, 3000);
		return () => clearInterval(st);
	});
	// Os plugins do cliente a cada 10 s enquanto ele roda; o desvio do perfil a cada 30 s; as
	// rotinas e o perfil (os grupos do cartão) a cada minuto.
	$effect(() => {
		if (!running) {
			plugins = null;
			return;
		}
		void refreshScripts();
		const t = setInterval(() => {
			if (!document.hidden) void refreshScripts();
		}, 10_000);
		return () => clearInterval(t);
	});
	$effect(() => {
		void account;
		if (!running) {
			drift = null;
			return;
		}
		void refreshDrift();
		const t = setInterval(() => {
			if (!document.hidden) void refreshDrift();
		}, 30_000);
		return () => clearInterval(t);
	});
	$effect(() => {
		void account;
		if (!agent) return;
		void refreshUse();
		const t = setInterval(() => {
			if (!document.hidden) void refreshUse();
		}, 60_000);
		return () => clearInterval(t);
	});
	// O grupo de config de cada plugin do cartão, para saber que desvio é dele.
	$effect(() => {
		if (!card) return;
		for (const r of [...card.plays, ...card.routines, ...card.always]) {
			if (r.protected || r.className in groups) continue;
			groups[r.className] = '';
			microbotApi
				.configSchema(r.className)
				.then((s) => (groups[r.className] = s.group))
				.catch(() => {});
		}
	});
	// Desde quando o primeiro que joga roda (o status do Agent Server), a cada minuto.
	const first = $derived(card?.plays.find((r) => r.active)?.className ?? '');
	$effect(() => {
		const cls = first;
		if (!cls) return;
		const read = async () => {
			try {
				const at = (await microbotApi.scriptDetail(cls)).status?.startedAt;
				startedAt = at ? { [cls]: Date.parse(at) } : {};
			} catch {
				startedAt = {};
			}
		};
		void read();
		const t = setInterval(() => {
			if (!document.hidden) void read();
		}, 60_000);
		return () => clearInterval(t);
	});
	// A gaveta fecha quando o cliente para ou o plugin some.
	$effect(() => {
		if (openPlugin && (!running || (plugins && !openRow))) openPlugin = null;
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
	// Depois de cada ação do menu, o jogo é relido.
	$effect(() => {
		gameMenu.onDone = () => void refreshGame();
		return () => {
			gameMenu.onDone = null;
		};
	});
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

	// Imagem a cada 5 s enquanto o cliente roda, a aba está à vista e o VNC não está aberto.
	$effect(() => {
		if (!running || showLive) return;
		shotAt = Date.now();
		const t = setInterval(() => {
			if (!document.hidden) shotAt = Date.now();
		}, 5000);
		return () => clearInterval(t);
	});

	const LABEL: Record<MicrobotAction, string> = {
		'client/start': 'Iniciar o cliente',
		'client/stop': 'Parar o cliente',
		login: 'Entrar no jogo',
		logout: 'Sair do jogo',
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
		}
	}
	const label = (a: MicrobotAction) =>
		busy === a ? `${LABEL[a]}…` : armed === a ? `Confirmar: ${LABEL[a].toLowerCase()}` : LABEL[a];

	const viewHint = $derived(
		showLive
			? vnc.phase === 'connected'
				? intervene
					? 'você no controle'
					: 'só olhando'
				: vnc.phase === 'connecting'
					? 'conectando…'
					: (vnc.reason ?? 'desconectado')
			: 'imagem a cada 5 s'
	);
</script>

<svelte:head>
	<title>Cliente · Chronicle</title>
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

{#if off}
	<div class="empty7">
		<h3>Sem cliente do jogo</h3>
		<p>Este servidor só acompanha: não abre nem controla o cliente das contas.</p>
	</div>
{:else}
	<div class="strip7">
		<span class="lamp {tone}"></span>
		<span class="stnow"
			><b>{stateText}</b>{#if stateDetail}<span class="mu"> · {stateDetail}</span>{/if}</span
		>
		<span class="sp"></span>
		{#if running}
			<div class="seg" role="group" aria-label="Como ver a tela">
				<button
					type="button"
					class:on={!showLive}
					aria-pressed={!showLive}
					title="Uma imagem a cada 5 s; leve"
					onclick={() => setLive(false)}>Imagem</button
				>
				<button
					type="button"
					class:on={showLive}
					aria-pressed={showLive}
					disabled={!vncReady}
					title={vncReady
						? 'Tela ao vivo; mouse e teclado só depois de "Intervir"'
						: !wide
							? 'Ao vivo só em tela larga'
							: 'Ao vivo só com o cliente sem janela (headless)'}
					onclick={() => setLive(true)}>Ao vivo</button
				>
			</div>
			<!-- V7Cliente640: a janela ao lado do jogo não troca de mundo. -->
			{#if wide}
				<label class="vh" for="mundo">Mundo</label>
				<input
					id="mundo"
					class="in7"
					inputmode="numeric"
					placeholder={currentWorld ? String(currentWorld) : 'mundo'}
					bind:value={world}
				/>
			{/if}
			{#if loggedIn}
				{#if wide}{@render button('world')}{/if}
				{@render button('logout')}
			{:else}
				{@render button('login')}
			{/if}
			{@render button('client/stop', true)}
		{:else if agent}
			{@render button('client/start')}
		{/if}
	</div>

	{#if failure}
		<div class="band" role="alert">
			<Glyph name="alert" /><span>{failure}</span>
			<button class="btn btn-g btn-s" type="button" onclick={() => (failure = null)}>Fechar</button>
		</div>
	{/if}
	{#if gameMenu.error}
		<div class="band" role="alert">
			<Glyph name="alert" /><span>{gameMenu.error}</span>
			<button class="btn btn-g btn-s" type="button" onclick={() => (gameMenu.error = null)}
				>Fechar</button
			>
		</div>
	{/if}
	{#if inGame && playing.length && !paused}
		<div class="note7" role="status">
			<Glyph name="alert" />
			<span
				><b>{playing[0].name} está jogando.</b>
				<span class="wide-only">O que você clicar aqui disputa o mouse com ele.</span></span
			>
		</div>
	{/if}

	<div class="cols">
		<section class="g-cl" aria-label="Cliente do jogo">
			<div class="g-l">
				<div class="g-vp">
					{#if showLive}
						<VncView url={vncURL()} viewOnly={!intervene} onphase={onVnc} />
						{#if vnc.phase !== 'connected'}
							<div class="ph">
								<Glyph name="automacao" size={22} />
								<span
									>{vnc.phase === 'connecting'
										? 'Abrindo a tela ao vivo…'
										: `Ao vivo fechado${vnc.reason ? `: ${vnc.reason}` : ''}`}</span
								>
							</div>
						{/if}
					{:else if running && shotAt}
						<img
							class="shot-img"
							src={microbotApi.screenshotURL(shotAt)}
							alt="Tela do cliente do jogo"
							onload={() => (shotOk = true)}
							onerror={() => (shotOk = false)}
						/>
					{/if}
					{#if bankOpen && game}<GameBank view={game} />{/if}
					{#if !showLive && (!running || !shotOk) && !bankOpen}
						<div class="ph">
							<Glyph name="automacao" size={22} />
							<span
								>{!status
									? 'Lendo o cliente…'
									: !agent
										? 'Nenhum cliente ligado a esta conta. Inicie o worker no cabeçalho, ou rode o agent no zaros.'
										: running
											? 'Carregando a tela…'
											: '“Iniciar o cliente” abre o jogo na tela de login.'}</span
							>
						</div>
					{/if}
					{#if running && !(inGame && gameMenu.selected)}
						<div class="hud">
							{#if currentWorld}<span>mundo {currentWorld}</span>{/if}
							{#if pos}<span>{pos.x}, {pos.y}{wide ? `, ${pos.plane}` : ''}</span>{/if}
							<span>{viewHint}{client?.headless ? '' : ' · com janela'}</span>
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
				{#if showLive}
					<div class="under">
						<span
							>{intervene
								? 'Mouse e teclado vão para o jogo. Um script rodando disputa o input com você.'
								: 'Abrir a tela ao vivo fica registrado.'}</span
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
			{#if inGame && game}<SidePanel view={game} />{/if}
		</section>

		{#if status}
			<aside class="rcol">
				{#if wide}{@render pluginsCard()}{/if}
				{#if inGame && game}
					<NearCard view={game} walk={wide} />
					<div class="wide-only"><BankCard view={game} /></div>
				{/if}
				{#if !wide}{@render pluginsCard()}{/if}
			</aside>
		{/if}
		{#if openRow}
			<PluginDrawer
				row={openRow}
				rows={drawerGroups}
				{account}
				{hasProfile}
				drift={openRow.drift}
				{inGame}
				{wide}
				accountName={[stateText, currentWorld ? `mundo ${currentWorld}` : '']
					.filter(Boolean)
					.join(' · ')}
				onclose={() => (openPlugin = null)}
				onswitch={(c) => (openPlugin = c)}
				onchange={refreshPlugins}
			/>
		{/if}
	</div>
	<p class="foot">{ICON_ATTRIBUTION}. Cliente de terceiros, fora do planner.</p>
{/if}

{#snippet pluginsCard()}
	<PluginsCard
		{card}
		client={clientState}
		{paused}
		open={openPlugin}
		onopen={(c) => (openPlugin = openPlugin === c ? null : c)}
		onchange={refreshPlugins}
		onstart={() => act('client/start')}
		onlogin={() => act('login')}
	/>
{/snippet}

<GameMenu />
{#if gameMap.open}<WorldMap me={pos} />{/if}

<style>
	/* Not `.st`: that global class is the step-status circle. */
	.strip7 .stnow {
		min-width: 0;
	}
	.strip7 .stnow b {
		font-weight: 600;
	}
	.seg button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	/* position: relative: a gaveta de configuração se posiciona sobre a coluna da direita. */
	.cols {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.g-cl {
		flex: 999 1 600px;
		min-width: 0;
	}
	.rcol {
		flex: 0 0 320px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.g-vp .shot-img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: contain;
	}
	.g-vp .ph {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 0 24px;
		max-width: 46ch;
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
	.under {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
		justify-content: space-between;
		font-size: 11px;
		color: var(--on-frame-muted);
	}
	.foot {
		font-size: 11px;
	}
	/* V7Cliente640: a tela, o chat e o painel de itens empilhados; Por perto embaixo. */
	@media (max-width: 700px) {
		.g-cl {
			flex-direction: column;
		}
		.g-vp {
			aspect-ratio: auto;
			min-height: 380px;
		}
		.rcol {
			flex: 1 1 100%;
		}
		.wide-only {
			display: none;
		}
	}
</style>
