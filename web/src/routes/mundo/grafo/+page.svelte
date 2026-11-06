<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { SvelteSet } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import {
		buildGraphView,
		buildTree,
		type GraphOptions,
		type KindFilter,
		type NodeView
	} from '#lib/progress/grafo';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { session } from '#lib/progress/session.svelte';
	import { KIND_LABEL, routeLabel } from '#lib/progress/view';
	import type { Goal } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * Mundo › Grafo (V6Grafo1440 em colunas, V6Grafo640 em árvore): o grafo sempre entra por um
	 * foco (uma meta, ou qualquer entidade pelo "Focar aqui" do painel). Requisitos por
	 * profundidade à esquerda, o foco, e o que ele destrava à direita; o clique abre o painel de
	 * entidade de sempre. Requisitos da wiki (Bucket) no catálogo: hoje só quests têm.
	 */
	const account = $derived(session.account);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	const focus = $derived(page.url.searchParams.get('foco') ?? pinned[0]?.objective_key ?? null);
	let depth = $state(3);
	const graph = createResource(
		() => (account && focus ? progressApi.graph(account, focus, depth) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const g = $derived(graph.data && graph.data.focus === focus ? graph.data : null);

	let hideDone = $state(true);
	let criticalOnly = $state(false);
	let kind = $state<KindFilter>('tudo');
	let mode = $state<'colunas' | 'arvore' | null>(null);
	const shownMode = $derived(mode ?? (narrow.value ? 'arvore' : 'colunas'));
	const closed = new SvelteSet<string>();

	const routes = $derived(session.summary?.routes ?? []);
	const opts: GraphOptions = $derived({
		hideDone,
		criticalOnly,
		kind,
		routeName: (guide: string) =>
			routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide),
		cursorOf: (guide: string) => routes.find((r) => r.guide === guide)?.cursor?.step_id
	});
	const view = $derived(g ? buildGraphView(g, opts) : null);
	const tree = $derived(g ? buildTree(g, opts, closed) : []);
	const goalOf = $derived(pinned.find((x) => x.objective_key === focus));
	const focusKind = $derived(goalOf ? 'meta' : g ? (KIND_LABEL[g.kind] ?? g.kind) : '');
	const focusSmall = $derived(
		g?.requirements ? `${focusKind} · ${g.met} de ${g.total}` : focusKind
	);
	const plaqueRight = $derived(
		view && g
			? [
					`profundidade ${g.depth}`,
					`${view.nodes} nós`,
					view.collapsed ? `${view.collapsed} feitos colapsados` : ''
				]
					.filter(Boolean)
					.join(' · ')
			: ''
	);

	function setFocus(key: string) {
		const url = new URL(page.url.href);
		url.searchParams.set('foco', key);
		closed.clear();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replace: true, reset: false });
	}
	function toggle(id: string) {
		if (closed.has(id)) closed.delete(id);
		else closed.add(id);
	}
	const open = (n: NodeView) => n.ref && openEntity(n.ref);
	const sel = (n: NodeView) => (n.ref && entityPanel.ref === n.ref ? 'on' : '');
	const KINDS: KindFilter[] = ['tudo', 'quest', 'level', 'item', 'unlock'];
</script>

<svelte:head>
	<title>Grafo · Mundo · Chronicle</title>
</svelte:head>

