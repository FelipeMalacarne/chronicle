<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { session } from '#lib/progress/session.svelte';
	import { routeLabel } from '#lib/progress/view';
	import Glyph from '#lib/v6/Glyph.svelte';
	import GoalPlan from '#lib/v6/GoalPlan.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * O plano inteiro da meta de uma conta (?account=&meta=), aberto por "os N requisitos" das
	 * Metas da frota. A v7 não redesenhou o plano: vale o da v6 (V6Metas1440, a meta aberta), com
	 * gargalos de nível, ordem sugerida e "Fixar próximo disponível no Agora" (decisoes-canvas 7.7).
	 */
	const account = $derived(session.account);
	const goalId = $derived(page.url.searchParams.get('meta'));
	const routes = $derived(session.summary?.routes ?? []);
	const active = $derived(routes.find((r) => r.active));
	const label = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);

	let failure = $state<string | null>(null);
	async function unpin() {
		if (!account || !goalId) return;
		failure = null;
		try {
			await progressApi.removeGoal(account, goalId);
			void goto(resolve('/mundo/metas'));
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<svelte:head>
	<title>Plano da meta · Mundo · Chronicle</title>
</svelte:head>

<MundoHead title="Plano da meta" current="metas" />

<a class="back" href={resolve('/mundo/metas')}>← Metas da frota</a>

{#if failure}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>A meta não mudou: {failure}</span>
	</div>
{/if}

{#if account && goalId}
	{#key `${account}|${goalId}`}
		<GoalPlan {account} {goalId} activeGuide={active?.guide} routeName={label} onunpin={unpin} />
	{/key}
{:else}
	<p class="mu">Escolha uma meta nas <a href={resolve('/mundo/metas')}>Metas da frota</a>.</p>
{/if}

<p class="su tiny">{ICON_ATTRIBUTION}.</p>

<style>
	.back {
		align-self: flex-start;
		font-size: 13px;
		color: var(--gold-ink);
	}
	.tiny {
		font-size: 11px;
	}
</style>
