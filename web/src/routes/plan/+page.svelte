<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { createResource } from '#lib/api/resource.svelte';
	import { flattenSteps } from '#lib/planner/compare';
	import { registerGuideSwitch, clearGuideSwitch } from '#lib/planner/header.svelte';
	import { DEFAULT_GUIDE, pickGuide, planQuery } from '#lib/planner/navigation';
	import { currentAdapter, setParams } from '#lib/planner/session.svelte';
	import type { StepVM } from '#lib/planner/types';
	import BankLegCard from '#lib/components/BankLegCard.svelte';
	import CollapsibleSection from '#lib/components/CollapsibleSection.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import PlanStepRow from '#lib/components/PlanStepRow.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import VirtualList from '#lib/components/VirtualList.svelte';

	const guides = createResource(() => currentAdapter().listGuides());
	const guideId = $derived(
		pickGuide(page.url.searchParams.get('guide'), guides.data?.map((g) => g.id) ?? [DEFAULT_GUIDE])
	);
	const plan = createResource(() => currentAdapter().loadPlan(guideId));
	const leg = createResource(() => currentAdapter().nextBankLeg(guideId));

	const pickGuideInUrl = (id: string) => setParams('/plan', { guide: id });
	$effect(() => {
		if (guides.data) registerGuideSwitch(guides.data, guideId, pickGuideInUrl);
	});
	$effect(() => clearGuideSwitch);

	// "Feito"/"Pular" só avançam nesta tela: o progresso não é persistido até a API existir.
	let advanced = $state<Record<string, number>>({});
	const steps = $derived(plan.data ? flattenSteps(plan.data.sections) : []);
	const doneCount = $derived(
		plan.data ? Math.min(steps.length, plan.data.doneCount + (advanced[guideId] ?? 0)) : 0
	);
	const current = $derived<StepVM | null>(steps[doneCount] ?? null);
	const upcoming = $derived(steps.slice(doneCount + 1, doneCount + 4));
	const indexOf = $derived(new Map(steps.map((s, i) => [s.key, i])));
	const currentSection = $derived(
		plan.data?.sections.find((s) => s.steps.some((x) => x.key === current?.key)) ?? null
	);
	function advance() {
		advanced = { ...advanced, [guideId]: (advanced[guideId] ?? 0) + 1 };
	}

	// Seções: a do passo atual abre por padrão; o resto começa recolhido.
	let openState = $state<Record<string, boolean>>({});
	const isOpen = (key: string) => openState[key] ?? key === currentSection?.key;

	function doneIn(stepsOfSection: StepVM[]): number {
		return stepsOfSection.filter((s) => (indexOf.get(s.key) ?? 0) < doneCount).length;
	}
	function rowState(s: StepVM): 'done' | 'current' | 'pending' {
		const i = indexOf.get(s.key) ?? 0;
		return i < doneCount ? 'done' : i === doneCount ? 'current' : 'pending';
	}

	const guide = $derived(plan.data?.guide ?? null);
	const other = $derived(guides.data?.find((g) => g.id !== guideId) ?? null);
	const keep = $derived({
		mock: page.url.searchParams.get('mock'),
		bank: page.url.searchParams.get('bank')
	});
	const error = $derived(guides.error ?? plan.error);
	const loading = $derived(guides.loading || plan.loading);
	const reloadAll = () => (guides.reload(), plan.reload(), leg.reload());
</script>

<svelte:head>
	<title>{guide ? `Plano · ${guide.label}` : 'Plano'} · Ironman Planner</title>
</svelte:head>

