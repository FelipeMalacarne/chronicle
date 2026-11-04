<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { createResource } from '#lib/api/resource.svelte';
	import { buildBands, flattenSteps } from '#lib/planner/compare';
	import { DEFAULT_GUIDE, pickPair, planQuery } from '#lib/planner/navigation';
	import { currentAdapter, setParams } from '#lib/planner/session.svelte';
	import type { BankLeg, GuidePlan, StepVM } from '#lib/planner/types';
	import BankBadge from '#lib/components/BankBadge.svelte';
	import CollapsibleSection from '#lib/components/CollapsibleSection.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import PlanStepRow from '#lib/components/PlanStepRow.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import VirtualList from '#lib/components/VirtualList.svelte';

	const guides = createResource(() => currentAdapter().listGuides());
	const pair = $derived(
		pickPair(
			page.url.searchParams.get('left') ?? page.url.searchParams.get('guide'),
			page.url.searchParams.get('right'),
			guides.data?.map((g) => g.id) ?? [DEFAULT_GUIDE]
		)
	);
	const plans = createResource<[GuidePlan, GuidePlan]>(() => {
		const a = currentAdapter();
		const [l, r] = pair;
		return Promise.all([a.loadPlan(l), a.loadPlan(r)]);
	});
	const legs = createResource<(BankLeg | null)[]>(() => {
		const a = currentAdapter();
		return Promise.all(pair.map((g) => a.nextBankLeg(g)));
	});

	const lanes = $derived(
		plans.data
			? plans.data.map((p, i) => {
					const steps = flattenSteps(p.sections);
					return {
						plan: p,
						steps,
						index: new Map(steps.map((s, j) => [s.key, j])),
						current: steps[p.doneCount] ?? null,
						leg: legs.data?.[i] ?? null
					};
				})
			: []
	);
	const bands = $derived(lanes.length === 2 ? buildBands(lanes[0].steps, lanes[1].steps) : []);
	const shared = $derived(bands.filter((b) => b.milestone).length);

	function stateOf(lane: number, s: StepVM): 'done' | 'current' | 'pending' {
		const l = lanes[lane];
		const i = l.index.get(s.key) ?? 0;
		return i < l.plan.doneCount ? 'done' : i === l.plan.doneCount ? 'current' : 'pending';
	}

	let openState = $state<Record<string, boolean>>({});
	const bandId = (m: string | null) => m ?? 'inicio';
	// A banda com o passo atual de qualquer faixa abre por padrão.
	function defaultOpen(b: (typeof bands)[number]): boolean {
		return [b.left, b.right].some((side, li) => side.some((s) => lanes[li].current?.key === s.key));
	}
	const isOpen = (b: (typeof bands)[number]) => openState[bandId(b.milestone)] ?? defaultOpen(b);
	const allOpen = $derived(bands.length > 0 && bands.every((b) => isOpen(b)));
	function toggleAll() {
		const next = !allOpen;
		openState = Object.fromEntries(bands.map((b) => [bandId(b.milestone), next]));
	}
	const swap = () => setParams('/compare', { left: pair[1], right: pair[0], guide: null });

	const keep = $derived({
		mock: page.url.searchParams.get('mock'),
		bank: page.url.searchParams.get('bank')
	});
	const error = $derived(guides.error ?? plans.error);
	const empty = $derived(lanes.length === 2 && lanes.every((l) => l.steps.length === 0));
	const reloadAll = () => (guides.reload(), plans.reload(), legs.reload());
</script>

<svelte:head>
	<title>Comparar guias · Ironman Planner</title>
</svelte:head>

