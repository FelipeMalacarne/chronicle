<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { loadSummary } from '#lib/progress/session.svelte';
	import { KIND_LABEL, pct } from '#lib/progress/view';
	import type { Bottleneck, GoalGroup, GoalNode, StepPlace } from '#lib/progress/types';
	import Glyph from './Glyph.svelte';

	/*
	 * A meta aberta (V6Metas1440/640): cabeçalho com "N de M", gargalos de nível, ordem sugerida
	 * por grupo e onde cada rota chega. Quests ficam "não sabemos" sem leitura do plugin; uma quest
	 * cujo passo você marcou feito aparece como presumida, fora do N de M.
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

	const res = createResource(() => progressApi.goalPlan(account, goalId), { tags: ['progress'] });
	const plan = $derived(res.data?.plan);
	const goal = $derived(res.data?.goal);
	const byKey = $derived(new Map((plan?.nodes ?? []).map((n) => [n.key, n])));
	const reqs = $derived((plan?.nodes ?? []).filter((n) => n.depth > 0));
	const isLevel = (n: GoalNode) => n.key.startsWith('skill:') || n.key.startsWith('qp');
	const GROUPS: { group: GoalGroup[]; title: string; hint: string }[] = [
		{ group: ['done', 'presumed'], title: 'Já feito', hint: 'evidência ou marca' },
		{ group: ['available'], title: 'Disponível agora', hint: 'requisitos cumpridos' },
		{ group: ['blocked'], title: 'Bloqueado por requisito', hint: '' },
		{ group: ['unknown'], title: 'Não sabemos', hint: 'sem leitura do plugin' }
	];
	const groupOf = (g: GoalGroup[]) =>
		reqs.filter((n) => g.includes(n.group) && !(n.group === 'blocked' && isLevel(n)));
	const nameOf = (key: string) => byKey.get(key)?.label ?? key;
	const where = (p: StepPlace | undefined) => (p ? `${routeName(p.guide_id)} ${p.ref}` : '');
	function missingLine(n: GoalNode): string {
		const miss = n.missing ?? [];
		if (!miss.length) return where(n.step);
		const levels = miss.filter((k) => k.startsWith('skill:') || k.startsWith('qp')).length;
		const quests = miss.length - levels;
		const parts: string[] = [];
		if (quests)
			parts.push(
				quests === 1
					? `falta ${nameOf(miss.find((k) => !k.startsWith('skill:'))!)}`
					: `falta ${quests} quests`
			);
		if (levels) parts.push(`${levels} ${levels === 1 ? 'nível' : 'níveis'}`);
		return parts.join(' · ');
	}
	function forLine(b: Bottleneck): string {
		// A própria meta não precisa ser repetida em cada linha.
		const first = b.for?.find((k) => k !== goal?.objective_key);
		const name = first ? nameOf(first) : '';
		const tail = b.met
			? '✓'
			: b.step
				? b.step.ref
				: b.reach_step
					? `${b.reach_step.ref} chega a ${b.reach}`
					: 'sem passo';
		return [name, tail].filter(Boolean).join(' · ');
	}
	const sortedBottlenecks = $derived(
		[...(plan?.bottlenecks ?? [])].sort((a, b) => Number(a.met) - Number(b.met))
	);
	const missingLevels = $derived(sortedBottlenecks.filter((b) => !b.met).length);
	const V_CLS: Record<string, string> = {
		done: 'v-met',
		presumed: 'v-met',
		available: 'v-unmet',
		blocked: 'v-lock',
		unknown: 'v-unk'
	};

	// "Fixar próximo disponível no Agora": o primeiro requisito aberto que a rota ativa conclui
	// num passo não feito.
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
	const stepHref = (p: StepPlace) =>
		`${resolve('/rotas/[guide]/[section]', { guide: p.guide_id, section: p.section_id })}#passo-${p.step_id}`;
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs internos vêm de resolve() + âncora; o da wiki é externo -->

{#if res.error}
	<p class="su small">O plano não carregou: {res.error.message}</p>
{:else if !plan || !goal}
	<p class="su small" aria-busy="true">Carregando o plano…</p>
{:else}
	<section class="panel head">
		<div class="hl">
			<div class="ttl">
				<span class="sr gname">{goal.label}</span>
				<span class="su small"
					>{KIND_LABEL[goal.kind] ?? goal.kind}{goal.routes.length > 1
						? ` · nas ${goal.routes.length} rotas`
						: goal.routes.length === 1
							? ` · ${routeName(goal.routes[0].guide)}`
							: ''}</span
				>
			</div>
			{#if plan.requirements}
				<span
					class="mono score"
					title={`${plan.met} de ${plan.total} requisitos cumpridos${plan.presumed ? ` · +${plan.presumed} presumidos pela marca` : ''}`}
					>{plan.met} de {plan.total} · {pct(plan.met, plan.total)}%</span
				>
			{/if}
		</div>
		{#if plan.requirements}<div class="xp">
				<span style="width: {pct(plan.met, plan.total)}%"></span>
			</div>{/if}
		<div class="acts">
			{#if next}
				<button
					class="btn btn-p"
					type="button"
					disabled={busy}
					title={`Fixa no Agora o próximo requisito disponível, na ordem da rota ativa: ${next.label} (${where(next.step)})`}
					onclick={pinNext}>Fixar próximo disponível no Agora</button
				>
			{/if}
			<button class="btn btn-g" type="button" onclick={() => openEntity(goal.ref)}
				>Ver entidade</button
			>
			<button class="btn btn-g" type="button" onclick={onunpin}>Desafixar</button>
			{#if failure}<span class="red small">Não fixou: {failure}</span>{/if}
		</div>
	</section>

	{#if plan.requirements || plan.bottlenecks.length}
		<div class="two">
			{#if plan.bottlenecks.length}
				<section class="panel pb">
					<div class="plaque">
						Gargalos de nível<span class="r" title="Requisitos da wiki, conferidos na extração"
							>{missingLevels} {missingLevels === 1 ? 'falta' : 'faltam'}</span
						>
					</div>
					<div>
						{#each sortedBottlenecks as b (b.key)}
							{@const icon = b.skill === 'qp' ? null : iconFor(`skill:${b.skill}`)}
							<div
								class="bt"
								title={`${nameOf(b.key)}: ${b.level === null ? 'sem leitura' : `você está em ${b.level}`}, a meta pede ${b.target}${b.boostable ? ' (vale boost)' : ''}${b.during_quest ? ' (durante a quest)' : ''}`}
							>
								{#if icon}<img src={icon} alt="" />{:else if b.level === null}<span class="v v-unk"
										>?</span
									>{:else}<span></span>{/if}
								<span>{b.skill === 'qp' ? 'Quest points' : nameOf(b.key)}</span>
								{#if b.met}<span class="lv ok">{b.level} ≥ {b.target} ✓</span>{:else}<span
										class="lv">{b.level ?? '?'} → <b>{b.target}</b></span
									>{/if}
								<span class="for">{forLine(b)}</span>
							</div>
						{/each}
					</div>
				</section>
			{/if}
			{#if plan.requirements}
				<section class="panel pb">
					<div class="plaque">
						Ordem sugerida<span class="r" title="Segue a rota ativa quando possível"
							>rota ativa · {activeGuide ? routeName(activeGuide) : '—'}</span
						>
					</div>
					<div>
						{#each GROUPS as g (g.title)}
							{@const items = groupOf(g.group)}
							{#if items.length || (g.group.includes('blocked') && missingLevels)}
								<div class="sub">
									{g.title}<span class="n">{items.length}{g.hint ? ` · ${g.hint}` : ''}</span>
								</div>
								{#if g.group.includes('done')}
									<div class="ln ln-done">
										<span class="v v-met"><Glyph name="check" size={9} /></span><span
											class="nm wrap">{items.map((n) => n.label).join(' · ')}</span
										><span class="w"></span>
									</div>
								{:else}
									{#each items as n (n.key)}
										<button class="ln" type="button" onclick={() => n.ref && openEntity(n.ref)}
											><span class="v {V_CLS[n.group]}"
												>{#if n.group === 'unknown'}?{/if}</span
											><span class="nm wrap">{n.label}</span><span class="w">{missingLine(n)}</span
											></button
										>
									{/each}
									{#if g.group.includes('blocked') && missingLevels}
										<div class="ln">
											<span class="v v-unmet"></span><span class="nm wrap"
												>{sortedBottlenecks
													.filter((b) => !b.met)
													.slice(0, 2)
													.map((b) => `${b.target} ${b.skill === 'qp' ? 'QP' : nameOf(b.key)}`)
													.join(' · ')}{missingLevels > 2 ? ` +${missingLevels - 2}` : ''}</span
											><span class="w">gargalo</span>
										</div>
									{/if}
								{/if}
							{/if}
						{/each}
					</div>
				</section>
			{/if}
		</div>
	{/if}

	<section class="panel pb">
		<div class="plaque">Onde cada rota chega aqui<span class="r">passos reais dos guias</span></div>
		<div>
			{#each plan.routes as p (`${p.guide_id}::${p.step_id}`)}
				<a class="rtl" href={stepHref(p)}
					><span class="g">{routeName(p.guide_id)}</span><span class="i">{p.ref}</span><span
						class="t">{p.title}</span
					><span class="r"
						>{p.goal ? 'conclui a meta' : p.role === 'starts' ? 'inicia' : 'continua'}{p.done
							? ' · feito'
							: ''}</span
					></a
				>
			{:else}
				<p class="su small pad">Nenhuma rota conclui esta meta.</p>
			{/each}
		</div>
	</section>
{/if}

<style>
	.head {
		padding: 14px 16px 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.hl {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1 1 240px;
	}
	.gname {
		font-size: 22px;
	}
	.score {
		font-size: 12px;
		color: var(--ink-subtle);
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.acts .btn {
		height: 40px;
	}
	.two {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(360px, 100%), 1fr));
		gap: 16px;
		align-items: start;
	}
	.lv.ok {
		color: var(--done);
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
	.wrap {
		white-space: normal !important;
	}
	.rtl {
		text-decoration: none;
		color: var(--ink);
	}
	.rtl:hover {
		background: var(--elevated);
	}
	.pad {
		padding: 10px 12px;
		margin: 0;
	}
	.small {
		font-size: 12px;
	}
</style>
