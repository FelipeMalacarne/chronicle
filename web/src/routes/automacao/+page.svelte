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

	/*
	 * Automação › Cockpit (V6Microbot1440; no estreito, o modo só dashboard do V6Microbot640), doc 08
	 * fase 1: o estado do cliente pelo agent, a tela por screenshot a cada 5 s, iniciar e parar o
	 * cliente headless, login e logout, e a auditoria. O VNC ao vivo (fase 2) e os plugins (fase 3)
	 * entram depois; Rotinas ficam para depois das receitas.
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

	const client = $derived(status?.client);
	const running = $derived(!!client?.running);
	const loggedIn = $derived(!!client?.state?.loggedIn);
	const facts = $derived(cards(status));
	const rows = $derived(stateRows(status));
	const pos = $derived(client?.state?.player?.position);
	const dev = $derived(status?.dev ?? true);

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
	// Screenshot a cada 5 s enquanto o cliente roda e a aba está à vista (V6Microbot640).
	$effect(() => {
		if (!running) return;
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
		logout: 'Logout'
	};
	/** Ações que mexem na conta pedem um segundo clique em até 4 s. */
	const CONFIRM: MicrobotAction[] = ['client/stop', 'login', 'logout'];
	let disarm: ReturnType<typeof setTimeout> | undefined;

	async function act(a: MicrobotAction) {
		if (busy) return;
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
			await microbotApi.action(a, a === 'login' && w > 0 ? { world: w } : undefined);
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
		<span class="soon" title="Plugins do Hub: fase 3 do doc 08">Plugins</span>
		<a href="#auditoria">Auditoria</a>
	</div>
	<div class="seg modes" role="group" aria-label="Modo">
		<button type="button" class="on" aria-pressed="true">só dashboard</button>
		<button type="button" disabled title="VNC ao vivo: fase 2 do doc 08">VNC ao vivo</button>
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
			e
			<code>MICROBOT_AGENT_TOKEN</code> no <code>serve</code>, e rode <code>ironman agent</code> na máquina
			do jogo.
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
					{#if loggedIn}
						{@render button('logout')}
					{:else}
						<label class="wl"
							><span class="vh">Mundo</span><input
								class="world"
								inputmode="numeric"
								placeholder="mundo"
								bind:value={world}
							/></label
						>
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
					Tela do jogo<span class="r"
						>{running ? 'screenshot a cada 5 s' : 'cliente parado'}{client?.headless
							? ' · headless'
							: ''}</span
					>
				</div>
				<div class="pad">
					<div class="shot">
						{#if running && shotAt}
							<img
								src={microbotApi.screenshotURL(shotAt)}
								alt="Tela do cliente do Microbot"
								onload={() => (shotOk = true)}
								onerror={() => (shotOk = false)}
							/>
							<div class="hud">
								{#if client?.state?.player?.name}<span>{client.state.player.name}</span>{/if}
								{#if client?.login?.currentWorld}<span>mundo {client.login.currentWorld}</span>{/if}
								{#if pos}<span>{pos.x}, {pos.y}, {pos.plane}</span>{/if}
								<span>só dashboard</span>
							</div>
						{/if}
						{#if !running || !shotOk}
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
					</div>
				</div>
			</section>

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
			<p class="su tiny">
				Cliente de terceiros, fora do planner. Parar tudo: Ctrl+C no <code>ironman agent</code> no zaros.
				Login derruba o mesmo personagem de outro cliente.
			</p>
		</aside>
	</div>
{/if}

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
	.seg a,
	.seg .soon {
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
	.seg a + .soon,
	.seg .soon + a {
		border-left: 1px solid var(--border);
	}
	.seg a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.seg .soon,
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
