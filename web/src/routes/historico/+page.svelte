<script lang="ts">
	import { progressApi } from '#lib/api/client';
	import { microbotApi, type AuditEntry, type Run } from '#lib/api/microbot';
	import { currentAccount } from '#lib/progress/account.svelte';
	import {
		counts,
		dayStart,
		feedDays,
		feedItems,
		weekGains,
		weekRuns,
		windowStart,
		type FeedFilter
	} from '#lib/progress/historico';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { me } from '#lib/progress/me.svelte';
	import { microbotModule, probeMicrobot } from '#lib/progress/microbot.svelte';
	import type { AccountLevels } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Conta › Histórico (página 8 do canvas, V7Historico1440): o que a conta fez nos últimos 7 dias,
	 * por dia, com as execuções, os níveis, as ações do dono e os problemas. Fatia V7 de
	 * decisoes-canvas.md, 7.5; o que o artboard mostra e a API não mede ficou de fora (seção 7.7).
	 */
	const DAY_CAP = 8;
	const account = $derived(currentAccount());
	let levels = $state<AccountLevels | null>(null);
	let runs = $state<Run[] | null>(null);
	let audit = $state<AuditEntry[] | null>(null);
	let xpToday = $state<number | null>(null);
	let loaded = $state(false);
	let failure = $state<string | null>(null);
	let now = $state(Date.now());
	let filter = $state<FeedFilter>('all');
	let open = $state<string | null>(null);
	let opened = $state<Record<string, boolean>>({});

	const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

	async function load(id: string, microbot: boolean) {
		const t = Date.now();
		const iso = (ms: number) => new Date(ms).toISOString();
		const [lv, rh, rs, au] = await Promise.all([
			progressApi.levels(id, iso(windowStart(t))).catch((e: unknown) => {
				failure = e instanceof Error ? e.message : String(e);
				return null;
			}),
			quiet(progressApi.rhythm(id, iso(dayStart(t)))),
			microbot ? quiet(microbotApi.runsOf(id)) : null,
			microbot ? quiet(microbotApi.auditOf(id)) : null
		]);
		if (id !== account) return;
		if (lv) failure = null;
		levels = lv;
		xpToday = rh?.today.xp ?? null;
		runs = rs?.runs ?? (microbot ? [] : null);
		audit = au?.entries ?? (microbot ? [] : null);
		now = Date.now();
		loaded = true;
	}

	$effect(() => {
		void probeMicrobot();
	});
	$effect(() => {
		const id = account;
		const microbot = microbotModule.enabled === true;
		if (!id) return;
		levels = null;
		runs = audit = null;
		xpToday = null;
		loaded = false;
		void load(id, microbot);
		const t = setInterval(() => {
			if (!document.hidden) void load(id, microbot);
		}, 60_000);
		return () => clearInterval(t);
	});

	const since = $derived(windowStart(now));
	const items = $derived(
		feedItems({ runs: runs ?? [], levels: levels?.events ?? [], audit: audit ?? [] }, since, now)
	);
	const n = $derived(counts(items));
	const days = $derived(feedDays(items, filter, now, xpToday));
	const gains = $derived(levels ? weekGains(levels.skills) : null);
	const week = $derived(runs ? weekRuns(runs, since) : null);
	const withMicrobot = $derived(runs !== null);
	const pills = $derived(
		(
			[
				['all', 'Tudo', null],
				['runs', 'Execuções', n.runs],
				['levels', 'Níveis', n.levels],
				['mine', 'Suas ações', n.mine],
				['problems', 'Problemas', n.problems]
			] as [FeedFilter, string, number | null][]
		).filter(([f]) => withMicrobot || f === 'all' || f === 'levels')
	);
</script>

<svelte:head><title>Histórico · {me.state?.account.rsn ?? 'Conta'} · Chronicle</title></svelte:head>

