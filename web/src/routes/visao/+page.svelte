<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { MicrobotError, workersApi, type WorkersView } from '#lib/api/microbot';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { nextUp } from '#lib/progress/frota';
	import { me } from '#lib/progress/me.svelte';
	import { session } from '#lib/progress/session.svelte';
	import type { TimerView } from '#lib/progress/types';
	import { ago, routeLabel, STALE_SOURCE_MS } from '#lib/progress/view';
	import { workerLines } from '#lib/progress/workers';

	/*
	 * Conta › Visão geral (página 8 do canvas, V7Conta1440), com o que o servidor já tem: a rota,
	 * os relógios, o worker e as fontes. "Fazendo agora" com ritmo e ganhos, a Fila de rotinas,
	 * "Hoje" e as últimas execuções do artboard esperam as fatias V5 a V7 (decisoes-canvas.md, 7.5).
	 */
	const account = $derived(currentAccount());
	let timers = $state<TimerView[]>([]);
	let workers = $state<WorkersView | null>(null);
	let workersOff = $state(false);
	let now = $state(Date.now());

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

	const route = $derived(
		session.summary?.account === account ? session.summary?.routes.find((r) => r.active) : undefined
	);
	const pct = $derived(
		route?.counts.total ? Math.round((100 * route.counts.done) / route.counts.total) : 0
	);
	const clocks = $derived(nextUp(timers, now, 6));
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

	<section class="card">
		<div class="h">
			<h2>Relógios</h2>
			<span>Time Tracking</span>
		</div>
		<div class="b tight">
			{#each clocks as c (c.label)}
				<div class="cl">
					<span>{c.label}</span><span class="w" class:rdy={c.ready}>{c.when}</span>
				</div>
			{:else}
				<p class="mu">
					Nenhum relógio contando. Eles chegam pelo plugin, com o Time Tracking ligado no cliente.
				</p>
			{/each}
		</div>
	</section>

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
			{:else if workersOff}
				<p class="mu">Este servidor só acompanha: não guarda sessões nem sobe clientes.</p>
			{:else}
				<p class="mu">
					Só acompanhada. Para o Chronicle abrir o cliente dela, envie a sessão do jogo pelo
					launcher.
				</p>
				<div class="row"><a class="btn btn-s" href={resolve('/contas/nova')}>Como enviar</a></div>
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
	.w.rdy {
		color: var(--done);
		font-weight: 600;
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