{#if error}
	<ErrorState {error} retry={reloadAll} />
{:else if loading || !plan.data || !guide}
	<div class="layout" aria-busy="true">
		<section class="col-main panel"><Skeleton lines={4} height={16} /></section>
		<aside class="col-side panel"><Skeleton lines={3} height={14} /></aside>
	</div>
{:else if plan.data.sections.length === 0}
	<EmptyState
		eyebrow="{guide.label} · sem passos"
		title="Este guia ainda não foi importado"
		hint="Rode a importação do conteúdo do guia no servidor. Enquanto isso, o outro guia continua disponível."
	>
		{#if other}
			<a class="btn" href="{resolve('/plan')}{planQuery({ guide: other.id, ...keep })}">
				Abrir {other.label}
			</a>
		{/if}
	</EmptyState>
{:else}
	<div class="layout">
		<section class="col-main">
			<article class="now" aria-labelledby="now-title">
				<div class="row-top">
					<span class="eyebrow">
						Agora · {guide.label}{currentSection ? ` · seção ${currentSection.id}` : ''} · {doneCount}
						de {guide.steps}
					</span>
					{#if current}<span class="data muted push">{current.localId}</span>{/if}
				</div>
				{#if current}
					<h1 id="now-title" class="title">{current.text}</h1>
					<div class="chips">
						{#if current.location}
							<span class="chip"><Icon name="pin" size={14} /> {current.location}</span>
						{/if}
						{#if current.milestone}
							<span class="chip chip-accent">◆ {current.milestoneLabel}</span>
						{/if}
						{#if current.bank}<span class="chip">banco</span>{/if}
					</div>
					<div class="actions">
						<button class="btn btn-accent" type="button" onclick={advance}>Feito</button>
						<button class="btn" type="button" onclick={advance}>Pular</button>
						<span class="data subtle push">progresso só nesta tela até o banco entrar</span>
					</div>
				{:else}
					<h1 id="now-title" class="title">Guia concluído</h1>
				{/if}
			</article>

			<section class="checklist" aria-label="Seções do guia">
				<div class="row-top">
					<span class="eyebrow">Checklist · {guide.label}</span>
					<span class="caption">{plan.data.sections.length} seções · {guide.steps} passos</span>
					{#if other}
						<a
							class="push small"
							href="{resolve('/compare')}{planQuery({ left: guideId, right: other.id, ...keep })}"
						>
							comparar com {other.label}
						</a>
					{/if}
				</div>
				{#each plan.data.sections as s (s.key)}
					<CollapsibleSection
						id="sec-{s.id.replace(/\W/g, '-')}"
						code={s.id}
						title={s.title}
						done={doneIn(s.steps)}
						count={s.steps.length}
						open={isOpen(s.key)}
						onchange={(o) => (openState = { ...openState, [s.key]: o })}
					>
						<VirtualList items={s.steps} label="Passos da seção {s.id}">
							{#snippet row(step, i)}
								<PlanStepRow
									{step}
									state={rowState(step)}
									setsize={s.steps.length}
									posinset={i + 1}
								/>
							{/snippet}
						</VirtualList>
					</CollapsibleSection>
				{/each}
			</section>
		</section>

		<aside class="col-side">
			{#if leg.error}
				<ErrorState error={leg.error} retry={leg.reload} />
			{:else if leg.loading}
				<div class="panel"><Skeleton lines={4} height={14} /></div>
			{:else if leg.data}
				<BankLegCard leg={leg.data} />
			{:else}
				<EmptyState title="Sem banco à frente" hint="Nenhuma perna de banco restante neste guia." />
			{/if}

			{#if upcoming.length}
				<section class="panel next" aria-label="Em seguida">
					<span class="eyebrow">Em seguida</span>
					<ul>
						{#each upcoming as u (u.key)}
							<li>
								<span class="ring" role="img" aria-label="pendente"></span>
								<span class="ntext">
									<span>{u.text}</span>
									<span class="data subtle">{u.localId}{u.location ? ` · ${u.location}` : ''}</span>
								</span>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</aside>
	</div>
	<p class="caption credit">
		Guia: {guide.title}, por {guide.author}{guide.id === 'oziris-main'
			? ', via ironman.guide'
			: ''}.
	</p>
{/if}

<style>
	.layout {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-6);
		align-items: flex-start;
	}
	.col-main {
		flex: 999 1 560px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 22px;
	}
	.col-side {
		flex: 1 1 320px;
		min-width: 0;
		max-width: 400px;
		display: flex;
		flex-direction: column;
		gap: 22px;
	}
	.now {
		background: var(--surface);
		border-radius: var(--r-panel);
		padding: var(--sp-6);
		display: flex;
		flex-direction: column;
		gap: var(--sp-3);
		box-shadow: 0 20px 60px rgba(17, 17, 27, 0.65);
	}
	.row-top {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		flex-wrap: wrap;
	}
	.push {
		margin-left: auto;
	}
	.small {
		font-size: var(--fs-label);
	}
	.title {
		font-size: var(--fs-heading);
		font-weight: 400;
	}
	.chips {
		display: flex;
		gap: var(--sp-2);
		flex-wrap: wrap;
	}
	.chip {
		border-radius: 15px;
		background: var(--elevated);
		border: 0;
		color: var(--text);
		font-size: var(--fs-control);
	}
	.chip :global(svg) {
		color: var(--muted);
	}
	.chip-accent {
		background: var(--accent-tint);
		color: var(--accent);
	}
	.actions {
		display: flex;
		gap: var(--sp-2);
		flex-wrap: wrap;
		align-items: center;
		padding-top: 6px;
	}
	.checklist {
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
	}
	.next {
		display: flex;
		flex-direction: column;
		gap: var(--sp-3);
	}
	.next ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
	}
	.next li {
		display: flex;
		gap: 10px;
		align-items: flex-start;
	}
	.ring {
		width: 18px;
		height: 18px;
		border-radius: 9px;
		border: 1.7px solid var(--subtle);
		box-sizing: border-box;
		flex: none;
	}
	.ntext {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		font-size: var(--fs-control);
	}
	.credit {
		margin-top: var(--sp-4);
	}
	@media (max-width: 719px) {
		.col-side {
			max-width: none;
		}
		.now,
		.panel {
			padding: var(--sp-4);
			border-radius: 16px;
		}
		.title {
			font-size: 22px;
		}
		.actions .btn {
			flex: 1;
			height: 44px;
		}
		.actions .push {
			display: none;
		}
	}
</style>
