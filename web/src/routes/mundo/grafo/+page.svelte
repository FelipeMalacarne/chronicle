<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import {
		doneSummary,
		filterColumn,
		GROUP_GLYPH,
		GROUP_LABEL,
		KIND_FILTERS,
		nodeLine,
		nodeRef,
		treeRows,
		type KindFilter
	} from '#lib/progress/grafo';
	import { ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { session } from '#lib/progress/session.svelte';
	import { KIND_LABEL, routeLabel } from '#lib/progress/view';
	import type { Goal, GraphNode } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * Mundo › Grafo (V6Grafo1440 e V6Grafo640, F10): o grafo sempre entra por um foco (?focus=,
	 * senão a primeira meta fixada). Em 1440, colunas por profundidade (requer → foco → destrava);
	 * no estreito, a árvore com recuo. Cada nó abre o painel de entidade, que tem "Focar aqui".
	 */
	const account = $derived(session.account);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	const focus = $derived(page.url.searchParams.get('focus') ?? pinned[0]?.objective_key ?? null);
	const goalOfFocus = $derived(pinned.find((g) => g.objective_key === focus));

	let depth = $state(2);
	let hideDone = $state(true);
	let criticalOnly = $state(false);
	let kind = $state<KindFilter>('tudo');
	/* No estreito a árvore é o padrão; o botão colunas/árvore troca até a largura mudar. */
	let tree = $derived(narrow.value);
	const indents = (n: number) => Array.from({ length: n }, (_, j) => j);
	const filters = $derived({ hideDone, criticalOnly, kind });

	const graph = createResource(
		() => (account && focus ? progressApi.graph(account, focus, depth) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const g = $derived(graph.data && graph.data.focus.key === focus ? graph.data : null);

	const routes = $derived(session.summary?.routes ?? []);
	const routeName = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);
	const cursor = $derived(routes.find((r) => r.active)?.cursor?.step_id);
	const line = (n: GraphNode) => nodeLine(n, routeName, cursor);

	/* Colunas da esquerda para a direita: o requisito mais fundo primeiro, depois o foco e o que ele destrava. */
	const columns = $derived(g ? [...g.columns].reverse().map((c) => filterColumn(c, filters)) : []);
	const rows = $derived(g ? treeRows(g, filters) : []);
	const criticalHead = $derived(
		g?.critical.length
			? g.columns.flatMap((c) => c.nodes).find((n) => n.key === g.critical[0])?.label
			: undefined
	);
	const focusLine = $derived.by(() => {
		if (!g) return '';
		const parts = [goalOfFocus ? 'meta' : (KIND_LABEL[g.focus.kind] ?? g.focus.kind)];
		if (g.focus.missing?.length) parts.push(`faltam ${g.focus.missing.length}`);
		else parts.push(GROUP_LABEL[g.focus.group]);
		if (criticalHead) parts.push(`caminho crítico: ${criticalHead}`);
		return parts.join(' · ');
	});

	function refocus(key: string) {
		const url = new URL(page.url.href);
		url.searchParams.set('focus', key);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replace: false, reset: false });
	}
	const open = (n: GraphNode) => {
		const ref = nodeRef(n);
		if (ref) openEntity(ref);
	};
	const isOn = (n: GraphNode) => !!entityPanel.ref && entityPanel.ref === nodeRef(n);
	let picking = $state(false);
</script>

<svelte:head>
	<title>Grafo · Mundo · Chronicle</title>
</svelte:head>

