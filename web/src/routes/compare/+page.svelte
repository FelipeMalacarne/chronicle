<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { session, setSummary } from '#lib/progress/session.svelte';
	import {
		crossCell,
		crossObjectives,
		distanceTo,
		firstReaching,
		objectiveLabel,
		pct,
		routeLabel,
		sectionLabel,
		type CrossCell
	} from '#lib/progress/view';
	import type { Board, Goal, GoalPlan, RouteSummary, WorldEntity } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import RouteTabs from '#lib/v6/RouteTabs.svelte';

	/*
	 * Comparar rotas (V6Comparar1440, aba de Rotas): duas rotas lado a lado sobre o mesmo
	 * progresso. Cada coluna é uma rota inteira com cursor e marcas próprios (regra 7); só a
	 * evidência cruza de rota, a marca à mão não. Revê a decisão 3 da v4 (dono, 2026-11-04).
	 */
	const account = $derived(session.account);
	const routes = $derived(
		[...(session.summary?.routes ?? [])].sort((a, b) => Number(b.active) - Number(a.active))
	);
	let otherPick = $state<string | null>(null);
	const pair = $derived.by((): RouteSummary[] => {
		const [first, ...rest] = routes;
		if (!first) return [];
		const other = rest.find((r) => r.guide === otherPick) ?? rest[0];
		return other ? [first, other] : [first];
	});

	const boards = createResource(
		() => {
			const a = account;
			const guides = pair.map((r) => r.guide);
			return a ? Promise.all(guides.map((g) => progressApi.board(g, a))) : Promise.resolve([]);
		},
		{ tags: ['progress'] }
	);
	const boardOf = (guide: string): Board | undefined => boards.data?.find((b) => b.guide === guide);

	const world = createResource(
		() =>
			account ? progressApi.world(account) : Promise.resolve({ entities: [] as WorldEntity[] }),
		{ tags: ['progress'] }
	);
	const names = $derived(
		Object.fromEntries((world.data?.entities ?? []).map((e) => [e.ref, e.name]))
	);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);

	const AUTHORS: Record<string, string> = {
		'oziris-main': 'OzirisLoL · ironman.guide',
		'bruhsailer-main': 'So Iron BRUH e ParasailerOSRS'
	};
	const label = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);

	function cursorOf(r: RouteSummary) {
		const b = boardOf(r.guide);
		const id = b?.cursor?.step_id;
		if (!b || !id) return null;
		const step = b.steps.find((s) => s.id === id);
		if (!step) return null;
		const inSection = b.steps.filter((s) => s.section_id === step.section_id);
		return {
			step,
			pos: inSection.findIndex((s) => s.id === id) + 1,
			of: inSection.length,
			title: r.sections.find((s) => s.id === step.section_id)?.title ?? ''
		};
	}
	const sectionHref = (guide: string, section: string, step?: string) =>
		`${resolve('/rotas/[guide]/[section]', { guide, section })}${step ? `#passo-${step}` : ''}`;

	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);
	async function activate(guide: string) {
		if (!account) return;
		busy = guide;
		failure = null;
		try {
			setSummary(await progressApi.setActiveGuide(account, guide));
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = null;
		}
	}

	/* Feito numa, conta na outra */
	const FILTERS: [string, string][] = [
		['all', 'tudo'],
		['ahead', 'só adiantados'],
		['manual', 'feitos à mão']
	];
	let filter = $state('all');
	const rows = $derived(
		boards.data && pair.length === 2
			? crossObjectives(pair.map((r) => ({ guide: r.guide, steps: boardOf(r.guide)?.steps ?? [] })))
			: []
	);
	const shown = $derived(
		rows
			.filter((r) => {
				const cells = pair.map((p) => crossCell(r, p.guide));
				if (filter === 'ahead') return r.verdict === 'met';
				if (filter === 'manual') return cells.includes('feito à mão');
				return true;
			})
			.slice(0, 40)
	);
	const metShared = $derived(rows.filter((r) => r.verdict === 'met').length);
	const CELL_CLS: Record<CrossCell, string> = {
		feito: 'badge-done',
		adianta: 'badge-auto',
		'feito à mão': '',
		'não sabemos': 'badge-unk',
		pendente: '',
		pulado: ''
	};
	const V_CLS = { met: 'v-met', unmet: 'v-unmet', unknown: 'v-unk' } as const;

	/* Até o mesmo marco */
	let markId = $state<string | null>(null);
	const activeGoals = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	const mark = $derived(activeGoals.find((g) => g.id === markId) ?? activeGoals[0] ?? null);
	const plan = createResource(
		() => {
			const a = account;
			const m = mark;
			return a && m
				? progressApi.goalPlan(a, m.id).then((v) => v.plan)
				: Promise.resolve(null as GoalPlan | null);
		},
		{ tags: ['progress'] }
	);
	const gargalo = $derived(plan.data?.bottlenecks?.find((b) => !b.met) ?? null);
	function markIn(guide: string) {
		const b = boardOf(guide);
		const occ = mark?.routes.find((o) => o.guide === guide);
		if (!b || !occ) return null;
		const step = b.steps.find((s) => s.id === occ.step);
		const dist = distanceTo(b.steps, occ.step);
		const reach = gargalo ? firstReaching(b.steps, gargalo.key) : null;
		return { occ, step, dist, reach };
	}
