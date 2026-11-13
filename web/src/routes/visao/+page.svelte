<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import {
		microbotApi,
		MicrobotError,
		workersApi,
		type GameView,
		type MicrobotStatus,
		type ScriptEntry,
		type WorkersView
	} from '#lib/api/microbot';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { rowChips } from '#lib/progress/agora';
	import { iconFor, itemIcon } from '#lib/progress/icons';
	import { scriptRows } from '#lib/progress/jogo';
	import { me } from '#lib/progress/me.svelte';
	import { microbotModule } from '#lib/progress/microbot.svelte';
	import { dayCard, nowStats, sessionItems, todaySince, trainingBar } from '#lib/progress/ritmo';
	import { session } from '#lib/progress/session.svelte';
	import type { AccountRhythm, TimerView } from '#lib/progress/types';
	import { boardIn } from '#lib/progress/undo';
	import { chipsOf, routeLabel } from '#lib/progress/view';
	import { RESTART_NOW_TITLE, workerFacts, workerLines } from '#lib/progress/workers';
	import AuditCard from '#lib/v6/AuditCard.svelte';
	import ClientAccount from '#lib/v6/ClientAccount.svelte';
	import ClientLog from '#lib/v6/ClientLog.svelte';
	import ClocksCard from '#lib/v6/ClocksCard.svelte';
	import FazerAgora from '#lib/v6/FazerAgora.svelte';
	import FilaConta from '#lib/v6/FilaConta.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import HistoricoRuns from '#lib/v6/HistoricoRuns.svelte';
	import ObjectiveChip from '#lib/v6/ObjectiveChip.svelte';

	/*
	 * Conta › Visão geral (página 8 do canvas, V7Conta1440; fatias V4 e V5 de decisoes-canvas.md, 7.5),
	 * com o que o servidor já tem: Fazendo agora (a tela do jogo e o script, quando o cliente roda, e o
	 * ritmo: a skill que treina, XP por hora, a sessão, até a pausa e o que ganhou), a Fila com Relógios
	 * e Fazer agora, a rota com o passo atual, Hoje (XP por skill e GP desde a meia-noite), o worker e
	 * as últimas execuções. Sem dado, a linha some (seção 7.14).
	 */
	const account = $derived(currentAccount());
	let timers = $state<TimerView[]>([]);
	let workers = $state<WorkersView | null>(null);
	let workersOff = $state(false);
	let now = $state(Date.now());
	let status = $state<MicrobotStatus | null>(null);
	let scripts = $state<ScriptEntry[]>([]);
	let game = $state<GameView | null>(null);
	let shotAt = $state(0);
	let shotOk = $state(false);
	let runsVersion = $state(0);
	let showLog = $state(false);
	let showAccount = $state(false);
	let armed = $state(false);
	let busy = $state(false);
	let failure = $state<string | null>(null);

	async function load(id: string) {
		try {
			timers = (await progressApi.timers(id)).timers;
		} catch {
			timers = [];
		}
		try {
			workers = await workersApi.list();
			workersOff = false;
		} catch (e) {
			workers = null;
			workersOff = e instanceof MicrobotError && e.status === 404;
		}
		now = Date.now();
	}
	$effect(() => {
		const id = account;
		if (!id) return;
		void load(id);
		const t = setInterval(() => void load(id), 15_000);
		return () => clearInterval(t);
	});

	/* O cliente da conta (o layout aponta o Microbot para a conta aberta). */
	async function loadClient() {
		try {
			status = await microbotApi.status();
		} catch {
			status = null;
		}
	}
	$effect(() => {
		if (!account || !microbotModule.enabled) return;
		void loadClient();
		const t = setInterval(() => {
			if (!document.hidden) void loadClient();
		}, 10_000);
		return () => clearInterval(t);
	});
	const agent = $derived(!!status?.agent.connected);
	const client = $derived(status?.client ?? null);
	const running = $derived(agent && !!client?.running);
	const loggedIn = $derived(running && !!client?.state?.loggedIn);
	const paused = $derived(running && !!client?.break?.active);

	/* O ritmo da conta, desde a meia-noite deste navegador. */
	let rhythm = $state<AccountRhythm | null>(null);
	async function loadRhythm(id: string) {
		try {
			const r = await progressApi.rhythm(id, todaySince(new Date()));
			if (r.account === id) rhythm = r;
		} catch {
			/* sem o ritmo, Fazendo agora fica com a tela e Hoje some */
		}
	}
	$effect(() => {
		const id = account;
		rhythm = null;
		if (!id) return;
		void loadRhythm(id);
		const t = setInterval(() => {
			if (!document.hidden) void loadRhythm(id);
		}, 30_000);
		return () => clearInterval(t);
	});
	// A sessão é a de agora só enquanto a conta joga (ou ganhou XP há pouco); parada, é a última.
	const live = $derived(loggedIn || paused || !!rhythm?.training);
	const bar = $derived(trainingBar(rhythm));
	const stats = $derived(nowStats(live ? rhythm : null, running ? client?.break : null));
	const items = $derived(sessionItems(live ? rhythm : null));
	const hasRhythm = $derived(!!bar || stats.length > 0 || items.gained.length > 0 || !!items.out);
	const today = $derived(dayCard(rhythm));

	/* Fazendo agora: o script que joga (os plugins ligados e, logada, o estado e as voltas). */
	$effect(() => {
		if (!running) {
			scripts = [];
			return;
		}
		const read = async () => {
			try {
				scripts = (await microbotApi.plugins()).scripts.scripts;
			} catch {
				/* sem a lista, o cartão só mostra a tela */
			}
		};
		void read();
		const t = setInterval(() => {
			if (!document.hidden) void read();
		}, 15_000);
		return () => clearInterval(t);
	});
	let gameBusy = false;
	$effect(() => {
		if (!loggedIn) {
			game = null;
			return;
		}
		const read = async () => {
			if (gameBusy) return;
			gameBusy = true;
			try {
				game = await microbotApi.game();
			} catch {
				/* o estado do cliente já diz o que houve */
			} finally {
				gameBusy = false;
			}
		};
		void read();
		const t = setInterval(() => {
			if (!document.hidden) void read();
		}, 15_000);
		return () => clearInterval(t);
	});
	// A tela a cada 5 s enquanto o cliente roda e a aba está à vista.
	$effect(() => {
		if (!running) return;
		shotAt = Date.now();
		const t = setInterval(() => {
			if (!document.hidden) shotAt = Date.now();
		}, 5000);
		return () => clearInterval(t);
	});
	const playing = $derived(scriptRows(scripts, game));
	const doing = $derived(
		!running
			? 'à mão'
			: paused
				? 'em pausa · Break Handler'
				: playing[0]
					? [playing[0].name, playing[0].detail, playing.length > 1 ? `+${playing.length - 1}` : '']
							.filter(Boolean)
							.join(' · ')
					: loggedIn
						? 'nenhum script jogando'
						: 'na tela de login'
	);
	const pos = $derived(game?.observe?.player?.position ?? client?.state?.player?.position);
	const world = $derived(client?.login?.currentWorld);

	/* Rota: o passo atual da rota ativa, com os objetivos dele. */
	const route = $derived(
		session.summary?.account === account ? session.summary?.routes.find((r) => r.active) : undefined
	);
	const pct = $derived(
		route?.counts.total ? Math.round((100 * route.counts.done) / route.counts.total) : 0
	);
	const guideId = $derived(route?.guide);
	const board = createResource(
		() => (account && guideId ? progressApi.board(guideId, account) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const boardData = $derived(
		boardIn(board.data, account && guideId ? { account, guide: guideId } : null)
	);
	const cursorId = $derived(boardData?.cursor?.step_id);
	const cursor = $derived(boardData?.steps.find((s) => s.id === cursorId));
	const section = createResource(() =>
		guideId && cursor?.section_id
			? progressApi.section(guideId, cursor.section_id)
			: Promise.resolve(null)
	);
	const chips = $derived.by(() => {
		const step = section.data?.steps.find((s) => s.id === cursorId);
		return step && cursor ? rowChips(chipsOf(step, cursor), 2) : { shown: [], more: 0 };
	});

	/* Worker. */
	const line = $derived(workerLines(workers, now).find((l) => l.account === account));
	const row = $derived(workers?.workers.find((w) => w.account === account));
	const pluginAt = $derived(
		session.summary?.account === account
			? session.summary?.sources.find((s) => s.source === 'plugin')?.last_observed_at
			: undefined
	);
	const facts = $derived(workerFacts(row, line, { at: pluginAt, bridge: client?.bridge }, now));
	const canRestart = $derived(!!line?.actions.includes('restart'));
	let disarm: ReturnType<typeof setTimeout> | undefined;
	async function restart() {
		if (!account || busy) return;
		if (!armed) {
			armed = true;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = false), 4000);
			return;
		}
		armed = false;
		busy = true;
		failure = null;
		try {
			await workersApi.action(account, 'restart');
			await load(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{me.state?.account.rsn ?? 'Conta'} · Chronicle</title></svelte:head>

<!-- A Fila do artboard (rotinas, V6) e, abaixo dela, Relógios e Fazer agora (provisório da V3). Ao
	lado de Fazendo agora quando o cliente roda; sem ele, entre a rota e o worker. -->
{#snippet fila()}
	<div class="fila">
		{#if account}<FilaConta {account} />{/if}
		<ClocksCard
			{timers}
			{now}
			empty="Nenhum relógio contando. Eles chegam pelo plugin, com o Time Tracking ligado no cliente."
		>
			{#if running}
				<FazerAgora bare {loggedIn} onchange={() => runsVersion++} />
			{/if}
		</ClocksCard>
	</div>
{/snippet}

{#if running || bar}
	<div class="row1">
		<section class="card now" class:wide={hasRhythm}>
			<div class="h">
				<h2>Fazendo agora</h2>
				<span title={doing}>{doing}</span>
			</div>
			<div class="b" class:split={hasRhythm}>
				{#if running}
					<a href={resolve('/automacao')} class="shot" aria-label="Abrir o cliente desta conta">
						{#if shotAt}
							<img
								src={microbotApi.screenshotURL(shotAt)}
								alt="Tela do cliente do jogo"
								onload={() => (shotOk = true)}
								onerror={() => (shotOk = false)}
							/>
						{/if}
						{#if !shotOk}
							<span class="ph"
								><Glyph name="automacao" size={22} /><span>tela do jogo · atualiza a cada 5 s</span
								></span
							>
						{/if}
						{#if world || pos}
							<span class="hud"
								>{#if world}<span>mundo {world}</span>{/if}{#if pos}<span>{pos.x}, {pos.y}</span
									>{/if}</span
							>
						{/if}
					</a>
				{/if}
				{#if hasRhythm}
					<div class="ritmo">
						{#if bar}
							{@const skill = iconFor(`skill:${bar.skill}`)}
							<div class="prog fz">
								<span class="l"
									>{#if skill}<img src={skill} alt="" />{/if}{bar.name}
									{bar.level}</span
								>
								<div
									class="track"
									class:live={!paused}
									class:rest={paused}
									role="progressbar"
									aria-valuenow={bar.pct}
									aria-valuemin="0"
									aria-valuemax="100"
									aria-label="{bar.name} {bar.level}, {bar.pct}% do nível"
									title="{bar.pct}% do caminho até o {bar.level + 1}"
								>
									<span style="width: {bar.pct}%"></span>
								</div>
								<span>{bar.next ?? ''}</span>
							</div>
						{/if}
						{#if stats.length}
							<div class="stats">
								{#each stats as st (st.k)}
									<div class="kv"><span class="k">{st.k}</span><span class="val">{st.v}</span></div>
								{/each}
							</div>
						{/if}
						{#if items.gained.length || items.out}
							<div>
								<p class="lbl ganhou">
									{items.gained.length ? 'Ganhou nesta sessão' : 'Nesta sessão'}
								</p>
								<div class="itens">
									{#each items.gained as g (g.item)}
										{@const sprite = itemIcon(g.item)}
										<span class="slot" title={g.title}
											><span class="bx"
												>{#if sprite}<img class="sp" src={sprite} alt="" />{/if}</span
											><span class="nm"><b>{g.qty}</b> {g.name}</span></span
										>
									{/each}
									{#if items.out}
										{@const sprite = itemIcon(items.out.item)}
										<span class="slot slot-miss acaba" title={items.out.title}
											><span class="bx"
												>{#if sprite}<img class="sp" src={sprite} alt="" />{/if}</span
											><span class="nm">{items.out.text}</span></span
										>
									{/if}
								</div>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</section>
		{#if running}{@render fila()}{/if}
	</div>
{/if}

<div class="row2">
	<section class="card">
		<div class="h">
			<h2>Rota</h2>
			{#if route}<span
					>{routeLabel(route.guide, route.title)} · {route.counts.done} de {route.counts
						.total}</span
				>{/if}
		</div>
		<div class="b">
			{#if route}
				<div class="xp"><span style="width: {pct}%"></span></div>
				{#if cursor}
					<p class="step">
						<span class="mono su">{cursor.ref}</span>
						{cursor.title}
					</p>
					{#if chips.shown.length}
						<div class="chips">
							{#each chips.shown as c (c.role + c.key)}<ObjectiveChip chip={c} small />{/each}
							{#if chips.more}<span class="su"
									>+{chips.more} {chips.more === 1 ? 'objetivo' : 'objetivos'}</span
								>{/if}
						</div>
					{/if}
				{:else if boardData}
					<p class="mu">Rota concluída: nenhum passo aberto.</p>
				{:else if route.cursor_ref}
					<p><span class="mono su">{route.cursor_ref}</span></p>
				{/if}
				<div class="acts"><a class="btn btn-s" href={resolve('/agora')}>Abrir o passo</a></div>
			{:else}
				<p class="mu">Esta conta não segue nenhuma rota.</p>
			{/if}
		</div>
	</section>

	{#if rhythm}
		<section class="card">
			<div class="h">
				<h2>Hoje</h2>
				<span>desde 00:00</span>
			</div>
			<div class="b hoje">
				{#each today.rows as d (d.skill)}
					{@const icon = iconFor(`skill:${d.skill}`)}
					<div class="dia">
						{#if icon}<img src={icon} alt="" />{:else}<span></span>{/if}<span>{d.name}</span><span
							class="u"
							class:up={d.up}>{d.levels}</span
						><span class="x">{d.xp}</span>
					</div>
				{:else}
					<p class="mu nada">Nenhum XP visto desde 00:00.</p>
				{/each}
				{#if today.more}
					<div class="dia">
						<span></span><span class="su"
							>mais {today.more.n} {today.more.n === 1 ? 'skill' : 'skills'}</span
						><span></span><span class="x">{today.more.xp}</span>
					</div>
				{/if}
				{#if today.gp}
					{@const coins = itemIcon(995)}
					<div class="dia" title="Moedas e platinum tokens no banco e no inventário">
						{#if coins}<img src={coins} alt="" />{:else}<span></span>{/if}<span>GP</span><span
							class="u">{today.gp.value}</span
						><span class="x">{today.gp.today ?? ''}</span>
					</div>
				{/if}
			</div>
		</section>
	{/if}

	{#if !running}{@render fila()}{/if}

	<section class="card">
		<div class="h">
			<h2>Worker</h2>
			{#if line?.runtime}<span>{line.host}</span>{/if}
		</div>
		<div class="b">
			{#if facts.length}
				<div class="kvg">
					{#each facts as f (f.k)}
						<span class="k">{f.k}</span><span class:bad={f.bad}
							>{#if f.mono}<span class="mono">{f.mono}</span> ·
							{/if}{f.v}</span
						>
					{/each}
				</div>
			{/if}
			{#if !line}
				{#if agent}
					<p class="mu">Um agent ligado a esta conta abre o cliente dela.</p>
				{:else if workersOff}
					<p class="mu">Este servidor só acompanha: não guarda sessões nem sobe clientes.</p>
				{:else}
					<p class="mu">
						Só acompanhada. Para o Chronicle abrir o cliente dela, envie a sessão do jogo pelo
						launcher.
					</p>
				{/if}
			{/if}
			{#if failure}<p class="bad">{failure}</p>{/if}
			{#if canRestart || agent || (!line && !agent && !workersOff)}
				<div class="acts">
					{#if canRestart}
						<button
							class="btn btn-s"
							class:btn-d={armed}
							type="button"
							disabled={busy}
							title={line?.outdated ? RESTART_NOW_TITLE : 'Sai do jogo'}
							onclick={restart}
							>{busy ? '…' : armed ? 'Reiniciar e sair do jogo?' : 'Reiniciar'}</button
						>
					{/if}
					{#if agent}
						<button
							class="btn btn-s btn-g"
							type="button"
							aria-pressed={showLog}
							onclick={() => (showLog = !showLog)}>Log do cliente</button
						>
						{#if line?.runtime === 'anexado' || !line}
							<button
								class="btn btn-s btn-g"
								type="button"
								aria-pressed={showAccount}
								onclick={() => (showAccount = !showAccount)}>Conta do cliente</button
							>
						{/if}
					{/if}
					{#if !line && !agent && !workersOff}
						<a class="btn btn-s" href={resolve('/contas/nova')}>Enviar a sessão</a>
					{/if}
				</div>
			{/if}
		</div>
	</section>
</div>

{#if agent && (showLog || showAccount)}
	<div class="row2">
		{#if showLog}<ClientLog />{/if}
		{#if showAccount}<ClientAccount {running} />{/if}
	</div>
{/if}

{#if microbotModule.enabled}
	<HistoricoRuns version={runsVersion} />
	<AuditCard version={runsVersion} />
{/if}

<style>
	.row1 {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: stretch;
	}
	.now {
		flex: 1 1 340px;
		max-width: 520px;
	}
	.now .shot {
		text-decoration: none;
	}
	/* With the rhythm, the card takes the row and puts it beside the screen (V7Conta1440). */
	.now.wide {
		flex: 999 1 560px;
		max-width: none;
	}
	.now .b.split {
		flex-direction: row;
		flex-wrap: wrap;
		gap: 18px;
	}
	.split .shot {
		flex: 1 1 340px;
		max-width: 420px;
		align-self: flex-start;
	}
	.ritmo {
		flex: 999 1 300px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.prog.fz {
		font-size: 12px;
	}
	.prog.fz .track {
		height: 14px;
	}
	.prog.fz .track.live span {
		background-image: linear-gradient(
			110deg,
			rgba(255, 255, 255, 0.16) 0 25%,
			transparent 25% 50%,
			rgba(255, 255, 255, 0.16) 50% 75%,
			transparent 75%
		);
		background-size: 28px 14px;
		animation: flow 1.1s linear infinite;
	}
	@keyframes flow {
		from {
			background-position: 0 0;
		}
		to {
			background-position: 28px 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.prog.fz .track span {
			animation: none !important;
		}
	}
	/* Not `.v`: that global class is the verdict circle. */
	.stats .kv .val {
		font:
			700 18px/1.1 Alegreya,
			Georgia,
			serif;
	}
	.ganhou {
		padding-bottom: 6px;
	}
	.itens {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.itens .slot .nm b {
		font-weight: 600;
		color: var(--ink);
	}
	.itens .acaba {
		width: 150px;
		align-items: flex-start;
	}
	.itens .acaba .nm {
		max-width: none;
	}
	/* Hoje: one line per skill that gained XP, then the GP. */
	.b.hoje {
		gap: 0;
	}
	.dia {
		display: grid;
		grid-template-columns: 20px minmax(0, 1fr) auto auto;
		gap: 10px;
		align-items: center;
		min-height: 32px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.dia:last-child {
		border-bottom: 0;
	}
	.dia img {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.dia .u {
		font-size: 12px;
		color: var(--ink-subtle);
	}
	.dia .u.up {
		font:
			600 12px 'JetBrains Mono',
			monospace;
		color: var(--done);
	}
	.dia .x {
		width: 64px;
		text-align: right;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.nada {
		margin: 0;
		padding: 6px 0;
		font-size: 13px;
	}
	.fila {
		flex: 1 1 320px;
		display: flex;
		flex-direction: column;
	}
	.fila {
		gap: 20px;
	}
	.fila > :global(.card:last-child) {
		flex: 1;
	}
	.row2 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
		gap: 20px;
		align-items: stretch;
	}
	.step {
		line-height: 1.45;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
		font-size: 12px;
	}
	.acts {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		margin-top: auto;
	}
	.bad {
		color: var(--not-started);
	}
</style>