{#snippet badge(n: NodeView)}
	<span class="v {n.vcls}"
		>{#if n.vcls === 'v-met'}<Glyph
				name="check"
				size={9}
			/>{:else if n.vcls === 'v-unk'}?{/if}</span
	>
{/snippet}

{#snippet nodeBtn(n: NodeView)}
	<button
		type="button"
		class="node {sel(n)}"
		class:crit={n.critical}
		title={n.keys ? n.keys.join(' · ') : n.key}
		aria-haspopup={n.ref ? 'dialog' : undefined}
		disabled={!n.ref}
		onclick={() => open(n)}
		>{@render badge(n)}<span
			>{n.label}{#if n.sub}<span class="w">{n.sub}</span>{/if}</span
		></button
	>
{/snippet}

<MundoHead title="Grafo por foco" current="grafo" />

{#if !account}
	<p class="su small">Escolha uma conta para ver o grafo.</p>
{:else if !focus}
	<section class="panel pb">
		<div class="plaque">Grafo por foco</div>
		<p class="su small pad">
			{goals.loading
				? 'Carregando…'
				: 'O grafo sempre entra por um foco. Fixe uma meta em Metas, ou abra uma entidade e use “Focar aqui”.'}
			{#if !goals.loading}<a href={resolve('/mundo/metas')}>Metas</a> ·
				<a href={resolve('/mundo')}>Entidades</a>{/if}
		</p>
	</section>
{:else}
	<section class="panel pb">
		<div class="plaque">
			{shownMode === 'arvore'
				? 'Árvore · requer'
				: `Grafo por foco · ${view?.focus.label ?? '…'}`}<span
				class="r"
				title="Requisitos da wiki (Bucket); nós de nível são um por skill">{plaqueRight}</span
			>
		</div>
		<div class="ctl">
			<label
				class="sel foc"
				title="O grafo sempre entra por um foco: uma meta, ou uma entidade pelo “Focar aqui” do painel"
				>Foco · {view?.focus.label ?? focus}
				<small>{focusSmall}</small><Glyph name="chevron" size={12} /><select
					aria-label="Foco do grafo"
					value={focus}
					onchange={(ev) => setFocus(ev.currentTarget.value)}
				>
					{#if !goalOf}<option value={focus}>{g?.label ?? focus}</option>{/if}
					{#each pinned as p (p.id)}<option value={p.objective_key}>{p.label} · meta</option>{/each}
				</select></label
			>
			<span class="lbl">profundidade</span>
			<div class="seg" role="group" aria-label="Profundidade">
				{#each [1, 2, 3] as d (d)}<button
						type="button"
						class:on={depth === d}
						aria-pressed={depth === d}
						onclick={() => (depth = d)}>{d}</button
					>{/each}
			</div>
			<label class="chk"><input type="checkbox" bind:checked={hideDone} />esconder feitos</label>
			{#if !narrow.value}
				<label class="chk"
					><input type="checkbox" bind:checked={criticalOnly} />só caminho crítico</label
				>
				<div class="seg end" role="group" aria-label="Tipo">
					{#each KINDS as k (k)}<button
							type="button"
							class:on={kind === k}
							aria-pressed={kind === k}
							onclick={() => (kind = k)}>{k}</button
						>{/each}
				</div>
			{:else}
				<div class="seg end" role="group" aria-label="Disposição">
					<button
						type="button"
						class:on={shownMode === 'colunas'}
						aria-pressed={shownMode === 'colunas'}
						onclick={() => (mode = 'colunas')}>colunas</button
					><button
						type="button"
						class:on={shownMode === 'arvore'}
						aria-pressed={shownMode === 'arvore'}
						onclick={() => (mode = 'arvore')}>árvore</button
					>
				</div>
			{/if}
		</div>

		{#if graph.error && !g}
			<p class="su small pad" role="alert">Não deu para montar o grafo: {graph.error.message}</p>
		{:else if !view || !g}
			<p class="su small pad" aria-busy="true">Carregando…</p>
		{:else if shownMode === 'colunas'}
			<div class="scroll">
				<div
					class="gcols"
					style="grid-template-columns: repeat({view.columns.length + 2}, minmax(180px, 1fr));"
				>
					{#each view.columns as c (c.depth)}
						<div class="gcol">
							<div class="hd">
								{c.title}<span class="n"
									>{c.done.count ? `${c.shown} · +${c.done.count} feitos` : c.shown}</span
								>
							</div>
							{#each c.nodes as n (n.key)}{@render nodeBtn(n)}{/each}
							{#if c.done.count}
								<div class="node done">
									<span class="v v-met"><Glyph name="check" size={9} /></span><span
										>+{c.done.count} feitos<span class="w">{c.done.names}</span></span
									>
								</div>
							{/if}
							{#if !c.nodes.length && !c.done.count}<p class="su tiny pad">nada</p>{/if}
						</div>
					{/each}
					<div class="gcol gcol-focus">
						<div class="hd">Foco<span class="n">{focusKind}</span></div>
						{@render nodeBtn(view.focus)}
						{#if !g.requirements}<p class="su tiny pad">sem requisitos no catálogo</p>{/if}
					</div>
					<div class="gcol">
						<div class="hd">Destrava<span class="n">{g.unlocks_total}</span></div>
						{#each view.unlocks as n (n.key)}{@render nodeBtn(n)}{/each}
						{#if g.unlocks_total > view.unlocks.length}<p class="su tiny pad">
								+{g.unlocks_total - view.unlocks.length} no catálogo
							</p>{/if}
						{#if !g.unlocks_total}<p class="su tiny pad">nada no catálogo</p>{/if}
					</div>
				</div>
			</div>
		{:else}
			<div>
				{#each tree as row (row.id)}
					<div class="tr" class:done={row.done} class:on={!!sel(row.node)}>
						<span class="ind"
							>{#each Array.from({ length: row.depth }, (_, i) => i) as i (i)}<i></i>{/each}</span
						>{#if row.parent}<button
								type="button"
								class="cvb"
								aria-expanded={!closed.has(row.id)}
								aria-label={closed.has(row.id) ? 'Abrir requisitos' : 'Recolher requisitos'}
								onclick={() => toggle(row.id)}
								><span class="cv rot" class:shut={closed.has(row.id)}
									><Glyph name="chevron" size={12} /></span
								></button
							>{:else}<span class="cv"></span>{/if}{@render badge(row.node)}{#if row.done}<span
								class="nm">{row.node.label}</span
							>{:else}<button
								type="button"
								class="nm nmb"
								title={row.node.keys ? row.node.keys.join(' · ') : row.node.key}
								aria-haspopup={row.node.ref ? 'dialog' : undefined}
								disabled={!row.node.ref}
								onclick={() => open(row.node)}>{row.node.label}</button
							>{/if}<span class="w">{row.node.sub}</span>
					</div>
				{:else}
					<p class="su small pad">
						{g.requirements ? 'Nada a mostrar com estes filtros.' : 'Sem requisitos no catálogo.'}
					</p>
				{/each}
			</div>
		{/if}

		<div class="glg">
			<span><span class="v v-met"><Glyph name="check" size={9} /></span>feito</span>
			<span><span class="v v-prog"></span>em progresso</span>
			<span><span class="v v-unmet"></span>disponível</span>
			<span><span class="v v-lock"></span>bloqueado</span>
			<span><span class="v v-unk">?</span>não sabemos</span>
			<span><span class="cbar"></span>caminho crítico</span>
			<span class="end"
				>{shownMode === 'colunas'
					? 'cada coluna à esquerda é requisito da coluna à direita · clique abre o painel'
					: 'cada recuo é requisito da linha de cima · clique abre o painel'}</span
			>
		</div>
	</section>
{/if}

<style>
	.ctl {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		padding: 12px 12px 0;
	}
	.foc {
		position: relative;
		max-width: 100%;
	}
	.foc select {
		position: absolute;
		inset: 0;
		width: 100%;
		opacity: 0;
		cursor: pointer;
		font-size: 16px;
	}
	.foc:focus-within {
		outline: 2px solid var(--gold);
		outline-offset: 2px;
	}
	.end {
		margin-left: auto;
	}
	.scroll {
		overflow-x: auto;
		padding: 12px;
	}
	.gcols {
		min-width: min-content;
	}
	.node:disabled {
		cursor: default;
	}
	.pad {
		padding: 8px 12px;
		margin: 0;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	.cvb {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 14px;
		height: 44px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--ink-subtle);
		cursor: pointer;
	}
	.rot {
		display: inline-flex;
		transform: rotate(90deg);
	}
	.rot.shut {
		transform: none;
	}
	.nmb {
		border: 0;
		background: transparent;
		padding: 0;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
		height: 44px;
	}
	.nmb:disabled {
		cursor: default;
	}
	.tr {
		cursor: default;
	}
	.glg {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 16px;
		margin: 0 0 12px;
		padding: 10px 12px 0;
		font-size: 11px;
		color: var(--ink-subtle);
		border-top: 1px solid var(--hairline);
	}
	.glg > span {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.glg .cbar {
		width: 3px;
		height: 14px;
		background: var(--gold-ink);
		display: inline-block;
	}
</style>
