<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { MicrobotError, workersApi, type WorkersView } from '#lib/api/microbot';
	import AutomacaoTabs from '#lib/v6/AutomacaoTabs.svelte';
	import { chooseCockpitAccount } from '#lib/progress/microbot.svelte';
	import {
		ACTION_CONFIRM,
		actionLabel,
		imageTag,
		workerLines,
		type WorkerActionKind,
		type WorkerLine
	} from '#lib/progress/workers';

	/*
	 * Automação › Workers (página 7 do canvas: V6MicrobotWorkers1440 e os estados de
	 * V6MicrobotWorkersEstados1440; docs/11). Uma linha por conta: onde roda, estado, sessão e ações,
	 * na tabela do artboard (painel, placa, células de duas linhas). Ainda sem o painel de
	 * capacidade, a coluna CPU · RAM e o worker aberto à direita, que dependem das métricas da F7.
	 */
	const CONNECT = 'chronicle accounts connect --character <nome do personagem>';

	let view = $state<WorkersView | null>(null);
	let off = $state(false);
	let loadError = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let worlds = $state<Record<string, string>>({});
	let now = $state(Date.now());

	const lines = $derived(workerLines(view, now));
	const withSession = $derived(view?.workers.filter((w) => w.session).length ?? 0);

	function count(n: number, noun: string): string {
		return `${n} ${noun}${n === 1 ? '' : 's'}`;
	}

	async function load() {
		try {
			view = await workersApi.list();
			loadError = null;
			now = Date.now();
		} catch (e) {
			if (e instanceof MicrobotError && e.status === 404) {
				off = true;
				return;
			}
			loadError = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		void load();
		const t = setInterval(() => void load(), 5000);
		return () => clearInterval(t);
	});

	async function act(line: WorkerLine, action: WorkerActionKind) {
		const key = `${line.account}:${action}`;
		if (armed !== key) {
			armed = key;
			return;
		}
		armed = null;
		busy = key;
		failure = null;
		const world = Number(worlds[line.account] ?? '') || undefined;
		try {
			await workersApi.action(line.account, action, action === 'start' ? world : undefined);
			await load();
		} catch (e) {
			failure = `${line.account}: ${e instanceof Error ? e.message : String(e)}`;
		} finally {
			busy = null;
		}
	}

	function openCockpit(account: string) {
		chooseCockpitAccount(account);
		void goto(resolve('/automacao'));
	}
</script>

<svelte:head><title>Workers · Automação · Chronicle</title></svelte:head>