{#snippet glyph(n: GraphNode)}
	<span class="v {GROUP_GLYPH[n.group]}"
		>{#if n.group === 'done'}<Glyph
				name="check"
				size={9}
			/>{:else if n.group === 'unknown'}?{/if}</span
	>
{/snippet}

{#snippet card(n: GraphNode, sub: string)}
	<button
		type="button"
		class="node"
		class:crit={n.critical}
		class:on={isOn(n)}
		class:done={n.group === 'done'}
		title={n.key}
		onclick={() => open(n)}
		>{@render glyph(n)}<span>{n.label}<span class="w">{sub}</span></span></button
	>
{/snippet}

<MundoHead title="Grafo por foco" current="grafo" />

<section class="panel pb">
	<div class="plaque">
		Grafo por foco{#if g}&nbsp;· {g.focus.label}{/if}
		{#if g}<span class="r" title="Requisitos da wiki; os níveis de cada quest num cartão só"
				>profundidade {g.depth} · {g.nodes} nós · {g.done_nodes} feitos recolhidos</span
			>{/if}
	</div>
	<div class="tools">
		<div class="pick">
			<button
				class="sel"
				type="button"
				aria-expanded={picking}
				title="O grafo sempre entra por um foco: uma meta, ou “Focar aqui” no painel de qualquer entidade"
				onclick={() => (picking = !picking)}
				>Foco · {g?.focus.label ?? goalOfFocus?.label ?? '—'}
				<small>{goalOfFocus ? 'meta' : g ? (KIND_LABEL[g.focus.kind] ?? g.focus.kind) : ''}</small
				></button
			>
			{#if picking}
				<div class="menu panel" role="menu">
					{#each pinned as m (m.id)}
						<button
							class="ln"
							class:ln-on={m.objective_key === focus}
							type="button"
							role="menuitem"
							onclick={() => {
								picking = false;
								refocus(m.objective_key);
							}}><span></span><span class="nm">{m.label}</span><span class="w">meta</span></button
						>
					{:else}
						<p class="su small pad">Nenhuma meta fixada.</p>
					{/each}
					<a class="ln add" href={resolve('/mundo')}
						><span></span><span class="nm">Outra entidade: “Focar aqui” no painel</span><span
							class="w">Entidades</span
						></a
					>
				</div>
			{/if}
		</div>
		<span class="lbl">profundidade</span>
		<div class="seg" role="group" aria-label="Profundidade">
			{#each [1, 2, 3] as d (d)}
				<button
					type="button"
					class:on={depth === d}
					aria-pressed={depth === d}
					onclick={() => (depth = d)}>{d}</button
				>
			{/each}
		</div>
		<label class="chk"><input type="checkbox" bind:checked={hideDone} />esconder feitos</label>
		{#if !tree}
			<label class="chk"
				><input type="checkbox" bind:checked={criticalOnly} />só caminho crítico</label
			>
		{/if}
		{#if narrow.value}
			<div class="seg" role="group" aria-label="Vista">
				<button type="button" class:on={!tree} aria-pressed={!tree} onclick={() => (tree = false)}
					>colunas</button
				><button type="button" class:on={tree} aria-pressed={tree} onclick={() => (tree = true)}
					>árvore</button
				>
			</div>
		{:else}
			<div class="seg kinds" role="group" aria-label="Tipo">
				{#each KIND_FILTERS as k (k)}
					<button
						type="button"
						class:on={kind === k}
						aria-pressed={kind === k}
						onclick={() => (kind = k)}>{k}</button
					>
				{/each}
			</div>
		{/if}
	</div>

	{#if !focus}
		<p class="su small pad">
			{goals.loading
				? 'Carregando…'
				: 'Escolha um foco: fixe uma meta, ou use “Focar aqui” no painel de qualquer entidade.'}
		</p>
	{:else if graph.error && !g}
		<p class="red small pad">Não deu para montar o grafo: {graph.error.message}</p>
	{:else if !g}
		<p class="su small pad" aria-busy="true">Carregando…</p>
	{:else if tree}
		<div class="sub">
			Árvore · requer<span class="n">{g.nodes} nós · {g.done_nodes} feitos recolhidos</span>
		</div>
		<div class="rows">
			{#each rows as r, i (i)}
				{#if r.node}
					{@const n = r.node}
					<button
						type="button"
						class="tr"
						class:on={isOn(n)}
						class:done={n.group === 'done'}
						title={n.key}
						onclick={() => open(n)}
						><span class="ind"
							>{#each indents(r.indent) as j (j)}<i></i>{/each}</span
						><span class="cv"
							>{#if r.parent}<Glyph name="chevron" size={12} />{/if}</span
						>{@render glyph(n)}<span class="nm"
							>{n.label}{#if n.kind === 'levels' && n.levels}<span class="lvls"
									>{n.levels
										.filter((l) => l.verdict !== 'met')
										.map((l) => l.level ?? '?')
										.join(' · ')}</span
								>{/if}</span
						><span class="w"
							>{n.critical
								? 'crítico'
								: n.kind === 'levels'
									? line(n).replace(/^p\/ [^·]+· /, '')
									: n.missing?.length
										? `falta ${n.missing[0]}`
										: GROUP_LABEL[n.group]}</span
						></button
					>
				{:else if r.done}
					{@const s = doneSummary(r.done)}
					<div class="tr done">
						<span class="ind"
							>{#each indents(r.indent) as j (j)}<i></i>{/each}</span
						><span class="cv"></span><span class="v v-met"><Glyph name="check" size={9} /></span
						><span class="nm">{s.count} · {s.names}</span><span class="w"></span>
					</div>
				{/if}
			{:else}
				<p class="su small pad">Nada exigido antes de {g.focus.label} no catálogo.</p>
			{/each}
		</div>
	{:else}
		<div class="gcols" style:grid-template-columns="repeat({columns.length + 2}, minmax(0, 1fr))">
			{#each columns as c (c.depth)}
				<div class="gcol">
					<div class="hd">
						Requer · nível {c.depth}<span class="n"
							>{c.nodes.length}{#if c.done.length}&nbsp;· +{c.done.length} feitos{/if}</span
						>
					</div>
					{#each c.nodes as n (n.key)}{@render card(n, line(n))}{/each}
					{#if c.done.length && hideDone}
						{@const s = doneSummary(c.done)}
						<div class="node done">
							<span class="v v-met"><Glyph name="check" size={9} /></span><span
								>{s.count}<span class="w">{s.names}</span></span
							>
						</div>
					{:else}
						{#each c.done as n (n.key)}{@render card(n, line(n))}{/each}
					{/if}
				</div>
			{/each}
			<div class="gcol gcol-focus">
				<div class="hd">
					Foco<span class="n"
						>{goalOfFocus ? 'meta' : (KIND_LABEL[g.focus.kind] ?? g.focus.kind)}</span
					>
				</div>
				<button
					type="button"
					class="node crit"
					class:on={isOn(g.focus)}
					title={g.focus.key}
					onclick={() => open(g.focus)}
					>{@render glyph(g.focus)}<span>{g.focus.label}<span class="w">{focusLine}</span></span
					></button
				>
				{#if !g.columns.length}
					<p class="su small pad">Nada exigido antes dele no catálogo.</p>
				{/if}
			</div>
			<div class="gcol">
				<div class="hd">Destrava<span class="n">{g.unlocks.length}</span></div>
				{#each g.unlocks as n (n.key)}
					{@render card(n, [KIND_LABEL[n.kind] ?? n.kind, GROUP_LABEL[n.group]].join(' · '))}
				{:else}
					<p class="su small pad">Nada no catálogo.</p>
				{/each}
			</div>
		</div>
		<div class="legend">
			<span><span class="v v-met"><Glyph name="check" size={9} /></span>feito</span>
			<span><span class="v v-prog"></span>em progresso</span>
			<span><span class="v v-unmet"></span>disponível</span>
			<span><span class="v v-lock"></span>bloqueado</span>
			<span><span class="v v-unk">?</span>não sabemos</span>
			<span><span class="critbar"></span>caminho crítico</span>
			<span class="end"
				>cada coluna à esquerda é requisito da coluna à direita · clique abre o painel</span
			>
		</div>
	{/if}
</section>

<p class="su tiny">
	Requisitos de quest da OSRS Wiki. Passos das rotas: Guias por OzirisLoL / ironman.guide e por So
	Iron BRUH e ParasailerOSRS. {ICON_ATTRIBUTION}.
</p>

<style>
	.plaque {
		white-space: nowrap;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		padding: 12px 12px 0;
	}
	.pick {
		position: relative;
	}
	.menu {
		position: absolute;
		top: 40px;
		left: 0;
		z-index: 4;
		min-width: 300px;
		display: flex;
		flex-direction: column;
	}
	.menu .ln {
		width: 100%;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		font: inherit;
		font-size: 13px;
		color: var(--ink);
		text-align: left;
		cursor: pointer;
	}
	.menu .ln-on {
		background: var(--gold-tint);
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.add {
		color: var(--gold-ink);
		text-decoration: none;
	}
	.kinds {
		margin-left: auto;
	}
	.gcols {
		padding: 12px;
	}
	.gcol-focus .hd {
		color: var(--gold-ink);
		border-bottom-color: var(--gold-ink);
	}
	.rows {
		display: flex;
		flex-direction: column;
		padding: 0 0 8px;
	}
	.lvls {
		margin-left: 8px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 16px;
		padding: 10px 12px 12px;
		font-size: 11px;
		color: var(--ink-subtle);
		border-top: 1px solid var(--hairline);
	}
	.legend > span {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.legend .end {
		margin-left: auto;
	}
	.critbar {
		display: inline-block;
		width: 3px;
		height: 14px;
		background: var(--gold-ink);
	}
	.pad {
		padding: 10px 12px;
		margin: 0;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
</style>
