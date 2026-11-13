<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { fleet } from '#lib/progress/fleet.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import {
		closenessLabel,
		fleetRows,
		goalLine,
		goalTitle,
		kindTag,
		needRows,
		needsAccount,
		plural,
		routeBlocks,
		type NeedMark
	} from '#lib/progress/mundo';
	import { routeLabel } from '#lib/progress/view';
	import type { FleetGoal, StepPlace } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * Mundo › Metas da frota (página 8 do canvas, V7Mundo1440; fatia V10): as metas que qualquer
	 * conta fixou, à esquerda; a meta aberta mostra quem da frota a persegue e quão perto está, o
	 * que falta para uma conta (o plano dela, GET /api/goals/{id}/plan) e onde cada rota chega.
	 * Divergências do artboard em docs/ui/decisoes-canvas.md, seção 7.7.
	 */
	const res = createResource(() => progressApi.fleetGoals(), { tags: ['progress'] });
	const accounts = $derived(res.data?.accounts ?? []);
	const goals = $derived(res.data?.goals ?? []);
	const goal = $derived(
		goals.find((g) => g.objective_key === page.url.searchParams.get('meta')) ?? goals[0] ?? null
	);
	const needs = $derived(goal ? needsAccount(goal, page.url.searchParams.get('conta')) : undefined);
	const needsRsn = $derived(accounts.find((a) => a.id === needs?.account)?.rsn ?? needs?.account);
	const rows = $derived(goal ? fleetRows(goal, accounts) : []);
	const routeName = (guide: string) => routeLabel(guide, guide);
	const blocks = $derived(goal ? routeBlocks(goal, accounts, routeName) : []);

	/* O plano da conta de "O que falta"; a chave em texto evita refazer a leitura a cada releitura da frota. */
	const planKey = $derived(needs?.goal ? `${needs.account}|${needs.goal.id}` : '');
	const planRes = createResource(
		() => {
			const [account, id] = planKey.split('|');
			return planKey ? progressApi.goalPlan(account, id) : Promise.resolve(null);
		},
		{ tags: ['progress'] }
	);
	const plan = $derived(
		planRes.data && planRes.data.goal.id === needs?.goal?.id ? planRes.data.plan : null
	);
	const where = (p: StepPlace) => `${routeName(p.guide_id)} ${p.ref}`;
	const needList = $derived(plan ? needRows(plan, where) : []);

	function setQuery(changes: Record<string, string | null>) {
		const url = new URL(page.url.href);
		for (const [k, v] of Object.entries(changes)) {
			if (v === null) url.searchParams.delete(k);
			else url.searchParams.set(k, v);
		}
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replace: true, reset: false });
	}
	const pick = (g: FleetGoal) => {
		armed = false;
		setQuery({ meta: g.objective_key, conta: null });
	};

	let busy = $state<string | null>(null);
	let armed = $state(false);
	let failure = $state<string | null>(null);
	const fail = (e: unknown) => (failure = e instanceof Error ? e.message : String(e));

	/** "fixar para ela": a conta passa a perseguir a meta (POST /api/goals da conta). */
	async function pinFor(account: string) {
		if (!goal || busy) return;
		busy = account;
		failure = null;
		try {
			await progressApi.addGoal(account, goal.objective_key);
			res.refresh();
		} catch (e) {
			fail(e);
		} finally {
			busy = null;
		}
	}

	/** Desafixar tira a meta de todas as contas que a perseguem; com mais de uma, pede confirmação. */
	async function unpin() {
		if (!goal || busy) return;
		const pins = goal.accounts.filter((s) => s.goal);
		if (pins.length > 1 && !armed) {
			armed = true;
			return;
		}
		armed = false;
		busy = 'unpin';
		failure = null;
		try {
			for (const s of pins) await progressApi.removeGoal(s.account, s.goal!.id);
			setQuery({ meta: null, conta: null });
			res.reload();
		} catch (e) {
			fail(e);
			res.refresh();
		} finally {
			busy = null;
		}
	}

	let query = $state('');
	function search(e: SubmitEvent) {
		e.preventDefault();
		const q = query.trim();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + a busca
		void goto(`${resolve('/mundo')}${q ? `?q=${encodeURIComponent(q)}` : ''}`);
	}

	const stepHref = (guide: string, section: string, step: string) =>
		`${resolve('/rotas/[guide]/[section]', { guide, section })}#passo-${step}`;
	const VD: Record<NeedMark, string> = { met: 'met', prog: 'prog', unmet: 'unmet', unk: 'unk' };
