<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { session } from '#lib/progress/session.svelte';
	import { KIND_LABEL, routeLabel } from '#lib/progress/view';
	import type { Goal, GoalPlan as Plan } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import GoalPlan from '#lib/v6/GoalPlan.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * Mundo › Metas (V6Metas1440 e V6Metas640): as metas fixadas à esquerda e a meta aberta à
	 * direita (no estreito, uma abaixo da outra). Os presets do guia ainda não existem no conteúdo:
	 * a lista termina em "+ fixar qualquer entidade".
	 */
	const account = $derived(session.account);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);
	const routes = $derived(session.summary?.routes ?? []);
	const active = $derived(routes.find((r) => r.active));
	const label = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	const achieved = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'achieved'));
	const selectedId = $derived(page.url.searchParams.get('meta') ?? pinned[0]?.id ?? null);

	/* "6 de 20" de cada meta na lista: o plano de cada uma (poucas metas). */
	let plans = $state<Record<string, Plan>>({});
	$effect(() => {
		const acc = account;
		if (!acc) return;
		for (const g of pinned.slice(0, 8))
			progressApi
				.goalPlan(acc, g.id)
				.then((r) => (plans = { ...plans, [g.id]: r.plan }))
				.catch(() => {});
	});
	function pick(id: string) {
		const url = new URL(page.url.href);
		url.searchParams.set('meta', id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replace: true, reset: false });
	}
	let failure = $state<string | null>(null);
	async function unpin(id: string) {
		if (!account) return;
		failure = null;
		try {
			await progressApi.removeGoal(account, id);
			goals.reload();
			const url = new URL(page.url.href);
			url.searchParams.delete('meta');
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
			void goto(url, { replace: true, reset: false });
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		}
	}
	const V_CLS: Record<string, string> = { met: 'v-met', unmet: 'v-unmet', unknown: 'v-unk' };
	const vOf = (g: Goal) => {
		const p = plans[g.id];
		if (g.verdict === 'met') return 'v-met';
		if (p && p.met > 0) return 'v-prog';
		return V_CLS[g.verdict] ?? 'v-unk';
	};
</script>

<svelte:head>
	<title>Metas · Mundo · Chronicle</title>
</svelte:head>

<MundoHead title="Metas" current="metas" />

{#if failure}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>A meta não mudou: {failure}</span>
	</div>
{/if}

<div class="grid">
	<section class="panel pb list">
		<div class="plaque">Metas fixadas<span class="r">{pinned.length}</span></div>
		{#each pinned as g (g.id)}
			{@const p = plans[g.id]}
			<button
				class="ln"
				class:ln-on={g.id === selectedId}
				type="button"
				aria-current={g.id === selectedId ? 'true' : undefined}
				onclick={() => pick(g.id)}
				><span class="v {vOf(g)}"
					>{#if g.verdict === 'met'}<Glyph
							name="check"
							size={9}
						/>{:else if g.verdict === 'unknown' && !p?.met}?{/if}</span
				><span class="nm">{g.label}<span class="ty">{KIND_LABEL[g.kind] ?? g.kind}</span></span
				><span class="w">{p?.requirements ? `${p.met} de ${p.total}` : ''}</span></button
			>
		{:else}
			<p class="su small pad">{goals.loading ? 'Carregando…' : 'Nenhuma meta fixada.'}</p>
		{/each}
		{#if achieved.length}
			<div class="sub">Alcançadas<span class="n">{achieved.length}</span></div>
			{#each achieved as g (g.id)}
				<div class="ln ln-done">
					<span class="v v-met"><Glyph name="check" size={9} /></span><span class="nm"
						>{g.label}</span
					><span class="w"></span>
				</div>
			{/each}
		{/if}
		<a class="ln add" href={resolve('/mundo')}
			><span></span><span class="nm">+ fixar qualquer entidade</span><span class="w">Entidades</span
			></a
		>
	</section>
	<div class="main">
		{#if account && selectedId && pinned.some((g) => g.id === selectedId)}
			{#key selectedId}
				<GoalPlan
					{account}
					goalId={selectedId}
					activeGuide={active?.guide}
					routeName={label}
					onunpin={() => unpin(selectedId)}
				/>
			{/key}
		{:else if !goals.loading}
			<p class="su small">
				Fixe uma entidade no <a href={resolve('/mundo')}>Mundo</a> com “Virar meta” para ver o plano.
			</p>
		{/if}
	</div>
</div>

<p class="su tiny">{ICON_ATTRIBUTION}.</p>

<style>
	.grid {
		display: grid;
		grid-template-columns: 330px minmax(0, 1fr);
		gap: 16px;
		align-items: start;
	}
	@media (max-width: 900px) {
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.main {
		display: flex;
		flex-direction: column;
		gap: 16px;
		min-width: 0;
	}
	button.ln {
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
	button.ln-on {
		background: var(--gold-tint);
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.add {
		color: var(--gold-ink);
		text-decoration: none;
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
