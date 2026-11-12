<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { MicrobotError, workersApi, type WorkersView } from '#lib/api/microbot';
	import AutomacaoTabs from '#lib/v6/AutomacaoTabs.svelte';
	import { chooseCockpitAccount } from '#lib/progress/microbot.svelte';
	import {
		ACTION_CONFIRM,
		ACTION_LABEL,
		workerLines,
		type WorkerActionKind,
		type WorkerLine
	} from '#lib/progress/workers';

	/*
	 * Automação › Workers (página 7 do canvas: V6MicrobotWorkers1440 e os estados de
	 * V6MicrobotWorkersEstados1440; docs/11). Uma linha por conta: onde roda, estado, sessão e ações.
	 * Diverge do canvas por decisão do dono (2026-11-12: "tudo funcional primeiro, a aparência
	 * depois"): sem o painel de capacidade e sem o worker aberto à direita, que dependem das
	 * métricas da F7.
	 */
	let view = $state<WorkersView | null>(null);
	let off = $state(false);
	let loadError = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let worlds = $state<Record<string, string>>({});
	let now = $state(Date.now());

	const lines = $derived(workerLines(view, now));

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
			<p class="ey">Microbot · workers · doc 11</p>
			<h1 class="sr">Workers</h1>
		</div>
		<AutomacaoTabs current="workers" />
	</header>

	{#if off}
		<p class="pg-panel pad">
			Sem workers neste servidor: o controle do Microbot está desligado ou falta o
			<code>CHRONICLE_VAULT_KEY</code>.
		</p>
	{:else}
		{#if view && !view.enabled}
			<p class="note">
				Workers desligados neste servidor (<code>WORKERS=off</code>): as sessões ficam guardadas,
				mas nada sobe no cluster.
			</p>
		{/if}
		{#if loadError}<p class="err">Não deu para ler os workers: {loadError}</p>{/if}
		{#if failure}<p class="err">{failure}</p>{/if}

		<section class="pg-panel">
			<div class="head">
				<h2>Contas</h2>
				{#if view}<span class="meta">
						{view.workers.length} com sessão ou worker{view.image
							? ` · imagem atual ${view.image}`
							: ''}
					</span>{/if}
			</div>
			{#if view && lines.length === 0 && view.attached.length === 0}
				<p class="pad empty">
					Nenhuma conta conectada. No computador com o launcher do Microbot:
					<code>chronicle accounts connect --character &lt;nome do personagem&gt;</code>
				</p>
			{:else}
				<table>
					<thead>
						<tr><th>Conta</th><th>Onde</th><th>Estado</th><th>Sessão</th><th>Ações</th></tr>
					</thead>
					<tbody>
						{#each lines as l (l.account)}
							<tr>
								<td><strong>{l.rsn}</strong><br /><span class="meta">{l.account}</span></td>
								<td>
									{l.where}
									{#if l.outdated}<br /><span class="tag warn">versão antiga</span>{/if}
								</td>
								<td>
									<span class="dot {l.tone}"></span>{l.state}
									{#if l.detail}<br /><span class="meta" title={l.detail}>{l.detail}</span>{/if}
								</td>
								<td class:warnc={l.sessionTone === 'warn'}>{l.session}</td>
								<td class="acts">
									{#if l.actions.includes('start')}
										<input
											class="world"
											inputmode="numeric"
											placeholder="mundo"
											title="Mundo para entrar depois (301 a 999); vazio usa o do perfil. O worker para na tela de login."
											bind:value={worlds[l.account]}
										/>
									{/if}
									{#each l.actions as a (a)}
										<button
											type="button"
											class="pg-btn"
											class:btn-d={a !== 'start' || armed === `${l.account}:${a}`}
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
													: ACTION_LABEL[a]}
										</button>
									{/each}
									{#if l.cockpit}
										<button type="button" class="pg-btn" onclick={() => openCockpit(l.account)}
											>Cockpit</button
										>
									{/if}
									{#if l.blocked}<br /><span class="meta">{l.blocked}</span>{/if}
								</td>
							</tr>
						{/each}
						{#each view?.attached ?? [] as a (a.id)}
							{#if !lines.some((l) => l.account === a.account)}
								<tr>
									<td><strong>{a.account || 'sem conta declarada'}</strong></td>
									<td>{a.host ?? 'zaros'} · anexado</td>
									<td><span class="dot ok"></span>agent ligado · {a.version ?? ''}</td>
									<td class="meta">sessão no launcher local</td>
									<td class="acts">
										<button
											type="button"
											class="pg-btn"
											onclick={() => openCockpit(a.account ?? '')}>Cockpit</button
										>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			{/if}
		</section>
		<p class="meta foot">
			Cada worker é um pod no namespace <code>chronicle</code>: Iniciar sobe o pod e para na tela de
			login; Parar sai do jogo e apaga o pod. Uma conta fica num lugar só. Cliente de terceiros,
			fora do planner.
		</p>
	{/if}
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 16px;
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
	.head {
		display: flex;
		align-items: baseline;
		gap: 10px;
		padding: 12px 12px 4px;
	}
	.head h2 {
		font-size: 16px;
	}
	.pad {
		padding: 12px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		text-align: left;
		vertical-align: top;
		padding: 8px 12px;
		border-top: 1px solid var(--border);
	}
	th {
		font-weight: 500;
		color: var(--ink-muted);
		font-size: 12px;
	}
	.meta {
		color: var(--ink-subtle);
		font-size: 12px;
	}
	.acts {
		white-space: nowrap;
	}
	.acts .pg-btn + .pg-btn {
		margin-left: 6px;
	}
	.world {
		width: 64px;
		height: 28px;
		margin-right: 6px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--ground);
		color: var(--ink);
		font: inherit;
		padding: 0 6px;
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		margin-right: 6px;
		background: var(--ink-subtle);
	}
	.dot.ok {
		background: var(--ok, #3a8a3a);
	}
	.dot.busy {
		background: var(--gold);
	}
	.dot.warn {
		background: var(--danger, #b33);
	}
	.tag.warn,
	.warnc {
		color: var(--danger, #b33);
	}
	.err {
		color: var(--danger, #b33);
	}
	.note,
	.foot {
		margin: 0;
	}
	.empty code,
	.note code,
	.foot code {
		font-size: 12px;
	}
	@media (max-width: 900px) {
		table,
		thead,
		tbody,
		tr,
		td,
		th {
			display: block;
		}
		thead {
			display: none;
		}
		.acts {
			white-space: normal;
		}
	}
</style>
