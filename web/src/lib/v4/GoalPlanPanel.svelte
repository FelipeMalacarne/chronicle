<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { loadSummary } from '#lib/progress/session.svelte';
	import { sectionLabel } from '#lib/progress/view';
	import type { Bottleneck, GoalGroup, GoalNode, StepPlace } from '#lib/progress/types';

	/*
	 * Plano da meta (V4Meta, coluna central): "N de M", gargalos de nível, ordem sugerida em
	 * colunas e onde cada rota chega. Fora desta entrega (decisão do dono, 2026-11-04): o grafo
	 * por foco, os presets do guia e a estimativa de tempo. Quests ficam "não sabemos" até o
	 * plugin 1.1; uma quest cujo passo você marcou feito aparece como presumida, fora do N de M.
	 */
	let {
		account,
		goalId,
		activeGuide,
		routeName,
		onunpin
	}: {
		account: string;
		goalId: string;
		activeGuide: string | undefined;
		routeName: (guide: string) => string;
		onunpin: () => void;
	} = $props();

	const res = createResource(() => progressApi.goalPlan(account, goalId));
	const plan = $derived(res.data?.plan);
	const goal = $derived(res.data?.goal);
	const byKey = $derived(new Map((plan?.nodes ?? []).map((n) => [n.key, n])));
	const pctOf = (met: number, total: number) => (total ? Math.round((met / total) * 100) : 0);

	const COLUMNS: { group: GoalGroup[]; title: string; hint: string }[] = [
		{ group: ['done', 'presumed'], title: 'Já feito', hint: 'evidência, ou presumido pela marca' },
		{ group: ['available'], title: 'Disponível agora', hint: 'requisitos cumpridos' },
		{ group: ['blocked'], title: 'Bloqueado por requisito', hint: 'falta algo antes' },
		{ group: ['unknown'], title: 'Não sabemos', hint: 'quests até o plugin 1.1' }
	];
	const reqs = $derived((plan?.nodes ?? []).filter((n) => n.depth > 0));
	const isLevel = (n: GoalNode) => n.key.startsWith('skill:') || n.key.startsWith('qp');
	// Em "Bloqueado", os níveis viram um card só de gargalo (detalhados em Gargalos de nível).
	const column = (g: GoalGroup[]) =>
		reqs.filter((n) => g.includes(n.group) && !(n.group === 'blocked' && isLevel(n)));
	const openLevels = $derived((plan?.bottlenecks ?? []).filter((b) => !b.met));

	const nameOf = (key: string) => byKey.get(key)?.label ?? key;
	const where = (p: StepPlace | undefined) =>
		p ? `${routeName(p.guide_id)} · ${p.ref}` : 'sem passo na rota ativa';
	function missingLine(n: GoalNode): string {
		const miss = n.missing ?? [];
		if (!miss.length) return n.group === 'unknown' ? 'requisitos ok' : '';
		const levels = miss.filter((k) => k.startsWith('skill:') || k.startsWith('qp')).length;
		const quests = miss.length - levels;
		const parts: string[] = [];
		if (quests)
			parts.push(
				quests === 1
					? `falta ${nameOf(miss.find((k) => k.startsWith('quest:'))!)}`
					: `${quests} quests`
			);
		if (levels) parts.push(`${levels} ${levels === 1 ? 'nível' : 'níveis'}`);
		return parts.join(' · ');
	}
	function bottleneckWhere(b: Bottleneck): string {
		const forName = b.for?.length ? nameOf(b.for[0]) : '';
		if (b.met) return `✓ ${forName}`;
		if (b.step) return `${forName} · ${b.step.ref}`;
		if (b.reach_step) return `${forName} · ${b.reach_step.ref} chega a ${b.reach}`;
		return `${forName} · sem passo`;
	}
	const abbr = (skill: string) => (skill === 'qp' ? 'QP' : skill.slice(0, 3).toUpperCase());

	// "Fixar próximo disponível em Agora": o primeiro requisito da ordem, ainda aberto, que a
	// rota ativa conclui num passo não feito.
	const next = $derived(
		reqs.find(
			(n) =>
				(n.group === 'available' || n.group === 'unknown') &&
				!(n.missing ?? []).length &&
				n.step &&
				n.step.guide_id === activeGuide &&
				!n.step.done
		)
	);
	let busy = $state(false);
	let failure = $state<string | null>(null);
	async function pinNext() {
		if (!next?.step) return;
		busy = true;
		failure = null;
		try {
			await progressApi.pin(next.step.guide_id, account, next.step.step_id);
			void loadSummary(account);
			void goto(resolve('/'));
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<section class="panel gp" aria-label="Plano da meta">
	{#if res.error}
		<div class="pad su small">O plano não carregou: {res.error.message}</div>
	{:else if !plan || !goal}
		<div class="pad su small">Carregando o plano…</div>
	{:else}
		{@const icon = iconFor(goal.ref)}
		<div class="head pad">
			<div class="hl">
				<span class="ey"
					>meta · {goal.kind}{goal.routes.length ? ` · ${goal.routes.length} rotas` : ''}</span
				>
				<h2 class="sr nm">
					{#if icon}<img class="ski" src={icon} alt="" />{/if}{goal.label}
				</h2>
				{#if plan.requirements}
					<span class="su small"
						>Plano = fechamento dos requisitos no catálogo, menos o que já está feito, na ordem da
						rota ativa quando houver empate.</span
					>
				{:else}
					<span class="su small"
						>Sem requisitos no catálogo para esta meta: por enquanto só quests têm requisitos.
						Abaixo, onde cada rota a conclui.</span
					>
				{/if}
			</div>
			{#if plan.requirements}
				<div class="score">
					<span class="mono small">{plan.met} de {plan.total} requisitos</span>
					<span class="pct">{pctOf(plan.met, plan.total)}%</span>
					{#if plan.presumed}<span class="su tiny">+{plan.presumed} presumidos pela marca</span
						>{/if}
				</div>
			{/if}
		</div>
		<div class="pad acts">
			{#if next}
				<button class="btn btn-p btn-s" type="button" disabled={busy} onclick={pinNext}
					>Fixar próximo disponível em Agora</button
				>
				<span class="su tiny">{next.label} · {where(next.step)}</span>
			{/if}
			<button class="btn btn-g btn-s" type="button" onclick={onunpin}>Desafixar</button>
			{#if failure}<span class="su small">Não fixou: {failure}</span>{/if}
		</div>

		{#if plan.bottlenecks.length}
			<div class="plaque">
				Gargalos de nível<span class="r">hiscores · requisitos do wiki</span>
			</div>
			<div class="pad bgrid">
				{#each plan.bottlenecks as b (b.key)}
					{@const sicon = b.skill === 'qp' ? null : iconFor(`skill:${b.skill}`)}
					<div class="bn" class:bn-met={b.met}>
						<span class="ab"
							>{#if sicon}<img src={sicon} alt="" />{/if}{abbr(b.skill)}</span
						>
						<span class="lv">{b.level ?? '?'} <span class="to">→ {b.target}</span></span>
						<span class="su tiny"
							>{bottleneckWhere(b)}{b.during_quest ? ' · durante a quest' : ''}{b.boostable &&
							!b.met
								? ' · boost'
								: ''}</span
						>
					</div>
				{/each}
			</div>
		{/if}

		{#if plan.requirements}
			<div class="plaque">
				Ordem sugerida<span class="r">segue a rota ativa quando possível</span>
			</div>
			<div class="pad cols">
				{#each COLUMNS as c (c.title)}
					{@const items = column(c.group)}
					{@const count = items.length + (c.group.includes('blocked') && openLevels.length ? 1 : 0)}
					<div class="col">
						<div class="ch">
							<span class="ey">{c.title}</span><span class="mono tiny">{count}</span>
						</div>
						<span class="su tiny">{c.hint}</span>
						{#if c.group.includes('blocked') && openLevels.length}
							<div class="req">
								<span class="rn"
									>{openLevels
										.slice(0, 3)
										.map((b) => `${b.target} ${b.skill === 'qp' ? 'QP' : nameOf(b.key)}`)
										.join(' · ')}{openLevels.length > 3 ? ` +${openLevels.length - 3}` : ''}</span
								>
								<span class="su tiny">gargalo · {openLevels.length} níveis abaixo do alvo</span>
							</div>
						{/if}
						{#each items as n (n.key)}
							{@const nicon = n.ref ? iconFor(n.ref) : null}
							<div class="req" class:req-pres={n.group === 'presumed'}>
								<span class="rn"
									>{#if nicon}<img
											class="ski"
											src={nicon}
											alt=""
										/>{/if}{n.label}{#if n.key.startsWith('skill:') || n.key.startsWith('qp')}
										<span class="mono tiny">{n.key.split('>=')[1]}</span>{/if}</span
								>
								<span class="su tiny"
									>{n.group === 'presumed' ? 'presumido pela marca' : missingLine(n)}{n.during_quest
										? ' · durante a quest'
										: ''}</span
								>
								{#if n.step}<span class="su tiny">{routeName(n.step.guide_id)} · {n.step.ref}</span
									>{/if}
							</div>
						{/each}
						{#if !count}<span class="su tiny">—</span>{/if}
					</div>
				{/each}
			</div>
		{/if}

		<div class="plaque">Onde cada rota chega aqui<span class="r">passos reais dos guias</span></div>
		<div class="pad places">
			{#each plan.routes as p (`${p.guide_id}::${p.step_id}`)}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + âncora do passo -->
				<a
					class="place"
					href={`${resolve('/rotas/[guide]/[section]', { guide: p.guide_id, section: p.section_id })}#passo-${p.step_id}`}
				>
					<span class="ey">{routeName(p.guide_id)}</span>
					<span class="mono">{p.ref}</span>
					<span class="small">{p.title}</span>
					<span class="su tiny"
						>{sectionLabel(p.section_id)} · pos. {p.section_pos}{p.goal
							? ' · conclui a meta'
							: p.role === 'starts'
								? ' · inicia'
								: ''}{p.done ? ' · feito' : ''}</span
					>
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{:else}
				<span class="su small">Nenhuma rota conclui esta meta.</span>
			{/each}
		</div>
	{/if}
</section>

<style>
	.gp {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.pad {
		padding: 12px 14px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		gap: 12px 20px;
		justify-content: space-between;
		align-items: flex-end;
	}
	.hl {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1 1 320px;
		min-width: 0;
	}
	.nm {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 24px;
		margin: 0;
	}
	.score {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 2px;
	}
	.pct {
		font:
			600 28px/1 Inter,
			system-ui,
			sans-serif;
		color: var(--gold-ink);
		font-variant-numeric: tabular-nums;
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		padding-top: 0;
	}
	.bgrid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(150px, 100%), 1fr));
		gap: 8px;
	}
	.bn {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 10px;
		border-radius: 2px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--on-frame-muted);
		min-width: 0;
	}
	.bn .su {
		color: var(--on-frame-muted);
	}
	.bn-met {
		opacity: 0.7;
	}
	.ab {
		display: flex;
		align-items: center;
		gap: 5px;
		font:
			500 10px/1 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
	}
	.ab img {
		width: 16px;
		height: 16px;
		image-rendering: pixelated;
	}
	.lv {
		font:
			600 20px/1.1 Inter,
			system-ui,
			sans-serif;
		font-variant-numeric: tabular-nums;
		color: var(--gold);
	}
	.lv .to {
		font-size: 14px;
		color: var(--on-frame);
	}
	.cols {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
		gap: 12px;
	}
	.col {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
	.ch {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.req {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 10px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--elevated);
		box-shadow: var(--bevel);
		min-width: 0;
	}
	.req-pres {
		border-style: dashed;
	}
	.rn {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 13px;
		font-weight: 500;
		overflow-wrap: anywhere;
	}
	.ski {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		flex: 0 0 auto;
	}
	.places {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(240px, 100%), 1fr));
		gap: 8px;
	}
	.place {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 10px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--elevated);
		color: var(--ink);
		text-decoration: none;
		min-width: 0;
	}
	.place:hover {
		border-color: var(--gold-ink);
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
</style>