</script>

<svelte:head>
	<title>Comparar rotas · Chronicle</title>
</svelte:head>

<div class="page">
	<RouteTabs current="compare" />
	<header class="top">
		<div class="ttl">
			<p class="su small">Rotas · lado a lado · o progresso é da conta</p>
			<h1 class="sr">Comparar rotas</h1>
		</div>
		{#if routes.length > 2}
			<div class="kinds">
				{#each routes.slice(1) as r (r.guide)}
					<button
						class="chip"
						class:chip-on={pair[1]?.guide === r.guide}
						type="button"
						onclick={() => (otherPick = r.guide)}>{label(r.guide)}</button
					>
				{/each}
			</div>
		{/if}
	</header>

	{#if !account}
		<div class="band">
			<Glyph name="alert" /><span>Sem conta: abra o primeiro acesso em /setup.</span>
		</div>
	{:else if failure}
		<div class="band"><Glyph name="alert" /><span>A rota ativa não mudou: {failure}</span></div>
	{/if}

	<div class="pair">
		{#each pair as r (r.guide)}
			{@const c = cursorOf(r)}
			{@const done = r.counts.done}
			<section class="panel col">
				<div class="plaque">
					{label(r.guide)}{#if r.active}<span class="badge badge-gold on-frame">ativa</span
						>{/if}<span class="r"
						>{AUTHORS[r.guide] ?? r.source?.author ?? ''} · {r.counts.total} passos</span
					>
				</div>
				<div class="pad body">
					<div class="cur">
						<span class="ey"
							>{r.active ? 'Passo atual · cursor' : 'Onde você estaria seguindo esta rota'}</span
						>
						{#if c}
							<span
								><span class="mono ref">{c.step.ref}</span>
								{c.step.title}</span
							>
							<span class="su tiny"
								>{sectionLabel(c.step.section_id)} · pos. {c.pos} de {c.of}{c.title
									? ` · ${c.title}`
									: ''}</span
							>
						{:else}
							<span class="su small">{boards.loading ? 'Carregando…' : 'Rota concluída.'}</span>
						{/if}
					</div>
					<div class="stats">
						<div class="stat">
							<span class="ey">feitos</span><span class="num">{done}</span><span class="su tiny"
								>{pct(done, r.counts.total)}% dos {r.counts.total}</span
							>
						</div>
						<div class="stat">
							<span class="ey">presumidos</span><span class="num">{r.counts.implied}</span>
							{#if r.counts.implied}<span class="su tiny">dentro dos feitos</span>{/if}
						</div>
						<div class="stat">
							<span class="ey">pulados</span><span class="num">{r.counts.skipped}</span>
						</div>
						<div class="stat">
							<span class="ey">restantes</span><span class="num">{r.counts.remaining}</span>
						</div>
					</div>
					<div class="bars">
						<div class="bl">
							<span class="su small">passos feitos</span><span class="mono tiny"
								>{done} / {r.counts.total}</span
							>
						</div>
						<div class="xp xp2">
							<span style:width={`${pct(done - r.counts.implied, r.counts.total)}%`}></span><span
								class="pr"
								style:width={`${pct(r.counts.implied, r.counts.total)}%`}
							></span>
						</div>
						<div class="bl">
							<span
								class="su small"
								title={r.active
									? 'Listrado = presumido pela cascata. Quests sem leitura do plugin contam "não sabemos", nunca como falta.'
									: 'O progresso é um só: objetivos feitos seguindo a outra rota valem aqui. Marcas à mão e cursor são desta rota.'}
								>objetivos com evidência</span
							><span class="mono tiny"
								>{r.coverage.met} / {r.coverage.objectives} · {pct(
									r.coverage.met,
									r.coverage.objectives
								)}%</span
							>
						</div>
						<div class="xp xp-gold">
							<span style:width={`${pct(r.coverage.met, r.coverage.objectives)}%`}></span>
						</div>
					</div>
					<div class="acts">
						{#if r.active}
							<a class="btn btn-p btn-s" href={resolve('/agora')}>Abrir no Agora</a>
						{:else}
							<button
								class="btn btn-p btn-s"
								type="button"
								disabled={busy !== null}
								onclick={() => activate(r.guide)}
								>{busy === r.guide ? 'Trocando…' : 'Tornar ativa'}</button
							>
						{/if}
						{#if c}
							<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + âncora do passo -->
							<a class="btn btn-s" href={sectionHref(r.guide, c.step.section_id, c.step.id)}
								>Ver {sectionLabel(c.step.section_id)}</a
							>
							<!-- eslint-enable svelte/no-navigation-without-resolve -->
						{/if}
						{#if !r.active}<span class="su tiny">trocar não perde o lugar na outra</span>{/if}
					</div>
				</div>
			</section>
		{/each}
	</div>

	{#if pair.length === 2}
		<section class="panel">
			<div class="plaque">
				Feito numa, conta na outra<span class="r"
					>objetivos compartilhados · {metShared} de {rows.length} com evidência</span
				>
			</div>
			<div class="pad xs">
				<div class="kinds">
					{#each FILTERS as [f, l] (f)}
						<button
							class="chip"
							class:chip-on={filter === f}
							type="button"
							onclick={() => (filter = f)}>{l}</button
						>
					{/each}
				</div>
				<div class="cross head">
					<span class="ey">objetivo</span>
					{#each pair as r (r.guide)}<span class="ey">{label(r.guide)}</span>{/each}
				</div>
				{#each shown as row (row.key)}
					{@const icon = iconFor(row.key.split(/[@>x]/)[0])}
					<div class="cross">
						<span class="nm"
							><span class="v {V_CLS[row.verdict]}">{row.verdict === 'unknown' ? '?' : ''}</span
							>{#if icon}<img class="ski" src={icon} alt="" />{/if}{objectiveLabel(
								row.key,
								names
							)}</span
						>
						{#each pair as r (r.guide)}
							{@const cell = crossCell(row, r.guide)}
							{@const p = row.places[r.guide]}
							<span class="cell"
								>{#if cell}<span class="badge {CELL_CLS[cell]}">{cell}</span><span class="w"
										>{p.ref}{p.role === 'starts' ? ' inicia' : ''}</span
									>{/if}</span
							>
						{/each}
					</div>
				{:else}
					<span class="su small">{boards.loading ? 'Carregando…' : 'Nada nesse filtro.'}</span>
				{/each}
				<span class="su tiny"
					>"Feito à mão" numa rota não vira feito na outra: só evidência (hiscores, plugin) cruza.{rows.length >
					40
						? ` Mostrando 40 de ${rows.length}.`
						: ''}</span
				>
			</div>
		</section>

		<section class="panel">
			<div class="plaque">
				Até o mesmo marco<span
					class="r"
					title="Contagem de passos, não de tempo. Passos têm tamanhos diferentes entre as rotas (linha curta no Oziris, parágrafo no BRUHsailer): o número compara posição, não esforço."
					>escolha uma meta</span
				>
			</div>
			<div class="pad xs">
				{#if activeGoals.length}
					<div class="kinds">
						{#each activeGoals as g (g.id)}
							{@const gicon = iconFor(g.ref)}
							<button
								class="chip"
								class:chip-on={mark?.id === g.id}
								type="button"
								onclick={() => (markId = g.id)}
								>{#if gicon}<img class="ski" src={gicon} alt="" />{/if}{g.label}</button
							>
						{/each}
						<a class="chip" href={resolve('/mundo')}>+ outra meta no Mundo</a>
					</div>
					<div class="marks">
						{#each pair as r (r.guide)}
							{@const m = markIn(r.guide)}
							<div class="mk">
								<div class="mh">
									<span class="badge" class:badge-gold={r.active}>{label(r.guide)}</span>
									{#if m}
										<span class="mono small">{m.occ.ref}</span><span class="su small"
											>{m.step?.title ?? m.occ.title ?? ''}</span
										>
									{/if}
								</div>
								{#if !m}
									<span class="su small">Esta rota não conclui {mark?.label}.</span>
								{:else}
									<div class="stats">
										<div class="stat">
											<span class="ey">passos até lá</span><span class="num"
												>{m.dist?.steps ?? 0}</span
											>
										</div>
										<div class="stat">
											<span class="ey">{r.guide.startsWith('bruh') ? 'capítulos' : 'seções'}</span
											><span class="num">{m.dist?.sections.length ?? 0}</span>
											{#if m.dist && m.dist.sections.length > 1}<span class="su tiny"
													>{sectionLabel(m.dist.sections[0])} → {sectionLabel(
														m.dist.sections[m.dist.sections.length - 1]
													)}</span
												>{/if}
										</div>
										{#if gargalo}
											<div class="stat">
												<span class="ey">gargalo</span><span class="num gl"
													>{objectiveLabel(gargalo.key, names)}</span
												><span class="su tiny"
													>{m.reach
														? `a rota chega em ${m.reach.ref}`
														: 'a rota não chega lá'}</span
												>
											</div>
										{/if}
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{:else}
					<span class="su small"
						>Nenhuma meta fixada. Fixe uma no <a href={resolve('/mundo')}>Mundo</a> para comparar quanto
						falta em cada rota.</span
					>
				{/if}
			</div>
		</section>
	{/if}

	<p class="su tiny foot">
		Guias por OzirisLoL / ironman.guide e por So Iron BRUH e ParasailerOSRS. {ICON_ATTRIBUTION}.
	</p>
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
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
	.aside {
		margin-left: auto;
	}
	.kinds {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}
	.pair {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(380px, 100%), 1fr));
		gap: 16px;
	}
	.col {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.on-frame {
		background: var(--gold-tint);
		color: var(--gold);
	}
	.pad {
		padding: 14px;
	}
	.body,
	.xs {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.cur {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 10px 12px;
		border-radius: 4px;
		background: var(--elevated);
		box-shadow: var(--bevel);
		border: 1px solid var(--border);
		min-width: 0;
	}
	.ref {
		font-size: 12px;
		color: var(--gold-ink);
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(90px, 1fr));
		gap: 10px;
	}
	.stat {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.num {
		font:
			600 22px/1.1 Inter,
			system-ui,
			sans-serif;
		font-variant-numeric: tabular-nums;
	}
	.num.gl {
		font-size: 16px;
	}
	.bars {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.bl {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}
	.xp2 {
		display: flex;
	}
	.xp2 > .pr {
		background: repeating-linear-gradient(135deg, var(--done) 0 3px, transparent 3px 6px);
	}
	.acts {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		align-items: center;
	}
	.cross {
		display: grid;
		grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr) minmax(0, 1fr);
		gap: 4px 14px;
		align-items: center;
		min-height: 40px;
		padding: 4px 8px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.cross.head {
		min-height: 28px;
	}
	.nm {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.cell {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		min-width: 0;
		font-size: 12px;
	}
	.w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
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
	}
	.v-met {
		background: var(--done);
		border-color: var(--done);
	}
	.v-unmet {
		border-color: var(--not-started);
	}
	.v-unk {
		border-style: dotted;
		border-color: var(--ink-subtle);
		color: var(--ink-subtle);
		background: var(--elevated);
	}
	.ski {
		width: 16px;
		height: 16px;
		image-rendering: pixelated;
		flex: 0 0 auto;
	}
	.chip .ski {
		margin-right: 2px;
	}
	.marks {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
		gap: 12px;
	}
	.mk {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		border-radius: 4px;
		background: var(--elevated);
		box-shadow: var(--bevel);
		border: 1px solid var(--border);
		min-width: 0;
	}
	.mh {
		display: flex;
		align-items: baseline;
		gap: 8px;
		flex-wrap: wrap;
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
	@media (max-width: 640px) {
		.page {
			padding: 16px;
		}
		.cross {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
		.cross .nm {
			grid-column: 1 / -1;
		}
	}
</style>