</script>

<svelte:head>
	<title>Metas · Mundo · Chronicle</title>
</svelte:head>

<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs internos vêm de resolve() + query ou âncora; o da wiki é externo -->

<MundoHead
	title="Mundo"
	current="metas"
	lead="Quests, itens, skills e desbloqueios, e quem da frota já tem cada um."
>
	<form class="field" role="search" onsubmit={search}>
		<Glyph name="search" size={14} />
		<input
			type="search"
			bind:value={query}
			placeholder="quest, item, skill, diary…"
			aria-label="Buscar entidade"
		/>
		<span class="kbd">⌘K</span>
	</form>
</MundoHead>

{#if res.error}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>As metas da frota não carregaram: {res.error.message}</span>
	</div>
{/if}
{#if failure}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>A meta não mudou: {failure}</span>
	</div>
{/if}

<div class="cols">
	<nav class="list" aria-label="Metas">
		<div class="grp7">
			<h2>Metas</h2>
			{#if goals.length}<span>{plural(goals.length, 'fixada', 'fixadas')}</span>{/if}
		</div>
		{#each goals as g (g.objective_key)}
			{@const icon = iconFor(g.ref)}
			<button
				class="gl7"
				class:on={g.objective_key === goal?.objective_key}
				type="button"
				aria-current={g.objective_key === goal?.objective_key ? 'true' : undefined}
				onclick={() => pick(g)}
			>
				{#if icon}<img src={icon} alt="" />{:else}<span class="gi"><Glyph name="mundo" /></span
					>{/if}
				<span class="x"><b>{goalTitle(g)}</b><span>{goalLine(g, accounts.length)}</span></span>
				<span class="tag">{kindTag(g.kind)}</span>
			</button>
		{:else}
			<p class="mu one">
				{res.loading ? 'Lendo as metas da frota…' : 'Nenhuma conta fixou uma meta.'}
			</p>
		{/each}
		<a class="add" href={resolve('/mundo')}
			><Glyph name="plus" size={14} /><span>Fixar outra meta</span></a
		>
	</nav>

	{#if goal}
		{@const icon = iconFor(goal.ref)}
		{@const pins = goal.accounts.filter((s) => s.goal).length}
		<div class="main">
			<header class="hero">
				<div class="stage">
					{#if icon}<img src={icon} alt="" />{:else}<Glyph name="mundo" size={28} />{/if}
				</div>
				<div class="hd">
					<h2 class="sr">{goalTitle(goal)}</h2>
					<span class="mu"
						>{kindTag(goal.kind)}{#if goal.wiki}
							· <a href={goal.wiki} target="_blank" rel="noreferrer">wiki</a>{/if}</span
					>
				</div>
				<div class="acts">
					<a
						class="btn"
						href={`${resolve('/mundo/grafo')}?focus=${encodeURIComponent(goal.objective_key)}${needs ? `&account=${encodeURIComponent(needs.account)}` : ''}`}
						>Ver no grafo</a
					>
					{#if pins}
						<button
							class="btn btn-g"
							class:btn-d={armed}
							type="button"
							disabled={!!busy}
							title={pins > 1 ? `Tira a meta das ${pins} contas que a perseguem` : undefined}
							onclick={unpin}>{armed ? `Tirar das ${pins} contas` : 'Desafixar'}</button
						>
					{/if}
				</div>
			</header>

			<section class="card">
				<div class="h">
					<h2>Na frota</h2>
					<span>quem persegue e quão perto está</span>
				</div>
				<div class="b rows">
					{#each rows as r (r.account)}
						{@const sel = r.pursues && r.account === needs?.account}
						<div class="who7r">
							<span class="n"
								><span class="lamp {fleet.tones[r.account] ?? 'unk'}"></span>{#if r.pursues}<button
										class="nm"
										class:sel
										type="button"
										aria-pressed={sel}
										title="Ver o que falta para {r.rsn}"
										onclick={() => setQuery({ conta: r.account })}>{r.rsn}</button
									>{:else}<b>{r.rsn}</b>{/if}</span
							>
							{#if r.measure}
								<div class="xp"><span style="width: {r.measure.pct}%"></span></div>
								<span class="pc">{closenessLabel(r.measure)}</span>
							{:else}
								<span></span><span></span>
							{/if}
							<span class="bl" class:ok={r.standing.verdict === 'met'}>
								{#if r.pursues || r.standing.verdict === 'met'}
									{r.line}
								{:else}
									não persegue · <button
										class="lnk inl"
										type="button"
										disabled={!!busy}
										onclick={() => pinFor(r.account)}>fixar para ela</button
									>
								{/if}
							</span>
						</div>
					{/each}
				</div>
			</section>

			<div class="two">
				{#if needs?.goal}
					<section class="card">
						<div class="h">
							<h2>O que falta</h2>
							<span>{needsRsn}</span>
						</div>
						<div class="b rows">
							{#if planRes.error}
								<p class="mu small">O plano não carregou: {planRes.error.message}</p>
							{:else if !plan}
								<p class="mu small" aria-busy="true">Lendo o plano…</p>
							{:else}
								{#each needList as n (n.key)}
									<div class="req7" class:sub={n.sub}>
										<span class="vd7 {VD[n.mark]}"
											>{#if n.mark === 'met'}<Glyph name="check" size={9} />{/if}</span
										>
										<span
											>{#if n.sub}{n.label}{:else}<b>{n.label}</b>{/if}</span
										>
										<span class="w">{n.hint}</span>
									</div>
								{:else}
									<p class="mu small">
										{plan.total ? 'Os requisitos estão feitos.' : 'Sem requisitos conhecidos.'}
									</p>
								{/each}
								{#if plan.total}
									<a
										class="lnk all"
										href={`${resolve('/mundo/metas/plano')}?account=${encodeURIComponent(needs.account)}&meta=${encodeURIComponent(needs.goal.id)}`}
										>{plan.total === 1 ? 'o requisito' : `os ${plan.total} requisitos`}</a
									>
								{/if}
							{/if}
						</div>
					</section>
				{/if}
				<section class="card">
					<div class="h">
						<h2>Onde cada rota chega</h2>
						<span>passos dos guias</span>
					</div>
					<div class="b routes">
						{#each blocks as b (b.guide)}
							<div class="rb">
								<p class="mono su rr">
									{routeName(b.guide).toUpperCase()} · {b.ref}{b.role ? ` · ${b.role}` : ''}
								</p>
								<p class="rt"><a href={stepHref(b.guide, b.section, b.step)}>{b.title}</a></p>
								{#each b.lines as l (l)}<p class="mu rl">{l}</p>{/each}
							</div>
						{:else}
							<p class="mu small">Nenhuma rota faz esta meta.</p>
						{/each}
					</div>
				</section>
			</div>
		</div>
	{/if}
</div>

<p class="foot">
	Requisitos de quest da OSRS Wiki. Passos das rotas: Guias por OzirisLoL / ironman.guide e por So
	Iron BRUH e ParasailerOSRS. {ICON_ATTRIBUTION}.
</p>

<style>
	.field {
		display: flex;
		align-items: center;
		gap: 10px;
		height: 36px;
		flex: 0 1 260px;
		padding: 0 12px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		box-sizing: border-box;
		color: var(--ink-subtle);
	}
	.field input {
		flex: 1;
		min-width: 0;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--ink);
		font:
			500 13px Inter,
			system-ui,
			sans-serif;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 28px;
		align-items: flex-start;
	}
	.list {
		flex: 1 1 280px;
		max-width: 340px;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	@media (max-width: 900px) {
		.list {
			max-width: none;
		}
	}
	.main {
		flex: 999 1 560px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.grp7 {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 0 2px 8px;
		border-bottom: 2px solid var(--frame);
	}
	.grp7 h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.grp7 span {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.gl7 {
		display: grid;
		grid-template-columns: 28px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		width: 100%;
		min-height: 52px;
		padding: 6px 10px;
		border-radius: 5px;
		border: 1px solid transparent;
		border-bottom-color: var(--hairline);
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-size: 13px;
		text-align: left;
		cursor: pointer;
	}
	.gl7:hover {
		border-color: var(--border);
	}
	.gl7.on {
		background: var(--gold-tint);
		border-color: var(--gold-ink);
	}
	.gl7 img {
		width: 26px;
		height: 26px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.gi {
		display: inline-flex;
		justify-content: center;
		color: var(--ink-subtle);
	}
	.x {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
	}
	.x b {
		font-weight: 600;
	}
	.x span {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.tag {
		display: inline-flex;
		align-items: center;
		height: 18px;
		padding: 0 6px;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		white-space: nowrap;
	}
	.one {
		margin: 0;
		padding: 12px 4px;
		font-size: 13px;
	}
	.add {
		display: flex;
		align-items: center;
		gap: 9px;
		min-height: 44px;
		margin-top: 10px;
		padding: 4px 8px;
		border-radius: 4px;
		border: 1px dashed var(--border);
		color: var(--ink-subtle);
		text-decoration: none;
		font-size: 13px;
	}
	.add:hover {
		color: var(--ink);
	}
	.hero {
		display: grid;
		grid-template-columns: 96px minmax(0, 1fr) auto;
		gap: 20px;
		align-items: center;
	}
	@media (max-width: 900px) {
		.hero {
			grid-template-columns: 76px minmax(0, 1fr);
		}
		.hero .stage {
			width: 76px;
			height: 76px;
		}
		.hero .acts {
			grid-column: 1 / -1;
		}
	}
	.stage {
		width: 96px;
		height: 96px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: inset 0 2px 6px rgba(0, 0, 0, 0.14);
		display: flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		color: var(--ink-subtle);
	}
	.stage img {
		width: 56px;
		height: 56px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.hd {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.hd h2 {
		font-size: 26px;
	}
	.hd .mu {
		font-size: 13px;
	}
	.acts {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
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
		gap: 12px;
		padding: 12px 16px 16px;
	}
	.card > .b.rows {
		gap: 0;
	}
	.card > .b.routes {
		gap: 10px;
	}
	.two {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
		gap: 20px;
		align-items: start;
	}
	.who7r {
		display: grid;
		grid-template-columns: 140px minmax(0, 1fr) 44px minmax(0, 1.2fr);
		gap: 12px;
		align-items: center;
		min-height: 44px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.who7r:last-child {
		border-bottom: 0;
	}
	@media (max-width: 640px) {
		.who7r {
			grid-template-columns: minmax(0, 1fr) 44px;
			padding: 6px 0;
		}
		.who7r .xp {
			display: none;
		}
		.who7r .bl {
			grid-column: 1 / -1;
		}
	}
	.who7r .n {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.who7r .n b,
	.who7r .nm {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nm {
		min-height: 32px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-weight: 600;
		text-align: left;
		cursor: pointer;
	}
	.nm:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.nm.sel {
		color: var(--gold-ink);
		text-decoration: underline;
		text-decoration-color: var(--gold-ink);
		text-underline-offset: 3px;
	}
	.pc {
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		text-align: right;
	}
	.bl {
		font-size: 12px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.bl.ok {
		color: var(--done);
	}
	.lnk.inl {
		min-height: 32px;
		padding: 0;
		border: 0;
		background: transparent;
		font: inherit;
		font-size: 12px;
		cursor: pointer;
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
	.req7 {
		display: grid;
		grid-template-columns: 18px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		min-height: 34px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.req7:last-of-type {
		border-bottom: 0;
	}
	.req7.sub {
		padding-left: 28px;
	}
	.req7 b {
		font-weight: 600;
	}
	.req7 .w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		text-align: right;
	}
	.vd7 {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		border: 1.5px solid var(--border);
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--ground);
	}
	.vd7.met {
		background: var(--done);
		border-color: var(--done);
	}
	.vd7.prog {
		background: linear-gradient(90deg, var(--gold-ink) 50%, transparent 50%);
		border-color: var(--gold-ink);
	}
	.vd7.unmet {
		border-color: var(--not-started);
	}
	.vd7.unk {
		border-style: dotted;
		border-color: var(--ink-subtle);
	}
	.lnk.all {
		min-height: 32px;
		padding: 0;
		align-self: flex-start;
	}
	.rb + .rb {
		border-top: 1px solid var(--hairline);
		padding-top: 10px;
	}
	.rb p {
		margin: 0;
	}
	.rr {
		font-size: 11px;
	}
	.rt {
		margin-top: 2px !important;
		font-size: 13px;
		line-height: 1.45;
	}
	.rt a {
		color: var(--ink);
		text-decoration: none;
	}
	.rt a:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.rl {
		margin-top: 2px !important;
		font-size: 12px;
	}
	.small {
		margin: 0;
		font-size: 12px;
	}
	.foot {
		font-size: 11px;
	}
</style>
