<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import {
		MicrobotError,
		microbotApi,
		workersApi,
		type ClientStatus,
		type WorkersView
	} from '#lib/api/microbot';
	import { rememberAccount } from '#lib/progress/account.svelte';
	import { fleet, loadFleet } from '#lib/progress/fleet.svelte';
	import { fleetSummary, lanes, needs, type FleetInput, type Lane } from '#lib/progress/frota';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { chooseCockpitAccount } from '#lib/progress/microbot.svelte';
	import type { AccountState, ProgressSummary, TimerView } from '#lib/progress/types';
	import { actionLabel, workerLines, type WorkerActionKind } from '#lib/progress/workers';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Frota (página 8 do canvas, V7Frota1440): uma raia por conta. Primeira fatia: só o que a API
	 * já tem. Ficam de fora do artboard, por faltar no servidor, o ritmo de XP e os ganhos da
	 * sessão, a fila de rotinas (aqui "Depois" são os relógios do Time Tracking), o diário, GP e a
	 * capacidade do saradomin no resumo.
	 */
	interface Facts {
		state: AccountState | null;
		summary: ProgressSummary | null;
		timers: TimerView[];
	}
	let facts = $state<Record<string, Facts>>({});
	let workers = $state<WorkersView | null>(null);
	let clients = $state<Record<string, { client: ClientStatus | null; scripts: string[] }>>({});
	let now = $state(Date.now());
	let armed = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);

	const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

	async function loadFacts() {
		const accounts = fleet.accounts ?? [];
		const all = await Promise.all(
			accounts.map(async (a) => {
				const [state, summary, timers] = await Promise.all([
					quiet(progressApi.accountState(a.id)),
					quiet(progressApi.summary(a.id)),
					quiet(progressApi.timers(a.id))
				]);
				return [a.id, { state, summary, timers: timers?.timers ?? [] }] as const;
			})
		);
		facts = Object.fromEntries(all);
		now = Date.now();
	}

	async function loadWorkers() {
		try {
			workers = await workersApi.list();
		} catch (e) {
			// 404: sem o módulo do Microbot ou sem cofre de sessões; a Frota segue só com as contas.
			if (!(e instanceof MicrobotError && e.status === 404)) failure = String(e);
			workers = null;
		}
		const lines = workerLines(workers, Date.now());
		const next: typeof clients = {};
		await Promise.all(
			lines
				.filter((l) => l.cockpit)
				.map(async (l) => {
					const [status, plugins] = await Promise.all([
						quiet(microbotApi.statusOf(l.account)),
						quiet(microbotApi.pluginsOf(l.account))
					]);
					next[l.account] = {
						client: status?.client ?? null,
						scripts: (plugins?.scripts.scripts ?? []).filter((s) => s.active).map((s) => s.name)
					};
				})
		);
		clients = next;
		now = Date.now();
	}

	$effect(() => {
		void loadFleet(true);
	});
	$effect(() => {
		if (!fleet.accounts) return;
		void loadFacts();
		const t = setInterval(() => void loadFacts(), 60_000);
		return () => clearInterval(t);
	});
	$effect(() => {
		void loadWorkers();
		const t = setInterval(() => void loadWorkers(), 5000);
		return () => clearInterval(t);
	});

	const all = $derived.by(() => {
		const lines = workerLines(workers, now);
		const inputs: FleetInput[] = (fleet.accounts ?? []).map((a) => ({
			account: a,
			state: facts[a.id]?.state ?? null,
			summary: facts[a.id]?.summary ?? null,
			timers: facts[a.id]?.timers ?? [],
			worker: lines.find((l) => l.account === a.id) ?? null,
			client: clients[a.id]?.client ?? null,
			scripts: clients[a.id]?.scripts ?? []
		}));
		return lanes(inputs, now);
	});
	const attention = $derived(needs(all));
	$effect(() => {
		fleet.tones = Object.fromEntries(all.map((l) => [l.id, l.tone]));
	});

	function open(l: Lane, to: '/' | '/automacao' = '/') {
		rememberAccount(l.id);
		if (to === '/automacao') chooseCockpitAccount(l.id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da conta
		void goto(`${resolve(to)}?account=${encodeURIComponent(l.id)}`);
	}

	async function act(l: Lane, action: WorkerActionKind) {
		const key = `${l.id}:${action}`;
		if (armed !== key) {
			armed = key;
			return;
		}
		armed = null;
		busy = key;
		failure = null;
		try {
			await workersApi.action(l.id, action);
			await loadWorkers();
		} catch (e) {
			failure = `${l.rsn}: ${e instanceof Error ? e.message : String(e)}`;
		} finally {
			busy = null;
		}
	}

	const playing = $derived(all.filter((l) => l.tone === 'ok').length);
	const managed = $derived(workers?.workers.filter((w) => w.desired === 'running').length ?? 0);
	const soonest = $derived(
		all
			.flatMap((l) => l.next.map((n) => ({ ...n, rsn: l.rsn })))
			.sort((a, b) => Number(b.ready) - Number(a.ready))[0]
	);
</script>

<svelte:head><title>Frota · Chronicle</title></svelte:head>

<header class="top">
	<div class="ttl">
		<h1 class="sr">Frota</h1>
		<p class="mu">{fleet.accounts ? fleetSummary(all) : 'Lendo as contas…'}</p>
	</div>
	<a class="btn btn-p" href={resolve('/setup')}><Glyph name="plus" size={14} />Nova conta</a>
</header>

{#if failure}<p class="band">{failure}</p>{/if}

{#if all.length}
	<section class="resumo" aria-label="Resumo da frota">
		<div class="ri">
			<span class="rk">No jogo</span>
			<span class="rv"
				>{playing} <small>de {all.length} {all.length === 1 ? 'conta' : 'contas'}</small></span
			>
			<div class="pips" aria-hidden="true">
				{#each all as l (l.id)}<i class={l.tone}></i>{/each}
			</div>
		</div>
		{#if workers}
			<div class="ri">
				<span class="rk">Workers</span>
				<span class="rv"
					>{managed}
					<small>{workers.enabled ? 'no saradomin' : 'desligados neste servidor'}</small></span
				>
				<span class="rd"
					>{workers.attached.length
						? `${workers.attached.length} cliente anexado`
						: 'nenhum cliente anexado'}</span
				>
			</div>
		{/if}
		<div class="ri ri-next">
			<span class="rk">Próximo relógio</span>
			{#if soonest}
				<span class="rv">{soonest.label} · {soonest.rsn}</span>
				<span class="rd">{soonest.ready ? 'pronto agora' : `em ${soonest.when}`}</span>
			{:else}
				<span class="rv none">nenhum relógio contando</span>
			{/if}
		</div>
	</section>
{/if}

<div class="cols">
	<section class="lanes" aria-label="Contas">
		{#each all as l (l.id)}
			{@const face = l.face ? iconFor(`skill:${l.face.skill}`) : undefined}
			<article class="lane" class:lane-off={l.tone === 'off'}>
				<div class="stage" class:live={l.tone === 'ok'} class:dim={l.tone === 'off'}>
					{#if face}<img src={face} alt={l.face?.skill} />{:else}<span
							class="ini"
							aria-hidden="true">{l.rsn.charAt(0).toUpperCase()}</span
						>{/if}
					{#if l.face}<span class="lv" title="{l.face.skill} {l.face.level}">{l.face.level}</span
						>{/if}
				</div>
				<div class="now">
					<div class="who">
						<span class="lamp {l.tone}"></span>
						<h2><button type="button" class="name" onclick={() => open(l)}>{l.rsn}</button></h2>
						<span class="m">{l.meta}</span>
						{#if l.where}<span class="tag" title={l.worker?.runtime}>{l.where}</span>{/if}
					</div>
					<p class="act {l.tone}">
						<b>{l.activity}</b>{#if l.detail}<span title={l.detail}>{l.detail}</span>{/if}
					</p>
					{#if l.route}
						<div class="prog">
							<span class="l">{l.route.label}</span>
							<div
								class="track"
								class:live={l.tone === 'ok'}
								role="progressbar"
								aria-valuenow={l.route.pct}
								aria-valuemin="0"
								aria-valuemax="100"
								aria-label="{l.route.label}: {l.route.done} de {l.route.total} passos"
							>
								<span style="width: {l.route.pct}%"></span>
							</div>
							<span>{l.route.done} / {l.route.total}{l.route.ref ? ` · ${l.route.ref}` : ''}</span>
						</div>
					{/if}
				</div>
				<div class="queue">
					<span class="k">Depois</span>
					{#each l.next as n (n.label)}
						<div class="q">
							<span>{n.label}</span><span class="w" class:rdy={n.ready}>{n.when}</span>
						</div>
					{:else}
						<div class="q none"><span>nenhum relógio</span></div>
					{/each}
				</div>
				<div class="ctl">
					{#if l.client}
						<button type="button" class="btn btn-s" onclick={() => open(l, '/automacao')}
							><Glyph name="eye" size={13} />Ver jogo</button
						>
					{:else}
						<button type="button" class="btn btn-s" onclick={() => open(l)}>Abrir</button>
					{/if}
					{#if l.worker?.actions.length}
						<div class="rowb">
							{#each l.worker.actions as a (a)}
								<button
									type="button"
									class="btn btn-s"
									class:btn-d={a === 'stop' || (a !== 'start' && armed === `${l.id}:${a}`)}
									disabled={!!busy || (a === 'start' && !!l.worker.blocked)}
									title={a === 'start'
										? (l.worker.blocked ?? 'Sobe o pod; o cliente para na tela de login')
										: a === 'restart' && l.worker.outdated
											? 'Reinicia na versão nova; sai do jogo'
											: 'Sai do jogo'}
									onclick={() => act(l, a)}
								>
									{busy === `${l.id}:${a}`
										? '…'
										: armed === `${l.id}:${a}`
											? 'Confirmar'
											: a === 'restart' && l.worker.outdated
												? 'Atualizar'
												: actionLabel(l.worker, a)}
								</button>
							{/each}
						</div>
					{/if}
				</div>
			</article>
		{:else}
			{#if fleet.accounts}
				<p class="mu empty">Nenhuma conta ainda.</p>
			{/if}
		{/each}
		<a class="lane lane-new" href={resolve('/setup')}>
			<span class="stage new"><Glyph name="plus" size={22} /></span>
			<span class="now"
				><b>Nova conta</b><span class="mu"
					>Entra na frota só acompanhada; a sessão do jogo vem depois.</span
				></span
			>
		</a>
	</section>

	{#if attention.length}
		<aside class="rail">
			<div class="rh">
				<h2>Precisa de você</h2>
				<span>{attention.length}</span>
			</div>
			{#each attention as n (n.account + n.text)}
				<div class="need" class:soft={!n.hard}>
					<Glyph name="alert" size={16} />
					<p><b>{n.rsn}</b> {n.text}</p>
					{#if n.sub}<p class="s">{n.sub}</p>{/if}
				</div>
			{/each}
		</aside>
	{/if}
</div>

<p class="foot">{ICON_ATTRIBUTION}. Cliente de terceiros, fora do planner.</p>

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: auto;
	}
	.ttl h1 {
		font-size: 30px;
	}
	.ttl p {
		margin: 0;
		font-size: 13px;
	}
	.band {
		margin: 0;
	}
	@keyframes flow {
		from {
			background-position: 0 0;
		}
		to {
			background-position: 28px 0;
		}
	}
	@keyframes bob {
		0%,
		100% {
			transform: translateY(0);
		}
		50% {
			transform: translateY(-3px);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.stage img,
		.track span {
			animation: none !important;
		}
	}
	.resumo {
		display: flex;
		flex-wrap: wrap;
		border-radius: 6px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--on-frame);
	}
	.ri {
		flex: 1 1 170px;
		display: flex;
		flex-direction: column;
		gap: 3px;
		padding: 12px 18px;
		min-width: 0;
		border-left: 1px solid var(--frame-2);
	}
	.ri:first-child {
		border-left: 0;
	}
	.rk {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--on-frame-muted);
	}
	.rv {
		font:
			700 22px/1.1 Alegreya,
			Georgia,
			serif;
		color: var(--gold);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rv small {
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
		color: var(--on-frame-muted);
	}
	.rd {
		font-size: 11px;
		color: var(--on-frame-muted);
	}
	.ri-next {
		flex: 1.6 1 260px;
	}
	.ri-next .rv {
		font:
			600 14px/1.5 Inter,
			system-ui,
			sans-serif;
		color: var(--on-frame);
	}
	.ri-next .rv.none {
		font-weight: 400;
		color: var(--on-frame-muted);
	}
	.pips {
		display: flex;
		gap: 4px;
		max-width: 160px;
	}
	.pips i {
		flex: 1;
		max-width: 24px;
		height: 10px;
		border-radius: 2px;
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
	}
	.pips i.ok {
		background: var(--gold);
	}
	.pips i.up {
		background: var(--auto);
	}
	.pips i.bad {
		background: var(--not-started);
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
	}
	.lanes {
		flex: 999 1 640px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.lane {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr) 190px 164px;
		gap: 16px;
		align-items: center;
		padding: 14px 16px;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		color: var(--ink);
		text-decoration: none;
	}
	.lane-off {
		background: transparent;
		box-shadow: none;
	}
	.lane-new {
		background: transparent;
		box-shadow: none;
		border-style: dashed;
		grid-template-columns: 76px minmax(0, 1fr);
		padding-block: 10px;
	}
	.lane-new:hover {
		border-color: var(--gold-ink);
	}
	.lane-new .now b {
		font-size: 14px;
	}
	.lane-new .now .mu {
		font-size: 12px;
	}
	.stage {
		position: relative;
		width: 76px;
		height: 76px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: inset 0 2px 6px rgba(0, 0, 0, 0.14);
		display: flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
	}
	.stage img {
		width: 44px;
		height: 44px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.stage.live img {
		animation: bob 1.8s ease-in-out infinite;
	}
	.stage.dim img {
		opacity: 0.5;
		filter: grayscale(1);
	}
	.stage .lv {
		position: absolute;
		right: -6px;
		bottom: -6px;
		min-width: 26px;
		height: 22px;
		padding: 0 5px;
		box-sizing: border-box;
		border-radius: 3px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--gold);
		font:
			700 14px/22px Alegreya,
			Georgia,
			serif;
		text-align: center;
	}
	.stage.new {
		width: 56px;
		height: 56px;
		justify-self: center;
		border-style: dashed;
		background: transparent;
		box-shadow: none;
		color: var(--ink-subtle);
	}
	.now {
		display: flex;
		flex-direction: column;
		gap: 5px;
		min-width: 0;
	}
	.who {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.who h2 {
		margin: 0;
		min-width: 0;
	}
	.name {
		border: 0;
		background: transparent;
		padding: 0;
		cursor: pointer;
		color: var(--ink);
		font:
			700 18px/1.15 Alegreya,
			Georgia,
			serif;
		white-space: nowrap;
	}
	.name:hover {
		color: var(--gold-ink);
	}
	.who .m {
		font-size: 12px;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.tag {
		display: inline-flex;
		align-items: center;
		height: 18px;
		padding: 0 6px;
		margin-left: auto;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		white-space: nowrap;
	}
	.lamp {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		box-sizing: border-box;
		background: var(--ink-subtle);
	}
	.lamp.ok {
		background: var(--done);
		box-shadow: 0 0 0 3px var(--done-tint);
	}
	.lamp.up {
		background: var(--auto);
	}
	.lamp.bad {
		background: var(--not-started);
	}
	.lamp.off {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	.act {
		display: flex;
		align-items: baseline;
		gap: 8px;
		min-width: 0;
		margin: 0;
		font-size: 14px;
	}
	.act b {
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}
	.act span {
		font-size: 12px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1 1 0;
		min-width: 0;
	}
	.act.up b {
		color: var(--auto);
	}
	.act.bad b {
		color: var(--not-started);
	}
	.prog {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 10px;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
	}
	.prog .l {
		color: var(--ink);
	}
	.track {
		height: 12px;
		border-radius: 3px;
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
		overflow: hidden;
	}
	.track span {
		display: block;
		height: 100%;
		background-color: var(--done);
	}
	.track.live span {
		background-image: linear-gradient(
			110deg,
			rgba(255, 255, 255, 0.16) 0 25%,
			transparent 25% 50%,
			rgba(255, 255, 255, 0.16) 50% 75%,
			transparent 75%
		);
		background-size: 28px 12px;
		animation: flow 1.1s linear infinite;
	}
	.queue {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
		padding-left: 14px;
		border-left: 1px solid var(--hairline);
		align-self: stretch;
		justify-content: center;
	}
	.queue .k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.q {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: center;
		gap: 7px;
		font-size: 12px;
		min-height: 22px;
	}
	.q span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.q .w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.q .w.rdy {
		color: var(--done);
		font-weight: 600;
	}
	.q.none {
		color: var(--ink-subtle);
	}
	.ctl {
		display: flex;
		flex-direction: column;
		gap: 6px;
		align-items: stretch;
	}
	.ctl .btn {
		padding: 0 8px;
	}
	.rowb {
		display: flex;
		gap: 6px;
	}
	.rowb .btn {
		flex: 1 1 0;
		min-width: 0;
	}
	.ini {
		font:
			700 30px Alegreya,
			Georgia,
			serif;
		color: var(--ink-subtle);
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.rail {
		flex: 1 1 280px;
		max-width: 320px;
		min-width: 0;
	}
	.rh {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 0 2px 8px;
		border-bottom: 2px solid var(--frame);
	}
	.rh h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.rh span {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.need {
		display: grid;
		grid-template-columns: 20px minmax(0, 1fr);
		gap: 4px 10px;
		padding: 12px 2px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
		color: var(--not-started);
	}
	.need.soft {
		color: var(--stale);
	}
	.need p {
		margin: 0;
		line-height: 1.35;
		color: var(--ink);
		overflow-wrap: anywhere;
	}
	.need p b {
		font-weight: 600;
	}
	.need .s {
		grid-column: 2;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.empty {
		margin: 0;
	}
	.foot {
		margin: 0;
		font-size: 11px;
		color: var(--ink-subtle);
	}
	@media (max-width: 900px) {
		.lane {
			grid-template-columns: 76px minmax(0, 1fr);
		}
		.queue {
			grid-column: 1 / -1;
			border-left: 0;
			padding-left: 0;
			border-top: 1px solid var(--hairline);
			padding-top: 10px;
		}
		.ctl {
			grid-column: 1 / -1;
			flex-direction: row;
			flex-wrap: wrap;
		}
		.who {
			flex-wrap: wrap;
		}
	}
</style>