{#if error}
	<ErrorState {error} retry={reloadAll} />
{:else if plans.loading || guides.loading || lanes.length < 2}
	<div class="panel" aria-busy="true"><Skeleton lines={8} height={16} /></div>
{:else if empty}
	<EmptyState
		eyebrow="Comparar"
		title="Nada para comparar"
		hint="Os dois guias estão sem passos importados."
	/>
{:else}
	{@const [L, R] = lanes}
	<div class="head">
		<div class="intro">
			<span class="eyebrow">Comparar · {shared} marcos em comum</span>
			<h1 class="display">{L.plan.guide.label} × {R.plan.guide.label}</h1>
			<p class="muted desc">
				Cada guia segue a própria rota. Só os marcos que existem nos dois (◆) são ligados; entre
				eles, cada faixa mostra os passos próprios daquele guia.
			</p>
		</div>
		<div class="tools">
			<button class="btn btn-ghost" type="button" onclick={swap}>
				<Icon name="swap" size={14} /> Trocar lados
			</button>
			<button class="btn" type="button" onclick={toggleAll}>
				{allOpen ? 'Recolher todos' : 'Expandir todos'}
			</button>
		</div>
	</div>

	<div class="lanes-summary">
		{#each lanes as l (l.plan.guide.id)}
			{@const pct = Math.round((l.plan.doneCount / Math.max(1, l.steps.length)) * 100)}
			<section class="lane-card" aria-label="Faixa {l.plan.guide.label}">
				<div class="lc-top">
					<a
						class="lc-name"
						href="{resolve('/plan')}{planQuery({ guide: l.plan.guide.id, ...keep })}"
					>
						{l.plan.guide.label}
					</a>
					<span class="data muted">{l.plan.doneCount}/{l.steps.length}</span>
					{#if l.leg}<span class="push"><BankBadge bank={l.leg.bank} /></span>{/if}
				</div>
				<span class="bar" aria-hidden="true"><span style:width="{pct}%"></span></span>
				<span class="lc-now">
					Agora:
					{#if l.current}
						<span class="data subtle">{l.current.localId}</span>
						{l.current.text}
					{:else}
						guia concluído
					{/if}
					{#if l.leg}· próximo banco <b>{l.leg.location}</b>{/if}
				</span>
			</section>
		{/each}
	</div>

	<ol class="bands" aria-label="Trechos da comparação">
		{#each bands as b, bi (bandId(b.milestone))}
			{@const id = `band-${bi}`}
			{@const title = b.leftAnchor?.milestoneLabel ?? b.leftAnchor?.text ?? 'Início das rotas'}
			<li class="band">
				{#if b.milestone && b.leftAnchor && b.rightAnchor}
					<div class="connector">
						{#each [b.leftAnchor, b.rightAnchor] as a, ai (a.key)}
							{@const st = stateOf(ai, a)}
							<div class="anchor" class:right={ai === 1}>
								<span
									class="circle {st}"
									role="img"
									aria-label={st === 'done'
										? 'concluído'
										: st === 'current'
											? 'passo atual'
											: 'pendente'}>{st === 'done' ? '✓' : ''}</span
								>
								<span class="data subtle">{a.localId}</span>
								<span class="atext">{title}</span>
							</div>
							{#if ai === 0}
								<div class="link" aria-hidden="true">
									<span class="line"></span>
									<span class="tag">◆ marco {bi}</span>
									<span class="line"></span>
								</div>
							{/if}
						{/each}
						<span class="visually-hidden">
							Marco compartilhado {title}: {L.plan.guide.label}
							{b.leftAnchor.localId}, {R.plan.guide.label}
							{b.rightAnchor.localId}.
						</span>
					</div>
				{/if}

				<CollapsibleSection
					{id}
					variant="segment"
					title={b.milestone ? `Depois de ${title}` : 'Início das rotas'}
					meta="{L.plan.guide.label} {b.left.length} · {R.plan.guide.label} {b.right.length}"
					open={isOpen(b)}
					onchange={(o) => (openState = { ...openState, [bandId(b.milestone)]: o })}
				>
					<div class="lanes">
						{#each [b.left, b.right] as side, li (li)}
							{@const name = lanes[li].plan.guide.label}
							<div class="lane">
								<span class="eyebrow lane-label">{name} · {side.length} passos</span>
								{#if side.length === 0}
									<p class="caption none">Sem passos próprios neste trecho.</p>
								{:else}
									<VirtualList
										items={side}
										label="Passos de {name}: {b.milestone ? `depois de ${title}` : 'início'}"
										rowHeight={42}
										height={300}
									>
										{#snippet row(step, i)}
											<PlanStepRow
												{step}
												compact
												state={stateOf(li, step)}
												setsize={side.length}
												posinset={i + 1}
											/>
										{/snippet}
									</VirtualList>
								{/if}
							</div>
						{/each}
					</div>
				</CollapsibleSection>
			</li>
		{/each}
	</ol>

	<p class="caption credit">
		Guias: {L.plan.guide.title}, por {L.plan.guide.author}; {R.plan.guide.title}, por
		{R.plan.guide.author}. Oziris via ironman.guide.
	</p>
{/if}

<style>
	.head {
		display: flex;
		align-items: flex-end;
		gap: var(--sp-4);
		flex-wrap: wrap;
		margin-bottom: var(--sp-4);
	}
	.intro {
		flex: 1 1 420px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
	}
	.desc {
		font-size: var(--fs-control);
		max-width: 70ch;
	}
	.tools {
		display: flex;
		gap: var(--sp-2);
		flex-wrap: wrap;
	}
	.lanes-summary {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-4);
		margin-bottom: var(--sp-4);
	}
	.lane-card {
		background: var(--surface);
		border-radius: var(--r-card);
		padding: 14px var(--sp-4);
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
		min-width: 0;
	}
	.lc-top {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		flex-wrap: wrap;
	}
	.lc-name {
		font-family: var(--font-serif);
		font-size: 18px;
		color: var(--text);
	}
	.push {
		margin-left: auto;
	}
	.bar {
		display: block;
		height: 4px;
		border-radius: 2px;
		background: var(--elevated);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
	}
	.lc-now {
		font-size: var(--fs-label);
		color: var(--muted);
	}
	.lc-now b {
		font-weight: 500;
		color: var(--text);
	}
	.bands {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.band {
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
	}
	.connector {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
		align-items: center;
	}
	.anchor {
		background: var(--surface);
		border: 1px solid var(--accent);
		border-radius: var(--r-row);
		padding: 10px 14px;
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	.atext {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.link {
		display: flex;
		align-items: center;
		padding: 0 4px;
	}
	.line {
		width: 28px;
		height: 2px;
		background: var(--accent);
	}
	.tag {
		height: 26px;
		padding: 0 10px;
		border-radius: 13px;
		background: var(--accent-tint);
		color: var(--accent);
		font-family: var(--font-mono);
		font-size: var(--fs-micro);
		letter-spacing: 0.6px;
		text-transform: uppercase;
		display: inline-flex;
		align-items: center;
		white-space: nowrap;
	}
	.circle {
		width: 18px;
		height: 18px;
		border-radius: 9px;
		box-sizing: border-box;
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 700;
		border: 1.7px solid var(--subtle);
	}
	.circle.done {
		border: 0;
		background: var(--success);
		color: var(--accent-text);
	}
	.circle.current {
		border: 5px solid var(--accent);
	}
	.lanes {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-3);
		padding: 4px var(--sp-2) var(--sp-2);
	}
	.lane {
		min-width: 0;
		border-left: 2px solid var(--divider);
		padding-left: var(--sp-2);
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.lane-label {
		display: none;
		padding: 4px 0;
	}
	.none {
		padding: 10px var(--sp-2);
	}
	.credit {
		margin-top: var(--sp-4);
	}
	@media (max-width: 719px) {
		.display {
			font-size: var(--fs-heading);
		}
		.tools {
			width: 100%;
		}
		.tools .btn {
			flex: 1;
			height: 44px;
		}
		.lanes-summary,
		.lanes {
			grid-template-columns: 1fr;
		}
		.lane-label {
			display: block;
		}
		.connector {
			grid-template-columns: 1fr;
			gap: 4px;
		}
		.link {
			justify-content: center;
		}
		.line {
			width: 2px;
			height: 8px;
		}
		.link .line:first-child {
			display: none;
		}
	}
</style>