<div class="page">
	<header class="mh">
		<div class="ttl">
			<p class="ey">Microbot · workers</p>
			<h1 class="sr">Workers</h1>
		</div>
		<AutomacaoTabs current="workers" />
	</header>

	{#if off}
		<section class="panel">
			<div class="plaque">Workers<span class="r">desligado</span></div>
			<p class="msg">
				Sem workers neste servidor: o controle do Microbot está desligado ou falta o
				<code>CHRONICLE_VAULT_KEY</code>.
			</p>
		</section>
	{:else}
		{#if view && !view.enabled}
			<p class="band">
				Workers desligados neste servidor (<code>WORKERS=off</code>): as sessões ficam guardadas,
				mas nada sobe no cluster.
			</p>
		{/if}
		{#if loadError}<p class="band bad">Não deu para ler os workers: {loadError}</p>{/if}
		{#if failure}<p class="band bad">{failure}</p>{/if}

		<section class="panel">
			<div class="plaque">
				Contas
				{#if view}<span class="r" title={view.image}>
						{count(view.workers.length, 'conta')} · {withSession} com sessão{view.image
							? ` · imagem ${imageTag(view.image)}`
							: ''}
					</span>{/if}
			</div>
			{#if view && lines.length === 0 && view.attached.length === 0}
				<div class="msg">
					<p>Para subir um worker, a conta precisa de uma sessão do launcher no Chronicle.</p>
					<p>No computador com o launcher do Microbot aberto, envie a sessão:</p>
					<div class="code"><code>{CONNECT}</code></div>
				</div>
			{:else if view}
				<div class="wk-t" role="table" aria-label="Workers por conta">
					<div class="wk-r hd" role="row">
						<span role="columnheader">Conta</span>
						<span role="columnheader">Onde</span>
						<span role="columnheader">Estado</span>
						<span role="columnheader">Sessão</span>
						<span role="columnheader" class="right">Ações</span>
					</div>
					{#each lines as l (l.account)}
						<div class="wk-r" role="row">
							<div class="wk-c" role="cell">
								<span class="a">{l.rsn}</span>
								<span class="b mono">{l.account}</span>
							</div>
							<div class="wk-c" role="cell" data-k="Onde">
								{#if l.runtime}
									<span class="a">
										{l.host}
										{#if l.outdated}<span class="badge badge-stale">versão antiga</span>{/if}
									</span>
									<span class="b mono">{l.runtime}</span>
								{:else}
									<span class="su">nenhum</span>
								{/if}
							</div>
							<div class="wk-c" role="cell" data-k="Estado">
								<span class="wk-s {l.tone}"><i></i>{l.state}</span>
								{#if l.detail}<span class="b" title={l.detail}>{l.detail}</span>{/if}
							</div>
							<div class="wk-c" role="cell" data-k="Sessão">
								{#if l.sessionWhen}
									<span>{l.sessionWhen}</span>
									<span class="b">{l.sessionNote}</span>
								{:else}
									<span class="badge badge-red">sem sessão</span>
								{/if}
							</div>
							<div class="wk-act" role="cell">
								{#if l.actions.includes('start') && !l.blocked}
									<input
										class="world mono"
										inputmode="numeric"
										placeholder="mundo"
										aria-label="Mundo de {l.rsn}"
										title="Mundo para entrar depois (301 a 999); vazio usa o do perfil. O worker para na tela de login."
										bind:value={worlds[l.account]}
									/>
								{/if}
								{#if l.cockpit}
									<button type="button" class="btn btn-s" onclick={() => openCockpit(l.account)}
										>Cockpit</button
									>
								{/if}
								{#each l.actions as a (a)}
									<button
										type="button"
										class="btn btn-s"
										class:btn-d={a === 'stop' || (a !== 'start' && armed === `${l.account}:${a}`)}
										class:btn-p={a === 'start' && !l.blocked}
										disabled={!!busy || (a === 'start' && !!l.blocked)}
										title={a === 'start'
											? (l.blocked ?? 'Sobe o pod; o cliente para na tela de login')
											: 'Sai do jogo'}
										onclick={() => act(l, a)}
									>
										{busy === `${l.account}:${a}`
											? '…'
											: armed === `${l.account}:${a}`
												? ACTION_CONFIRM[a]
												: actionLabel(l, a)}
									</button>
								{/each}
							</div>
							{#if l.blocked}
								<p class="why">
									{#if l.sessionWhen}{l.blocked}{:else}
										Conecte a conta no computador com o launcher:
										<code>{CONNECT}</code>
									{/if}
								</p>
							{/if}
						</div>
					{/each}
					{#each view.attached as a (a.id)}
						{#if !lines.some((l) => l.account === a.account)}
							<div class="wk-r" role="row">
								<div class="wk-c" role="cell">
									<span class="a">{a.account || 'sem conta declarada'}</span>
								</div>
								<div class="wk-c" role="cell" data-k="Onde">
									<span class="a">{a.host ?? 'zaros'}</span>
									<span class="b mono">anexado{a.version ? ` · ${a.version}` : ''}</span>
								</div>
								<div class="wk-c" role="cell" data-k="Estado">
									<span class="wk-s ok"><i></i>agent ligado</span>
								</div>
								<div class="wk-c" role="cell" data-k="Sessão">
									<span>launcher local</span>
									<span class="b">no {a.host ?? 'zaros'}</span>
								</div>
								<div class="wk-act" role="cell">
									<button
										type="button"
										class="btn btn-s"
										onclick={() => openCockpit(a.account ?? '')}>Cockpit</button
									>
								</div>
							</div>
						{/if}
					{/each}
				</div>
			{:else if !loadError}
				<p class="msg su">Lendo os workers…</p>
			{/if}
		</section>
		<p class="foot">
			Cliente de terceiros, fora do planner. Cada worker é um pod no namespace chronicle: Iniciar
			sobe o pod e para na tela de login; Parar sai do jogo e apaga o pod. Uma conta fica num lugar
			só.
		</p>
	{/if}
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
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
	.panel {
		display: flex;
		flex-direction: column;
	}
	.msg {
		margin: 0;
		padding: 14px 16px;
		font-size: 13px;
		line-height: 1.45;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.msg p {
		margin: 0;
	}
	.code {
		padding: 8px 10px;
		border-radius: 4px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--on-frame);
		overflow-x: auto;
	}
	code {
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.code code {
		white-space: nowrap;
	}
	.band {
		margin: 0;
		padding: 10px 14px;
		border: 1px solid var(--border);
		border-radius: 4px;
		font-size: 13px;
	}
	.band.bad {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.wk-t {
		display: grid;
		grid-template-columns:
			minmax(0, 1.1fr) minmax(0, 1fr) minmax(0, 1.8fr) minmax(0, 1fr)
			auto;
		column-gap: 12px;
	}
	.wk-r {
		display: grid;
		grid-column: 1 / -1;
		grid-template-columns: subgrid;
		row-gap: 4px;
		align-items: center;
		min-height: 64px;
		padding: 8px 16px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.wk-r:last-child {
		border-bottom: 0;
	}
	.wk-r.hd {
		min-height: 30px;
		padding-block: 6px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.right {
		text-align: right;
	}
	.wk-c {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.wk-c .a {
		font-weight: 600;
	}
	.wk-c .b {
		font-size: 11px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.wk-c .badge {
		width: max-content;
	}
	.wk-c .a .badge {
		margin-left: 4px;
		height: 16px;
		font-size: 9px;
		vertical-align: 1px;
	}
	.wk-s {
		display: inline-flex;
		align-items: baseline;
		gap: 7px;
		font-weight: 500;
	}
	.wk-s i {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: 0 0 auto;
		background: var(--ink-subtle);
	}
	.wk-s.ok i {
		background: var(--done);
	}
	.wk-s.busy i {
		background: var(--auto);
	}
	.wk-s.warn i {
		background: var(--not-started);
	}
	.wk-s.idle i {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
		box-sizing: border-box;
	}
	.wk-s.busy {
		color: var(--auto);
	}
	.wk-s.warn {
		color: var(--not-started);
	}
	.wk-act {
		display: flex;
		gap: 6px;
		justify-content: flex-end;
		flex-wrap: wrap;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.world {
		width: 72px;
		height: 32px;
		box-sizing: border-box;
		padding: 0 8px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--ground);
		box-shadow: var(--bevel);
		color: var(--ink);
		font-size: 12px;
	}
	.why {
		grid-column: 1 / -1;
		margin: 0;
		font-size: 12px;
		color: var(--ink-muted);
		overflow-wrap: anywhere;
	}
	.foot {
		margin: 0;
		font-size: 11px;
		color: var(--ink-subtle);
	}
	@media (max-width: 900px) {
		.wk-r.hd {
			display: none;
		}
		.wk-t {
			display: flex;
			flex-direction: column;
		}
		.wk-r {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			align-items: start;
			gap: 10px 12px;
			padding-block: 12px;
		}
		.wk-c:first-child,
		.wk-c[data-k='Estado'],
		.wk-act {
			grid-column: 1 / -1;
		}
		.wk-c[data-k='Estado'] {
			order: -1;
		}
		.wk-c:first-child {
			order: -2;
		}
		.wk-c .b {
			white-space: normal;
		}
		.wk-act {
			justify-content: flex-start;
		}
	}
</style>