<div class="top">
	<div class="flt" role="group" aria-label="O que mostrar">
		{#each pills as [f, label, count] (f)}
			<button
				type="button"
				class:on={filter === f}
				aria-pressed={filter === f}
				onclick={() => (filter = f)}
				>{label}{#if count !== null}
					<span>{count}</span>{/if}</button
			>
		{/each}
	</div>
	<span class="mu win">últimos 7 dias</span>
</div>

{#if failure}<p class="band">{failure}</p>{/if}

<div class="cols">
	<section class="feed" aria-label="O que a conta fez">
		{#if !loaded}
			<p class="mu none" aria-busy="true">Lendo…</p>
		{/if}
		{#each days as d (d.key)}
			{@const all = opened[d.key] || d.items.length <= DAY_CAP}
			<div class="dayh">
				<h2>{d.label}</h2>
				{#if d.summary}<span>{d.summary}</span>{/if}
			</div>
			{#each all ? d.items : d.items.slice(0, DAY_CAP) as i (i.key)}
				{@const sprite = i.skill ? iconFor(`skill:${i.skill}`) : undefined}
				{@const trail = !!i.trail?.length}
				{#snippet body()}
					<span class="t">{i.time}</span>
					{#if sprite}<img src={sprite} alt="" />{:else}<span class="ic"
							><Glyph name={i.glyph ?? 'conta'} size={18} /></span
						>{/if}
					<span class="x"
						><b>{i.title}</b>
						{#if i.sub || i.live}<span>· {[i.sub, i.live].filter(Boolean).join(' · ')}</span
							>{/if}</span
					>
					<span class="r"
						>{i.span ?? ''}{#if i.result && i.result.tone !== 'going'}<span
								class="res {i.result.tone}">{i.result.label}</span
							>{/if}</span
					>
				{/snippet}
				{#if trail}
					<button
						type="button"
						class="ev"
						class:open={open === i.key}
						aria-expanded={open === i.key}
						onclick={() => (open = open === i.key ? null : i.key)}>{@render body()}</button
					>
					{#if open === i.key}
						<div class="evd">
							{#each i.trail ?? [] as l, k (k)}
								<div><span class="t">{l.time}</span><span>{l.text}</span></div>
							{/each}
						</div>
					{/if}
				{:else}
					<div class="ev">{@render body()}</div>
				{/if}
			{/each}
			{#if !all}
				<button
					type="button"
					class="lnk more"
					onclick={() => (opened = { ...opened, [d.key]: true })}
					>mais {d.items.length - DAY_CAP} de {d.label.toLowerCase()}</button
				>
			{/if}
		{:else}
			{#if loaded}
				<p class="mu none">
					{filter === 'all'
						? 'Nada nos últimos 7 dias.'
						: 'Nada com este filtro nos últimos 7 dias.'}
				</p>
			{/if}
		{/each}
	</section>

	<aside class="rail">
		{#if gains}
			<section class="card">
				<div class="h">
					<h2>Na semana</h2>
					<span>7 dias</span>
				</div>
				<div class="b tight">
					{#each gains as g (g.skill)}
						{@const sprite = iconFor(`skill:${g.skill}`)}
						<div class="gain">
							{#if sprite}<img src={sprite} alt="" />{:else}<span></span>{/if}
							<span>{g.name}</span>
							<span class="u" class:up={g.up}>{g.levels}</span>
							<span class="xp7">{g.xp}</span>
						</div>
					{:else}
						<p class="mu">Nenhum XP nos últimos 7 dias.</p>
					{/each}
				</div>
			</section>
		{/if}
		{#if week}
			<section class="card">
				<div class="h">
					<h2>Execuções</h2>
					<span title={week.partial ? 'O servidor guarda as 30 mais novas de cada conta' : ''}
						>{week.partial ? 'últimas 30' : '7 dias'}</span
					>
				</div>
				<div class="b">
					<div class="stats">
						<div><span class="k">Feitas</span><span class="v">{week.done}</span></div>
						<div>
							<span class="k">Falharam</span><span class="v" class:bad={week.failed > 0}
								>{week.failed}</span
							>
						</div>
						{#if week.canceled}
							<div><span class="k">Canceladas</span><span class="v">{week.canceled}</span></div>
						{/if}
					</div>
					{#if week.lastFailed}
						<p class="mu fail">
							A que falhou: {week.lastFailed.name}{week.lastFailed.error
								? `, ${week.lastFailed.error}`
								: ''}.
						</p>
					{/if}
				</div>
			</section>
		{/if}
	</aside>
</div>

<p class="foot">{ICON_ATTRIBUTION}.</p>

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		gap: 10px 16px;
		align-items: center;
	}
	.win {
		margin-left: auto;
		font-size: 12px;
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
	.flt button span {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		opacity: 0.8;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
	}
	.feed {
		flex: 999 1 600px;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.none {
		margin: 0;
		font-size: 13px;
	}
	.dayh {
		display: flex;
		align-items: baseline;
		gap: 10px;
		padding: 18px 2px 8px;
		border-bottom: 2px solid var(--frame);
	}
	.dayh:first-child {
		padding-top: 0;
	}
	.dayh h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.dayh span {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.ev {
		display: grid;
		grid-template-columns: 48px 22px minmax(0, 1fr) auto;
		gap: 12px;
		align-items: center;
		min-height: 44px;
		padding: 4px 6px;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
		color: var(--ink);
		text-align: left;
		width: 100%;
		box-sizing: border-box;
	}
	button.ev {
		cursor: pointer;
	}
	button.ev:hover {
		background: var(--surface);
	}
	.ev .t,
	.evd .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.ev .t {
		text-align: right;
	}
	.ev img {
		width: 20px;
		height: 20px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.ev .ic {
		width: 20px;
		height: 20px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-subtle);
	}
	.ev .x {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.ev .x b {
		font-weight: 600;
	}
	.ev .x span {
		color: var(--ink-muted);
		font-size: 12px;
	}
	.ev .r {
		display: flex;
		align-items: center;
		gap: 10px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.res {
		font:
			600 11px 'JetBrains Mono',
			monospace;
		text-align: right;
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
	.ev.open {
		background: var(--surface);
		border: 1px solid var(--border);
		border-bottom: 0;
		border-radius: 6px 6px 0 0;
		box-shadow: var(--bevel);
	}
	.evd {
		display: flex;
		flex-direction: column;
		padding: 4px 16px 12px 94px;
		background: var(--surface);
		border: 1px solid var(--border);
		border-top: 0;
		border-radius: 0 0 6px 6px;
		font-size: 12px;
	}
	.evd div {
		display: grid;
		grid-template-columns: 44px minmax(0, 1fr);
		gap: 10px;
		min-height: 26px;
		align-items: center;
		border-top: 1px solid var(--hairline);
	}
	.more {
		min-height: 40px;
		width: max-content;
		border: 0;
		background: transparent;
		cursor: pointer;
		font-family: inherit;
	}
	.rail {
		flex: 1 1 280px;
		max-width: 320px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
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
		gap: 8px;
		padding: 12px 16px 16px;
		font-size: 13px;
	}
	.card > .b.tight {
		gap: 0;
	}
	.card p {
		margin: 0;
	}
	.gain {
		display: grid;
		grid-template-columns: 20px minmax(0, 1fr) auto auto;
		gap: 10px;
		align-items: center;
		min-height: 32px;
		border-bottom: 1px solid var(--hairline);
	}
	.gain:last-child {
		border-bottom: 0;
	}
	.gain img {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.gain .u {
		font:
			600 12px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.gain .u.up {
		color: var(--done);
	}
	.gain .xp7 {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 64px;
		text-align: right;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 10px;
	}
	.stats div {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.stats .k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.stats .v {
		font:
			700 18px/1.1 Alegreya,
			Georgia,
			serif;
		font-variant-numeric: lining-nums;
	}
	.stats .v.bad {
		color: var(--not-started);
	}
	.fail {
		font-size: 12px;
	}
	@media (max-width: 900px) {
		.ev {
			grid-template-columns: 44px 22px minmax(0, 1fr);
		}
		.ev .r {
			grid-column: 3;
		}
		.evd {
			padding-left: 16px;
		}
		.rail {
			max-width: none;
		}
	}
</style>
