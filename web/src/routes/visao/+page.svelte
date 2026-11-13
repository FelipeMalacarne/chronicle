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
	import { scriptRows } from '#lib/progress/jogo';
	import { me } from '#lib/progress/me.svelte';
	import { microbotModule } from '#lib/progress/microbot.svelte';
	import { session } from '#lib/progress/session.svelte';
	import type { TimerView } from '#lib/progress/types';
	import { boardIn } from '#lib/progress/undo';
	import { chipsOf, routeLabel } from '#lib/progress/view';
	import { workerFacts, workerLines } from '#lib/progress/workers';
	import AuditCard from '#lib/v6/AuditCard.svelte';
	import ClientAccount from '#lib/v6/ClientAccount.svelte';
	import ClientLog from '#lib/v6/ClientLog.svelte';
	import ClocksCard from '#lib/v6/ClocksCard.svelte';
	import FazerAgora from '#lib/v6/FazerAgora.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import HistoricoRuns from '#lib/v6/HistoricoRuns.svelte';
	import ObjectiveChip from '#lib/v6/ObjectiveChip.svelte';

	/*
	 * Conta › Visão geral (página 8 do canvas, V7Conta1440; fatia V4 de decisoes-canvas.md, 7.5), com
	 * o que o servidor já tem: Fazendo agora (a tela do jogo e o script, quando o cliente roda), um
	 * cartão provisório de Relógios e Fazer agora no lugar da Fila (até as rotinas, V6), a rota com o
	 * passo atual, o worker e as últimas execuções. O ritmo de "Fazendo agora" e o cartão "Hoje" entram
	 * com a fatia V5; as ações no cliente ficam aqui até o Histórico (V7).
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
		playing[0]
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

<!-- Provisório (fatia V3): Relógios e Fazer agora no lugar da Fila do artboard, que entra com as
	rotinas (V6). Ao lado de Fazendo agora quando o cliente roda; sem ele, entre a rota e o worker. -->
{#snippet fila()}
	<div class="fila">
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

{#if running}
	<div class="row1">
		<section class="card now">
			<div class="h">
				<h2>Fazendo agora</h2>
				<span title={doing}>{doing}</span>
			</div>
			<div class="b">
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
			</div>
		</section>
		{@render fila()}
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
							title={line?.outdated ? 'Reinicia na versão nova; sai do jogo' : 'Sai do jogo'}
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
	.fila {
		flex: 1 1 320px;
		display: flex;
		flex-direction: column;
	}
	.fila > :global(.card) {
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
