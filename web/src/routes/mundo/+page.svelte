<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { session } from '#lib/progress/session.svelte';
	import {
		entityVerdict,
		KIND_LABEL,
		mainKey,
		routeLabel,
		routesLine,
		sectionLabel,
		stateLabel
	} from '#lib/progress/view';
	import type { Goal, WorldEntity } from '#lib/progress/types';
	import Glyph from '#lib/v4/Glyph.svelte';
	import GoalPlanPanel from '#lib/v4/GoalPlanPanel.svelte';

	/*
	 * Mundo (V4Mundo): entidades e objetivos compartilhados entre as rotas, as metas fixadas e,
	 * por entidade, onde cada rota a faz. "Ver plano" abre o plano da meta (V4Meta) em ?meta=;
	 * o grafo por foco fica para o item do grafo.
	 */
	const account = $derived(session.account);
	const world = createResource(() =>
		account ? progressApi.world(account) : Promise.resolve({ entities: [] as WorldEntity[] })
	);
	const goals = createResource(() =>
		account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })
	);
	const routes = $derived(session.summary?.routes ?? []);
	const active = $derived(routes.find((r) => r.active));
	const label = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);

	const KINDS: [string, string][] = [
		['', 'Tudo'],
		['quest', 'Quest'],
		['item', 'Item'],
		['skill', 'Level'],
		['diary', 'Diary'],
		['unlock', 'Unlock']
	];
	let kind = $state('');
	let query = $state('');
	const all = $derived(world.data?.entities ?? []);
	const touched = $derived(
		active?.cursor_section
			? all.filter((e) =>
					e.routes.some((r) => r.guide === active.guide && r.section === active.cursor_section)
				)
			: all.slice(0, 20)
	);
	const searching = $derived(query.trim() !== '' || kind !== '');
	const list = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const pool = searching ? all : touched;
		return pool
			.filter((e) => !kind || e.kind === kind)
			.filter((e) => !q || e.name.toLowerCase().includes(q) || e.ref.includes(q))
			.slice(0, 60);
	});

	const selectedRef = $derived(page.url.searchParams.get('e') ?? list[0]?.ref ?? null);
	const selected = $derived(all.find((e) => e.ref === selectedRef) ?? null);
	const pick = (ref: string) => {
		const url = new URL(page.url.href);
		url.searchParams.set('e', ref);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replaceState: true });
	};

	const planId = $derived(page.url.searchParams.get('meta'));
	const showPlan = (id: string | null) => {
		const url = new URL(page.url.href);
		if (id) url.searchParams.set('meta', id);
		else url.searchParams.delete('meta');
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replaceState: true });
	};

	const goalOf = (key: string | undefined) =>
		key ? goals.data?.goals.find((g) => g.objective_key === key) : undefined;
	let busy = $state(false);
	let failure = $state<string | null>(null);
	async function pin(key: string) {
		if (!account) return;
		busy = true;
		failure = null;
		try {
			await progressApi.addGoal(account, key);
			goals.reload();
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
	async function unpin(id: string) {
		if (!account) return;
		busy = true;
		failure = null;
		try {
			await progressApi.removeGoal(account, id);
			goals.reload();
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	const V_CLS = { met: 'v-met', prog: 'v-prog', unmet: 'v-unmet', unknown: 'v-unk' } as const;
	const ROLE: Record<string, string> = {
		completes: 'conclui',
		starts: 'inicia',
		requires: 'requer',
		uses: 'usa'
	};
	const VERDICT_TEXT: Record<string, string> = {
		met: 'feito',
		unmet: 'não atende',
		unknown: 'não sabemos'
	};
</script>

<svelte:head>
	<title>Mundo · Ironman Planner</title>
</svelte:head>

<header class="top">
	<div class="ttl">
		<p class="ey">Mundo · compartilhado entre as rotas</p>
		<h1 class="sr">Entidades e objetivos</h1>
	</div>
	<div class="seg" role="tablist">
		<span class="on" role="tab" aria-selected="true">Entidades</span>
		<span role="tab" aria-selected="false" aria-disabled="true" title="O grafo chega no item 11"
			>Grafo</span
		>
		<a href="#metas" role="tab" aria-selected="false">Metas</a>
	</div>
	<label class="inp"
		><Glyph name="search" size={14} /><input
			type="search"
			bind:value={query}
			placeholder="quest, item, level, diary, unlock…"
			aria-label="Buscar entidades"
		/></label
	>
	<div class="kinds">
		{#each KINDS as [k, l] (k)}
			<button class="chip" class:chip-on={kind === k} type="button" onclick={() => (kind = k)}
				>{l}</button
			>
		{/each}
	</div>
</header>

{#if !account}
	<div class="band">
		<Glyph name="alert" /><span
			>Sem conta: abra com <code>?account=acc_&lt;rsn&gt;</code> para ver vereditos e metas.</span
		>
	</div>
{/if}
{#if failure}
	<div class="band"><Glyph name="alert" /><span>A meta não mudou: {failure}</span></div>
{/if}

<section class="goals" id="metas">
	<div class="gh">
		<h2 class="ey">Metas fixadas · {goals.data?.goals.length ?? 0}</h2>
		<span class="su small"
			>uma meta é um objetivo fixado; o plano é o fechamento dos requisitos</span
		>
	</div>
	<div class="ggrid">
		{#each goals.data?.goals ?? [] as g (g.id)}
			{@const icon = iconFor(g.ref)}
			<div class="goal">
				<div class="gt">
					<span class="badge badge-gold">meta</span>
					<button class="gname sr" type="button" onclick={() => pick(g.ref)}
						>{#if icon}<img class="ski" src={icon} alt="" />{/if}{g.label}</button
					>
					<span class="badge" class:badge-done={g.status === 'achieved'}
						>{g.status === 'achieved' ? 'alcançada' : VERDICT_TEXT[g.verdict]}</span
					>
				</div>
				<span class="su small"
					>{KIND_LABEL[g.kind] ?? g.kind} · {stateLabel(g.objective_key)}{g.routes.length
						? ` · ${routesLine(g.routes, label)}`
						: ' · nenhuma rota conclui'}</span
				>
				<div class="gacts">
					<button
						class="btn btn-s"
						class:btn-p={planId !== g.id}
						type="button"
						aria-pressed={planId === g.id}
						onclick={() => showPlan(planId === g.id ? null : g.id)}
						>{planId === g.id ? 'Fechar plano' : 'Ver plano'}</button
					>
					<button class="btn btn-g btn-s" type="button" disabled={busy} onclick={() => unpin(g.id)}
						>Desafixar</button
					>
				</div>
			</div>
		{:else}
			<span class="su small"
				>Nenhuma meta ainda. Escolha uma entidade abaixo e use "Virar meta".</span
			>
		{/each}
	</div>
	{#if account && planId && goals.data?.goals.some((g) => g.id === planId)}
		{#key planId}
			<GoalPlanPanel
				{account}
				goalId={planId}
				activeGuide={active?.guide}
				routeName={label}
				onunpin={() => {
					const id = planId;
					showPlan(null);
					void unpin(id);
				}}
			/>
		{/key}
	{/if}
</section>

<div class="split">
	<section class="panel listp">
		<div class="plaque">
			{searching ? 'Resultados' : 'Tocadas pelos passos atuais'}<span class="r"
				>{list.length}{active && !searching
					? ` · ${label(active.guide)} ${active.cursor_section ? sectionLabel(active.cursor_section) : ''}`
					: ''}</span
			>
		</div>
		<div class="ents">
			{#each list as e (e.ref)}
				{@const v = entityVerdict(e.objectives)}
				{@const icon = iconFor(e.ref)}
				{@const guides = new Set(e.routes.map((r) => r.guide))}
				<button
					class="ent"
					class:ent-on={e.ref === selectedRef}
					type="button"
					onclick={() => pick(e.ref)}
				>
					<span class="v {V_CLS[v]}">{v === 'unknown' ? '?' : ''}</span>
					<span class="k">{KIND_LABEL[e.kind] ?? e.kind}</span>
					<span class="nm"
						>{#if icon}<img class="ski" src={icon} alt="" />{/if}{e.name}</span
					>
					<span class="w">{guides.size} {guides.size === 1 ? 'rota' : 'rotas'}</span>
				</button>
			{:else}
				<span class="su small pad">{world.loading ? 'Carregando…' : 'Nenhuma entidade.'}</span>
			{/each}
		</div>
	</section>

	{#if selected}
		{@const v = entityVerdict(selected.objectives)}
		{@const main = mainKey(selected.objectives.map((o) => o.key))}
		{@const goal = goalOf(main)}
		<section class="panel detail">
			<div class="plaque">
				<span class="v {V_CLS[v]} small-v"></span>{selected.name}<span class="r"
					>{KIND_LABEL[selected.kind] ?? selected.kind} · {selected.ref}</span
				>
			</div>
			<div class="body">
				<div class="row1">
					<div class="objs">
						<p class="ey">Objetivos desta entidade</p>
						<div class="chips">
							{#each selected.objectives as o (o.key)}
								<span class="obj" class:obj-unk={o.verdict === 'unknown'}>
									<span
										class="v {o.verdict === 'met'
											? 'v-met'
											: o.verdict === 'unmet'
												? 'v-unmet'
												: 'v-unk'}"
										>{#if o.verdict === 'met'}<Glyph
												name="check"
												size={9}
											/>{:else if o.verdict === 'unknown'}?{/if}</span
									>{stateLabel(o.key)}<span class="k">{o.key}</span>
								</span>
							{/each}
						</div>
						<span class="su tiny"
							>Veredito com evidência quando a fonte existe; quests e diaries ficam "não sabemos"
							até o plugin 1.1.</span
						>
					</div>
					<div class="acts">
						{#if goal}
							<button class="btn btn-s" type="button" disabled={busy} onclick={() => unpin(goal.id)}
								>Desafixar meta</button
							>
						{:else if main}
							<button
								class="btn btn-p btn-s"
								type="button"
								disabled={busy || !account}
								onclick={() => pin(main)}>Virar meta</button
							>
						{/if}
						{#if selected.wiki}
							<!-- eslint-disable svelte/no-navigation-without-resolve -- link externo da wiki -->
							<a class="btn btn-g btn-s" href={selected.wiki} target="_blank" rel="noreferrer"
								>wiki</a
							>
							<!-- eslint-enable svelte/no-navigation-without-resolve -->{/if}
					</div>
				</div>
				<div class="rels">
					<p class="ey">Onde cada rota faz isso</p>
					{#each selected.routes as r, i (i)}
						{@const cur = routes.find((x) => x.guide === r.guide)?.cursor?.step_id === r.step}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + âncora do passo -->
						<a
							class="rel"
							href={`${resolve('/rotas/[guide]/[section]', { guide: r.guide, section: r.section })}#passo-${r.step}`}
						>
							<span class="badge" class:badge-gold={cur}>{label(r.guide)}</span><span class="id"
								>{r.ref}</span
							><span class="tx">{r.title}</span><span class="w"
								>{sectionLabel(r.section)}{r.position ? ` · pos. ${r.position} de ${r.total}` : ''} ·
								{ROLE[r.role] ?? r.role}{cur ? ' · cursor' : ''}</span
							>
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{/each}
					<span class="su tiny"
						>O progresso é um só: concluir aqui marca o objetivo para as duas rotas. A cascata
						"presumido" só roda na rota ativa.</span
					>
				</div>
				<div class="graph">
					<div class="gh">
						<p class="ey">Grafo por foco · profundidade 1</p>
						<span class="badge badge-stale">requisitos de quest no catálogo · pendente</span>
					</div>
					<div class="lanes">
						<div class="lane">
							<div class="lh"><span class="ey">Requer</span></div>
							<span class="su small">Chega com os requisitos de quest no catálogo.</span>
						</div>
						<div class="lane">
							<div class="lh"><span class="ey gold">Foco</span></div>
							<div class="node node-focus">
								<span class="nt"
									><span class="v {V_CLS[v]} small-v"></span><span class="nm">{selected.name}</span
									></span
								>
								<span class="why"
									>{KIND_LABEL[selected.kind] ?? selected.kind} · {new Set(
										selected.routes.map((r) => r.guide)
									).size} rotas</span
								>
							</div>
						</div>
						<div class="lane">
							<div class="lh"><span class="ey">Destrava</span></div>
							<span class="su small">Nada no grafo ainda.</span>
						</div>
					</div>
				</div>
			</div>
		</section>
	{/if}
</div>

<p class="su tiny foot">
	Uma entidade é a página. Nunca abrimos o grafo inteiro: só o foco. Ids canônicos do registry
	(content/reference); nomes são só exibição. {ICON_ATTRIBUTION}.
</p>

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: 10px;
	}
	.ttl h1 {
		font-size: 24px;
	}
	.seg {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
	}
	.seg > * {
		display: inline-flex;
		align-items: center;
		height: 26px;
		padding: 0 12px;
		border-radius: 3px;
		color: var(--ink-muted);
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		text-decoration: none;
	}
	.seg .on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.seg [aria-disabled='true'] {
		opacity: 0.55;
	}
	.kinds {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}
	.goals {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.gh {
		display: flex;
		align-items: baseline;
		gap: 12px;
		flex-wrap: wrap;
	}
	.ggrid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
		gap: 12px;
	}
	.goal {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 10px 12px;
		border-radius: 4px;
		background: var(--surface);
		box-shadow: var(--bevel);
		border: 1px solid var(--border);
		min-width: 0;
	}
	.gt {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.gname {
		font-size: 16px;
		background: transparent;
		border: 0;
		padding: 0;
		color: var(--ink);
		cursor: pointer;
		text-align: left;
		flex: 1;
		min-width: 0;
	}
	.gacts {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.split {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.listp {
		flex: 1 1 320px;
		max-width: 420px;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.ents {
		padding: 8px;
		display: flex;
		flex-direction: column;
		max-height: 640px;
		overflow: auto;
	}
	.ent {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 40px;
		padding: 0 12px;
		border-radius: 4px;
		color: var(--ink);
		background: transparent;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		cursor: pointer;
		text-align: left;
		font: inherit;
	}
	.ent:hover {
		background: var(--elevated);
	}
	.ent-on {
		background: var(--elevated);
		box-shadow: var(--bevel);
	}
	.ent .k {
		font:
			500 9px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		width: 52px;
		flex: 0 0 auto;
	}
	.ent .nm {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 13px;
	}
	.ent .w,
	.rel .w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.v {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		flex: 0 0 auto;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		border: 1.5px solid var(--border);
		font:
			600 9px/1 Inter,
			sans-serif;
		color: var(--ground);
	}
	.v.v-unk {
		color: var(--ink-subtle);
	}
	.small-v {
		width: 12px;
		height: 12px;
	}
	.detail {
		flex: 999 1 520px;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.body {
		padding: 16px 18px 18px;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.row1 {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		align-items: flex-start;
	}
	.objs {
		flex: 1 1 260px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.chips,
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.rels {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.rel {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 32px;
		font-size: 13px;
		color: var(--ink);
		text-decoration: none;
		border-radius: 4px;
		padding: 0 4px;
	}
	.rel:hover {
		background: var(--elevated);
	}
	.rel .id {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 56px;
		flex: 0 0 auto;
	}
	.rel .tx {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rel .w {
		margin-left: auto;
	}
	.graph {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.lanes {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
		align-items: start;
	}
	.lane {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
	.lh {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 0 4px 4px;
		border-bottom: 1px solid var(--hairline);
	}
	.node {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 10px;
		border-radius: 4px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		font-size: 12px;
	}
	.node-focus {
		border-color: var(--gold-ink);
		box-shadow:
			var(--bevel),
			0 0 0 1px var(--gold-ink);
	}
	.nt {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.nm {
		font-weight: 500;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.why {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.gold {
		color: var(--gold-ink);
	}
	.pad {
		padding: 8px;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	.foot {
		margin: 0;
	}
	code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}
</style>
