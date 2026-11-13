<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { MicrobotError, workersApi, type WorkersView } from '#lib/api/microbot';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { me } from '#lib/progress/me.svelte';
	import { session } from '#lib/progress/session.svelte';
	import type { TimerView } from '#lib/progress/types';
	import { ago, routeLabel, STALE_SOURCE_MS } from '#lib/progress/view';
	import { workerLines } from '#lib/progress/workers';
	import { microbotModule } from '#lib/progress/microbot.svelte';
	import { microbotApi, type MicrobotStatus } from '#lib/api/microbot';
	import AuditCard from '#lib/v6/AuditCard.svelte';
	import ClientAccount from '#lib/v6/ClientAccount.svelte';
	import ClientLog from '#lib/v6/ClientLog.svelte';
	import ClocksCard from '#lib/v6/ClocksCard.svelte';
	import FazerAgora from '#lib/v6/FazerAgora.svelte';
	import HistoricoRuns from '#lib/v6/HistoricoRuns.svelte';

	/*
	 * Conta › Visão geral (página 8 do canvas, V7Conta1440), com o que o servidor já tem: a rota,
	 * os relógios, o worker e as fontes. Da fatia V3 (decisoes-canvas.md, 7.5), o que saiu do
	 * Cliente: Relógios e Fazer agora num cartão provisório no lugar da Fila, as últimas execuções,
	 * as ações no cliente (até o Histórico), e o log e a conta do cliente no cartão Worker.
	 * "Fazendo agora" com ritmo e ganhos, a Fila de rotinas e "Hoje" esperam as fatias V5 e V6.
	 */
	const account = $derived(currentAccount());
	let timers = $state<TimerView[]>([]);
	let workers = $state<WorkersView | null>(null);
	let workersOff = $state(false);
	let now = $state(Date.now());
	let client = $state<MicrobotStatus['client'] | null>(null);
	let agent = $state(false);
	let runsVersion = $state(0);
	let showLog = $state(false);
	let showAccount = $state(false);

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
	/* O cliente da conta (o Cockpit já aponta para a conta aberta), para Fazer agora e o log. */
	async function loadClient() {
		try {
			const s = await microbotApi.status();
			agent = !!s?.agent.connected;
			client = s?.client ?? null;
		} catch {
			agent = false;
			client = null;
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
	const running = $derived(!!client?.running);
	const loggedIn = $derived(!!client?.state?.loggedIn);

	const route = $derived(
		session.summary?.account === account ? session.summary?.routes.find((r) => r.active) : undefined
	);
	const pct = $derived(
		route?.counts.total ? Math.round((100 * route.counts.done) / route.counts.total) : 0
	);
	const line = $derived(workerLines(workers, now).find((l) => l.account === account));
	const LABEL: Record<string, string> = {
		plugin: 'Plugin',
		hiscores: 'Hiscores',
		wom: 'Wise Old Man',
		wikisync: 'WikiSync'
	};
	const sources = $derived(
		(session.summary?.account === account ? (session.summary?.sources ?? []) : []).map((s) => {
			const at = Date.parse(s.last_observed_at);
			return {
				key: s.source,
				label: LABEL[s.source] ?? s.source,
				age: ago(at, now),
				stale: now - at > STALE_SOURCE_MS
			};
		})
	);
</script>

<svelte:head><title>{me.state?.account.rsn ?? 'Conta'} · Chronicle</title></svelte:head>

<div class="grid">
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
				<p>
					{#if route.cursor_ref}Passo atual: <b>{route.cursor_ref}</b>.{:else}Nenhum passo aberto.{/if}
					{#if route.counts.blocked}<span class="mu"
							>{route.counts.blocked} esperando requisito.</span
						>{/if}
				</p>
				<div class="row">
					<a class="btn btn-s" href={resolve('/agora')}>Abrir o passo</a>
					<a class="btn btn-s btn-g" href={resolve('/rotas')}>Rota inteira</a>
				</div>
			{:else}
				<p class="mu">Esta conta não segue nenhuma rota.</p>
			{/if}
		</div>
	</section>

	<!-- Provisório (fatia V3): no lugar da Fila do artboard, que entra com as rotinas (V6). -->
	<ClocksCard
		{timers}
		{now}
		empty="Nenhum relógio contando. Eles chegam pelo plugin, com o Time Tracking ligado no cliente."
	>
		{#if agent && running}
			<FazerAgora bare {loggedIn} onchange={() => runsVersion++} />
		{/if}
	</ClocksCard>

	<section class="card">
		<div class="h">
			<h2>Worker</h2>
			{#if line?.runtime}<span>{line.host}</span>{/if}
		</div>
		<div class="b">
			{#if line}
				<dl class="kv">
					<dt>Estado</dt>
					<dd class:bad={line.tone === 'warn'}>
						{line.state}{line.detail ? ` · ${line.detail}` : ''}
					</dd>
					{#if line.runtime}
						<dt>Onde</dt>
						<dd>{line.host} · {line.runtime}{line.outdated ? ' · versão antiga' : ''}</dd>
					{/if}
					<dt>Sessão</dt>
					<dd class:bad={!line.sessionWhen}>
						{line.sessionWhen ? `${line.sessionWhen} · ${line.sessionNote}` : 'sem sessão do jogo'}
					</dd>
				</dl>
				{#if line.blocked}<p class="mu">{line.blocked}</p>{/if}
			{:else if agent}
				<p class="mu">Um agent ligado a esta conta abre o cliente dela.</p>
			{:else if workersOff}
				<p class="mu">Este servidor só acompanha: não guarda sessões nem sobe clientes.</p>
			{:else}
				<p class="mu">
					Só acompanhada. Para o Chronicle abrir o cliente dela, envie a sessão do jogo pelo
					launcher.
				</p>
				<div class="row"><a class="btn btn-s" href={resolve('/contas/nova')}>Como enviar</a></div>
			{/if}
			{#if agent}
				<div class="row">
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
				</div>
			{/if}
		</div>
	</section>

	<section class="card">
		<div class="h">
			<h2>Fontes</h2>
			<span>de onde vem o que se sabe</span>
		</div>
		<div class="b tight">
			{#each sources as s (s.key)}
				<div class="cl">
					<span>{s.label}</span><span class="w" class:old={s.stale}>{s.age}</span>
				</div>
			{:else}
				<p class="mu">Nenhuma leitura ainda.</p>
			{/each}
		</div>
	</section>
</div>

{#if agent && (showLog || showAccount)}
	<div class="grid">
		{#if showLog}<ClientLog />{/if}
		{#if showAccount}<ClientAccount {running} />{/if}
	</div>
{/if}

{#if microbotModule.enabled}
	<HistoricoRuns version={runsVersion} />
	<AuditCard version={runsVersion} />
{/if}

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
		gap: 20px;
		align-items: start;
	}
	.card {
		display: flex;
		flex-direction: column;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		min-width: 0;
	}
	.h {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 12px 16px 0;
	}
	.h h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.h span {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.b {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 12px 16px 16px;
		font-size: 13px;
	}
	.b.tight {
		gap: 0;
	}
	.b p {
		margin: 0;
		line-height: 1.45;
	}
	.b b {
		font-weight: 600;
	}
	.row {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.cl {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 8px;
		align-items: center;
		min-height: 32px;
		border-bottom: 1px solid var(--hairline);
	}
	.cl:last-child {
		border-bottom: 0;
	}
	.w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.w.old {
		color: var(--stale);
	}
	.kv {
		display: grid;
		grid-template-columns: 64px minmax(0, 1fr);
		gap: 7px 12px;
		margin: 0;
	}
	.kv dt {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		padding-top: 2px;
	}
	.kv dd {
		margin: 0;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.bad {
		color: var(--not-started);
	}
</style>
