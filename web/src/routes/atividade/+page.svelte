<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { MicrobotError, microbotApi, workersApi, type WorkersView } from '#lib/api/microbot';
	import { rememberAccount } from '#lib/progress/account.svelte';
	import { accountPills, diary, fleetRuns, runsHeader, xpRows } from '#lib/progress/atividade';
	import { compact } from '#lib/progress/ritmo';
	import { fleet, loadFleet } from '#lib/progress/fleet.svelte';
	import { lanes, needs, type FleetInput, type Need } from '#lib/progress/frota';
	import { dayStart } from '#lib/progress/historico';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { microbotModule, probeMicrobot } from '#lib/progress/microbot.svelte';
	import type { FleetLevels, FleetRhythm, TimerView } from '#lib/progress/types';
	import { workerLines } from '#lib/progress/workers';
	import type { Run } from '#lib/api/microbot';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Atividade (página 8 do canvas, V7Atividade1440): o que pede uma decisão do dono e o que a frota
	 * fez hoje sozinha. Fatia V7 de decisoes-canvas.md, 7.5; o que o artboard mostra e a API não
	 * mede ficou de fora (seção 7.7).
	 */
	const CAP = 8;
	let workers = $state<WorkersView | null>(null);
	let timers = $state<Record<string, TimerView[]>>({});
	let runs = $state<Record<string, Run[]> | null>(null);
	let rhythm = $state<FleetRhythm | null>(null);
	let levels = $state<FleetLevels | null>(null);
	let loaded = $state(false);
	let failure = $state<string | null>(null);
	let now = $state(Date.now());
	let only = $state<string | null>(null);
	let morePills = $state(false);
	let allRuns = $state(false);
	let allDiary = $state(false);
	let armed = $state<string | null>(null);
	let busy = $state<string | null>(null);

	const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

	async function load(microbot: boolean) {
		const accounts = fleet.accounts ?? [];
		const midnight = new Date(dayStart(Date.now())).toISOString();
		const [w, rh, lv, tm, rs] = await Promise.all([
			workersApi.list().catch((e: unknown) => {
				// 404: sem o módulo do Microbot ou sem cofre de sessões; a tela segue sem os workers.
				if (!(e instanceof MicrobotError && e.status === 404)) failure = String(e);
				return null;
			}),
			quiet(progressApi.fleetRhythm(midnight)),
			quiet(progressApi.fleetLevels(midnight)),
			Promise.all(
				accounts.map(
					async (a) => [a.id, (await quiet(progressApi.timers(a.id)))?.timers ?? []] as const
				)
			),
			microbot
				? Promise.all(
						accounts.map(
							async (a) => [a.id, (await quiet(microbotApi.runsOf(a.id)))?.runs ?? []] as const
						)
					)
				: null
		]);
		workers = w;
		rhythm = rh;
		levels = lv;
		timers = Object.fromEntries(tm);
		runs = rs ? Object.fromEntries(rs) : null;
		now = Date.now();
		loaded = true;
	}

	$effect(() => {
		void probeMicrobot();
		void loadFleet();
	});
	$effect(() => {
		if (!fleet.accounts) return;
		const microbot = microbotModule.enabled === true;
		void load(microbot);
		const t = setInterval(() => {
			if (!document.hidden) void load(microbot);
		}, 30_000);
		return () => clearInterval(t);
	});

	const accounts = $derived(fleet.accounts ?? []);
	const names = $derived(Object.fromEntries(accounts.map((a) => [a.id, a.rsn])));
	const pills = $derived(accountPills(accounts, only, morePills));
	// Para "Precisa de você" bastam o worker e os relógios de cada conta (frota.ts, needs).
	const attention = $derived.by(() => {
		const lines = workerLines(workers, now);
		const inputs: FleetInput[] = accounts.map((a) => ({
			account: a,
			state: null,
			summary: null,
			timers: timers[a.id] ?? [],
			worker: lines.find((l) => l.account === a.id) ?? null,
			client: null,
			scripts: []
		}));
		return needs(lanes(inputs, now)).filter((n) => !only || n.account === only);
	});
	const runRows = $derived(
		runs
			? fleetRuns(
					accounts.map((a) => ({
						account: a.id,
						rsn: a.rsn,
						runs: runs?.[a.id] ?? [],
						levels: (levels?.events ?? []).filter((e) => e.account === a.id)
					})),
					dayStart(now),
					now
				).filter((r) => !only || r.account === only)
			: null
	);
	const xp = $derived(xpRows(rhythm, names, only ?? undefined));
	const days = $derived(diary(levels?.events ?? [], names, only ?? undefined));

	function openAccount(id: string) {
		rememberAccount(id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da conta
		void goto(`${resolve('/visao')}?account=${encodeURIComponent(id)}`);
	}

	async function restart(n: Need) {
		const key = `${n.account}:restart`;
		if (armed !== key) {
			armed = key;
			return;
		}
		armed = null;
		busy = key;
		failure = null;
		try {
			await workersApi.action(n.account, 'restart');
			await load(microbotModule.enabled === true);
		} catch (e) {
			failure = `${n.rsn}: ${e instanceof Error ? e.message : String(e)}`;
		} finally {
			busy = null;
		}
	}
</script>

<svelte:head><title>Atividade · Chronicle</title></svelte:head>

<header class="top">
	<div class="ttl">
		<h1 class="sr">Atividade</h1>
		<p class="mu">O que pede uma decisão sua e o que a frota fez sozinha.</p>
	</div>
	{#if accounts.length > 1}
		<div class="flt" role="group" aria-label="Conta">
			<button
				type="button"
				class:on={only === null}
				aria-pressed={only === null}
				onclick={() => (only = null)}>Todas as contas</button
			>
			{#each pills.shown as a (a.id)}
				<button
					type="button"
					class:on={only === a.id}
					aria-pressed={only === a.id}
					onclick={() => (only = only === a.id ? null : a.id)}>{a.rsn}</button
				>
			{/each}
			{#if pills.more}
				<button type="button" onclick={() => (morePills = true)}>+{pills.more}</button>
			{/if}
		</div>
	{/if}
</header>

{#if failure}<p class="band">{failure}</p>{/if}

{#if attention.length}
	<section class="needs" aria-label="Precisa de você">
		<div class="rh">
			<h2>Precisa de você</h2>
			<span>{attention.length}</span>
		</div>
		{#each attention as n (n.account + n.kind + n.text)}
			<div class="need" class:soft={!n.hard}>
				<Glyph name="alert" size={18} />
				<p><b>{n.rsn}</b> {n.text}</p>
				{#if n.sub}<p class="s">{n.sub}</p>{/if}
				<div class="a">
					{#if n.kind === 'outdated'}
						<button
							type="button"
							class="btn btn-s"
							class:btn-d={armed === `${n.account}:restart`}
							disabled={!!busy}
							title="Volta na versão nova; sai do jogo"
							onclick={() => restart(n)}
							>{busy === `${n.account}:restart`
								? '…'
								: armed === `${n.account}:restart`
									? 'Confirmar'
									: 'Reiniciar agora'}</button
						>
					{/if}
					<button type="button" class="btn btn-s btn-g" onclick={() => openAccount(n.account)}
						>Abrir a conta</button
					>
				</div>
			</div>
		{/each}
	</section>
{/if}

<div class="cols">
	{#if runRows}
		<section class="card runs" aria-label="Execuções">
			<div class="h">
				<h2>Execuções</h2>
				<span>{runsHeader(runRows)}</span>
			</div>
			{#if runRows.length}
				<div class="run hd" aria-hidden="true">
					<span>Hora</span><span>Conta</span><span></span><span>O que rodou</span><span>Durou</span
					><span class="end">Fim</span>
				</div>
				{#each allRuns ? runRows : runRows.slice(0, CAP) as r (r.key)}
					{@const sprite = r.skill ? iconFor(`skill:${r.skill}`) : undefined}
					<div class="run">
						<span class="t">{r.time}</span>
						<span class="n"
							><span class="lamp {fleet.tones[r.account] ?? 'unk'}"></span><b>{r.rsn}</b></span
						>
						<span class="ic"
							>{#if sprite}<img src={sprite} alt="" />{:else}<Glyph
									name="sync"
									size={16}
								/>{/if}</span
						>
						<span class="x"
							><b>{r.name}</b>{#if r.sub}
								<span>· {r.sub}</span>{/if}</span
						>
						<span class="d">{r.span}</span>
						<span class="res {r.end.tone}">{r.end.label}</span>
					</div>
				{/each}
				{#if !allRuns && runRows.length > CAP}
					<button type="button" class="lnk more" onclick={() => (allRuns = true)}
						>mais {runRows.length - CAP} de hoje</button
					>
				{/if}
			{:else}
				<p class="mu empty">{loaded ? 'Nenhuma execução hoje.' : 'Lendo…'}</p>
			{/if}
		</section>
	{/if}

	<aside class="rail">
		<section class="card">
			<div class="h">
				<h2>XP hoje</h2>
				{#if xp.total}<span>{compact(xp.total)}{only ? '' : ' na frota'}</span>{/if}
			</div>
			<div class="b">
				{#each xp.rows as r (r.account)}
					<div class="xpd">
						<span class="nm">{r.rsn}</span>
						<span class="bar7"><span style="width: {r.pct}%"></span></span>
						<span class="v7">{r.label}</span>
					</div>
				{:else}
					<p class="mu">{loaded ? 'Nenhum XP hoje ainda.' : 'Lendo…'}</p>
				{/each}
			</div>
		</section>
		<section>
			<div class="rh">
				<h2>Diário</h2>
				<span>hoje</span>
			</div>
			{#each allDiary ? days : days.slice(0, CAP) as d (d.key)}
				{@const sprite = iconFor(`skill:${d.skill}`)}
				<div class="feed">
					<span class="t">{d.time}</span>
					{#if sprite}<img src={sprite} alt="" />{:else}<span></span>{/if}
					<span><b>{d.rsn}</b> {d.text}</span>
				</div>
			{:else}
				<p class="mu empty-d">{loaded ? 'Nenhum nível hoje ainda.' : 'Lendo…'}</p>
			{/each}
			{#if !allDiary && days.length > CAP}
				<button type="button" class="lnk more" onclick={() => (allDiary = true)}
					>mais {days.length - CAP} de hoje</button
				>
			{/if}
		</section>
	</aside>
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
	.flt {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.flt button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 32px;
		padding: 0 12px;
		border-radius: 16px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--ink-muted);
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		cursor: pointer;
	}
	.flt button.on {
		background: var(--frame);
		border-color: var(--frame);
		color: var(--gold);
	}
	.needs {
		display: flex;
		flex-direction: column;
		gap: 10px;
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
		grid-template-columns: 22px minmax(0, 1fr) auto;
		gap: 4px 12px;
		align-items: center;
		padding: 14px 16px;
		border-radius: 6px;
		border: 1px solid var(--not-started);
		background: var(--surface);
		box-shadow: var(--bevel);
		font-size: 13px;
	}
	.need.soft {
		border-color: var(--stale);
	}
	.need > :global(svg) {
		color: var(--not-started);
		grid-row: 1 / 3;
		align-self: start;
		margin-top: 2px;
	}
	.need.soft > :global(svg) {
		color: var(--stale);
	}
	.need p {
		margin: 0;
		line-height: 1.4;
	}
	.need p b {
		font-weight: 600;
	}
	.need .s {
		grid-column: 2;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.need .a {
		grid-row: 1 / 3;
		grid-column: 3;
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		justify-content: flex-end;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
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
	.card > .h {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 12px 16px 0;
	}
	.runs > .h {
		padding-bottom: 10px;
	}
	.card > .h h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.card > .h span {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.card > .b {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 12px 16px 16px;
	}
	.card p {
		margin: 0;
		font-size: 13px;
	}
	.runs {
		flex: 999 1 600px;
	}
	.empty {
		padding: 0 16px 16px;
	}
	.run {
		display: grid;
		grid-template-columns: 48px 130px 22px minmax(0, 1fr) 60px 70px;
		gap: 12px;
		align-items: center;
		min-height: 42px;
		padding: 0 16px;
		border-top: 1px solid var(--hairline);
		font-size: 13px;
	}
	.run.hd {
		min-height: 30px;
		border-top: 0;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.run .end,
	.run .res {
		text-align: right;
	}
	.run .t,
	.run .d {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.run .n {
		display: flex;
		align-items: center;
		gap: 7px;
		min-width: 0;
	}
	.run .n b {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.run .ic {
		display: inline-flex;
		color: var(--ink-subtle);
	}
	.run .ic img {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.run .x {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.run .x b {
		font-weight: 600;
	}
	.run .x span {
		color: var(--ink-muted);
		font-size: 12px;
	}
	.res {
		font:
			600 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
	}
	.res.ok {
		color: var(--done);
	}
	.res.bad {
		color: var(--not-started);
	}
	.res.off {
		color: var(--ink-subtle);
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
	.lamp.off,
	.lamp.unk {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	.more {
		min-height: 40px;
		width: max-content;
		padding: 0 16px;
		border: 0;
		background: transparent;
		cursor: pointer;
		font-family: inherit;
	}
	.rail {
		flex: 1 1 300px;
		max-width: 340px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.xpd {
		display: grid;
		grid-template-columns: 110px minmax(0, 1fr) 54px;
		gap: 10px;
		align-items: center;
		min-height: 30px;
		font-size: 13px;
	}
	.xpd .nm {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.bar7 {
		height: 10px;
		border-radius: 2px;
		background: var(--hairline);
		overflow: hidden;
	}
	.bar7 span {
		display: block;
		height: 100%;
		background: var(--done);
	}
	.v7 {
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		text-align: right;
	}
	.feed {
		display: grid;
		grid-template-columns: 38px 18px minmax(0, 1fr);
		gap: 8px;
		align-items: start;
		padding: 8px 2px;
		border-bottom: 1px solid var(--hairline);
		font-size: 12px;
		line-height: 1.35;
	}
	.feed:last-of-type {
		border-bottom: 0;
	}
	.feed .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		padding-top: 1px;
	}
	.feed img {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.feed b {
		font-weight: 600;
	}
	.empty-d {
		margin: 8px 2px 0;
		font-size: 12px;
	}
	@media (max-width: 900px) {
		.need {
			grid-template-columns: 22px minmax(0, 1fr);
		}
		.need .a {
			grid-row: auto;
			grid-column: 2;
			justify-content: flex-start;
		}
		.run {
			grid-template-columns: 44px minmax(0, 1fr) 70px;
			padding-block: 6px;
		}
		.run.hd,
		.run .ic,
		.run .d {
			display: none;
		}
		.run .x {
			grid-column: 1 / -1;
			order: 5;
			white-space: normal;
		}
		.rail {
			max-width: none;
		}
	}
</style>
