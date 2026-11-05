<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { quietDetection } from '#lib/api/events.svelte';
	import { createResource } from '#lib/api/resource.svelte';
	import { rowChip, sessionLine, whoDid } from '#lib/progress/agora';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import {
		chipsOf,
		metaOf,
		pct,
		routeLabel,
		routeTitle,
		sectionLabel,
		segmentsOf,
		sourceBand,
		statusView
	} from '#lib/progress/view';
	import type {
		Board,
		BoardStep,
		Goal,
		GoalPlan,
		Mark,
		MarkAction,
		MarkRestore
	} from '#lib/progress/types';
	import BankPanel from '#lib/v6/BankPanel.svelte';
	import CurrentStep from '#lib/v6/CurrentStep.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import ObjectiveChip from '#lib/v6/ObjectiveChip.svelte';
	import PinDialog from '#lib/v6/PinDialog.svelte';
	import Toast from '#lib/v6/Toast.svelte';

	/*
	 * Agora (V6Agora1440 e V6Agora640; BRUHsailer em V6AgoraBruh1440/640; fonte falhou e Fixar
	 * outro em V6AgoraEstados1440): o passo atual, "Levar do banco" e "Em seguida". A 1440 o banco
	 * e as metas ficam na coluna da direita; a 640 o banco vem logo depois do passo e o que não tem
	 * conteúdo vira uma linha no rodapé (V6Fundamentos, átomo 3).
	 */
	const account = $derived(session.account);
	const route = $derived(
		session.summary?.routes.find((r) => r.active) ?? session.summary?.routes[0] ?? null
	);
	const guideId = $derived(route?.guide);
	const label = $derived(route ? routeLabel(route.guide, route.title) : '');

	const board = createResource(
		() => (account && guideId ? progressApi.board(guideId, account) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const cursorId = $derived(board.data?.cursor?.step_id);
	const boardById = $derived(new Map((board.data?.steps ?? []).map((s) => [s.id, s])));
	const refOf = (id: string) => boardById.get(id)?.ref ?? id;
	const sectionId = $derived(cursorId ? boardById.get(cursorId)?.section_id : undefined);
	const section = createResource(() =>
		guideId && sectionId ? progressApi.section(guideId, sectionId) : Promise.resolve(null)
	);
	const readiness = createResource(
		() =>
			account && guideId && cursorId
				? progressApi.readiness(guideId, account, cursorId).catch(() => null)
				: Promise.resolve(null),
		{ tags: ['progress'] }
	);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const runs = createResource(
		() => (account ? progressApi.syncRuns(account, 1).catch(() => null) : Promise.resolve(null)),
		{ tags: ['sync'] }
	);
	const events = createResource(
		() => (account && guideId ? progressApi.events(guideId, account, 60) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});
	const hiscoresAt = $derived.by(() => {
		const s = session.summary?.sources.find((x) => x.source === 'hiscores');
		return s ? Date.parse(s.last_observed_at) : undefined;
	});
	const band = $derived(sourceBand(runs.data?.runs[0], hiscoresAt, now));

	/* Metas fixadas com o plano: "14 de 31", o gargalo e onde esta rota chega. */
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	let plans = $state<Record<string, GoalPlan>>({});
	$effect(() => {
		const acc = account;
		if (!acc) return;
		for (const g of pinned.slice(0, 3)) {
			progressApi
				.goalPlan(acc, g.id)
				.then((r) => (plans = { ...plans, [g.id]: r.plan }))
				.catch(() => {});
		}
	});
	function goalLine(g: Goal): string {
		const plan = plans[g.id];
		const parts: string[] = [];
		const b = plan?.bottlenecks.find((x) => !x.met);
		if (b)
			parts.push(
				`Gargalo: ${b.target} ${b.skill}${b.level !== null ? ` (está em ${b.level})` : ''}.`
			);
		const here =
			plan?.routes.find((r) => r.guide_id === guideId && r.goal) ??
			g.routes.find((r) => r.guide === guideId);
		parts.push(here ? `Esta rota chega lá em ${here.ref}.` : 'Esta rota não conclui.');
		return parts.join(' ');
	}

	const step = $derived(section.data?.steps.find((s) => s.id === cursorId) ?? null);
	const segments = $derived(section.data ? segmentsOf(section.data) : []);
	const segOf = $derived(new Map(segments.flatMap((g) => g.steps.map((s) => [s.id, g] as const))));
	const segment = $derived(cursorId ? (segOf.get(cursorId) ?? null) : null);
	const mainSteps = $derived((board.data?.steps ?? []).filter((s) => s.main && s.visible));
	const cursorIndex = $derived(mainSteps.findIndex((s) => s.id === cursorId));
	const prev = $derived(
		cursorIndex > 0
			? mainSteps
					.slice(Math.max(0, cursorIndex - 2), cursorIndex)
					.filter((s) => s.status !== 'pending')
			: []
	);
	const paragraphRoute = $derived(guideId?.startsWith('bruhsailer') ?? false);
	const next = $derived.by(() => {
		if (!section.data || !step) return [];
		const steps = section.data.steps;
		const i = steps.findIndex((s) => s.id === step.id);
		return steps.slice(i + 1, i + (paragraphRoute ? 4 : 6));
	});
	/** Linha "banco · entre A e B" quando o próximo passo abre outro trecho de banco. */
	function bankBefore(i: number): string {
		const cur = i === 0 ? step : next[i - 1];
		const s = next[i];
		if (!cur || !s) return '';
		const a = segOf.get(cur.id);
		const b = segOf.get(s.id);
		if (!a || !b || a.id === b.id || b.kind !== 'bank_leg') return '';
		return `banco · entre ${cur.ref} e ${s.ref}`;
	}
	const sectionCounts = $derived(board.data?.sections.find((s) => s.id === sectionId)?.counts);
	const others = $derived(
		(session.summary?.routes ?? [])
			.filter((r) => r.guide !== guideId)
			.map((r) => ({ guide: r.guide, label: routeLabel(r.guide, r.title), route: r }))
	);
	const where = $derived.by(() => {
		if (!step) return '';
		const steps = section.data?.steps ?? [];
		const parts = [`${steps.findIndex((s) => s.id === step.id) + 1} de ${steps.length}`];
		if (step.location) parts.push(step.location);
		return parts.join(' · ');
	});
	const rotaBase = $derived(
		guideId && sectionId
			? resolve('/rotas/[guide]/[section]', { guide: guideId, section: sectionId })
			: resolve('/rotas')
	);
	const rotaHref = $derived(cursorId ? `${rotaBase}#passo-${cursorId}` : rotaBase);
	const recent = $derived(sessionLine(events.data, now));
	const bankActions = $derived(
		readiness.data?.current?.bank_actions?.length
			? readiness.data.current.bank_actions
			: (segment?.bankActions ?? [])
	);
	const bankFirst = $derived(segment?.steps[0]?.ref ?? '');
	const shortTitle = $derived(route ? routeTitle(route.guide, route.title) : '');

	/* Sincronizar */
	let syncing = $state(false);
	async function syncNow() {
		if (!account || syncing) return;
		syncing = true;
		failure = null;
		// Este sync mostra o próprio toast; o evento dele não repete a detecção.
		quietDetection('hiscores');
		try {
			const run = await progressApi.sync(account);
			toast =
				run.status === 'ok'
					? { title: 'Hiscores', text: `${run.steps_changed} passos mudaram`, undo: false }
					: run.status === 'unchanged'
						? { title: 'Hiscores', text: 'nada mudou desde o último sync', undo: false }
						: { title: 'Hiscores', text: run.error ?? 'falhou', undo: false };
			board.reload();
			events.reload();
			goals.reload();
			void loadSummary(account);
		} catch (e) {
			const tooSoon =
				e instanceof Error && 'status' in e && (e as { status: number }).status === 429;
			toast = {
				title: 'Hiscores',
				text: tooSoon ? 'aguarde: um sync por minuto, e nunca durante o recuo' : String(e),
				undo: false
			};
		} finally {
			syncing = false;
			runs.reload();
		}
	}

	/* Feito / Pular / Desfazer: marcas do dono pela API; o motor nunca as sobrescreve. */
	let busy = $state(false);
	let undo = $state<{ step: string; ref: string; prev?: Mark }[]>([]);
	let toast = $state<{ title: string; text: string; undo: boolean } | null>(null);
	let pinning = $state(false);
	let failure = $state<string | null>(null);

	async function write(run: () => Promise<Board>) {
		busy = true;
		failure = null;
		try {
			board.set(await run());
			events.reload();
			void loadSummary(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	function mark(kind: MarkAction) {
		if (!account || !guideId || !step || busy) return;
		batch = null;
		const current: BoardStep | undefined = boardById.get(step.id);
		const id = step.id;
		const ref = step.ref;
		undo = [...undo, { step: id, ref, prev: current?.mark }];
		toast = {
			title: kind === 'manual_done' ? 'Feito' : 'Pulado',
			text: `${ref} marcado`,
			undo: true
		};
		void write(() => progressApi.mark(guideId, id, account, kind));
	}

	function undoLast() {
		if (batch && batch.length && account && guideId) {
			const restore = batch;
			batch = null;
			toast = { title: 'Desfeito', text: `${restore.length} passos voltaram`, undo: false };
			void write(() => progressApi.restoreMarks(guideId, account, restore));
			return;
		}
		const last = undo[undo.length - 1];
		if (!last || !account || !guideId || busy) return;
		undo = undo.slice(0, -1);
		// Uma marca implied não volta pela API: desfazer limpa a marca e o motor decide.
		const kind: MarkAction =
			last.prev && last.prev.kind !== 'implied' ? (last.prev.kind as MarkAction) : 'clear';
		toast = { title: 'Desfeito', text: `${last.ref} voltou`, undo: false };
		void write(() => progressApi.mark(guideId, last.step, account, kind, last.prev?.note));
	}

	let batch = $state<MarkRestore[] | null>(null);
	function pin(stepId: string, before: boolean) {
		if (!account || !guideId) return;
		pinning = false;
		const ref = refOf(stepId);
		if (!before) {
			toast = { title: 'Fixado', text: `${ref} é o passo atual`, undo: false };
			void write(() => progressApi.pin(guideId, account, stepId));
			return;
		}
		void write(async () => {
			const b = await progressApi.markBefore(guideId, account, stepId);
			batch = b.undo ?? [];
			toast = {
				title: 'Marcados',
				text: `${batch.length} passos antes de ${ref}`,
				undo: batch.length > 0
			};
			return b;
		});
	}

	async function activate(guide: string) {
		if (!account) return;
		try {
			await progressApi.setActiveGuide(account, guide);
			void loadSummary(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		}
	}

	function keys(e: KeyboardEvent) {
		if (pinning || e.metaKey || e.ctrlKey || e.altKey) return;
		const t = e.target as HTMLElement | null;
		if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
		if (t?.closest('[role="dialog"]') || document.querySelector('[role="dialog"]')) return;
		if (e.key === 'j' || e.key === 'J') mark('manual_done');
		else if (e.key === 'k' || e.key === 'K') mark('skipped');
		else if (e.key === 'u' || e.key === 'U') undoLast();
	}
</script>

<svelte:window onkeydown={keys} />
<svelte:head>
	<title>Agora{label ? ` · ${label}` : ''} · Chronicle</title>
</svelte:head>

{#snippet bank()}
	{#if segment}
		<BankPanel
			readiness={readiness.data?.current ?? null}
			ahead={readiness.data?.ahead ?? null}
			{bankActions}
			firstRef={bankFirst}
			{refOf}
		/>
	{/if}
{/snippet}

{#snippet nextList()}
	{#if next.length}
		<section class="next">
			<div class="nh">
				<h2 class="lbl">Em seguida</h2>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- rotaHref = resolve() + âncora -->
				<a href={rotaHref} class="push small"
					>{paragraphRoute ? 'abrir o capítulo' : 'abrir a seção'}</a
				>
			</div>
			{#each next as s, i (s.id)}
				{@const b = boardById.get(s.id)}
				{@const v = statusView(b?.status)}
				{@const rc = rowChip(chipsOf(s, b))}
				{@const line = bankBefore(i)}
				{@const items = metaOf(s).find((m) => m.label === 'itens')}
				{#if line}<div class="bankline">{line}</div>{/if}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- rotaBase vem de resolve() -->
				<a class="row" href={`${rotaBase}#passo-${s.id}`}>
					<span class="st {v.st}"
						>{#if v.done}<Glyph name="check" size={10} />{/if}</span
					><span class="id" class:wid={paragraphRoute}>{s.ref}</span>
					<span class="tx">{b?.title ?? s.text}</span>
					{#if rc.chip}
						<ObjectiveChip chip={rc.chip} small />
						{#if rc.more}<span class="more">+{rc.more}</span>{/if}
					{:else if s.location}<span class="loc">{s.location}</span>{/if}
					{#if items}<span class="more" title={items.value}
							>{items.value.split(',').length} itens</span
						>{/if}
				</a>
			{/each}
		</section>
	{/if}
{/snippet}

{#snippet footLine()}
	<div class="foot" class:col={!narrow.value}>
		{#if pinned.length === 0}
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + aba Metas -->
			<a href={resolve('/mundo/metas')}>Nenhuma meta fixada</a>
		{/if}
		<span>Nesta sessão: {recent || 'nada mudou'}</span>
		<span>Microbot desligado</span>
	</div>
{/snippet}

{#if !account}
	<div class="band">
		<Glyph name="alert" /><span>Sem conta, nada é verificado nem gravado.</span>
		<a class="btn btn-s push" href={resolve('/setup')}>Primeiro acesso</a>
	</div>
{:else if session.error || board.error}
	<div class="band" role="alert">
		<Glyph name="alert" />
		<span>Não deu para carregar o progresso: {(session.error ?? board.error)?.message}</span>
		<button
			class="btn btn-s push"
			type="button"
			onclick={() => (loadSummary(account), board.reload())}>Tentar de novo</button
		>
	</div>
{:else if !route || !board.data}
	<p class="su" aria-busy="true">Carregando a rota ativa…</p>
{:else}
	{#if band}
		<div class="band" role="alert">
			<Glyph name={band.kind === 'failed' ? 'alert' : 'clock'} />
			<span>{band.text}</span>
			<button class="btn btn-s push" type="button" disabled={syncing} onclick={syncNow}
				>{band.kind === 'failed' ? 'Tentar agora' : 'Sincronizar agora'}</button
			>
			<a class="btn btn-g btn-s" href={resolve('/conta')}>Fontes</a>
		</div>
	{/if}

	<header class="top">
		<a class="sel" href={resolve('/rotas')} title={`${route.title} · trocar rota`}
			>{narrow.value ? label : shortTitle} <small>▾{narrow.value ? '' : ' rota ativa'}</small></a
		>
		<div class="sec">
			{#if section.data}
				<span class="stitle" class:clip={narrow.value}
					><span class="mono su tiny">{sectionLabel(section.data.id)}</span>&nbsp; {section.data
						.title}{#if section.data.description && !narrow.value}
						<span class="su"> · {section.data.description}</span>{/if}</span
				>
			{/if}
			{#if sectionCounts}
				<div class="prog">
					<div class="xp">
						<span style="width: {pct(sectionCounts.done, sectionCounts.total)}%"></span>
					</div>
					<span class="mono su tiny"
						>{sectionCounts.done} / {sectionCounts.total}{narrow.value ? '' : ' feitos'}</span
					>
				</div>
			{/if}
		</div>
		<button class="btn btn-s" type="button" disabled={syncing} onclick={syncNow}
			><Glyph name="sync" size={14} />{syncing ? 'Sincronizando…' : 'Sincronizar'}</button
		>
	</header>

	{#if failure}
		<div class="band" role="alert">
			<Glyph name="alert" /><span>Não foi gravado: {failure}</span>
		</div>
	{/if}

	{#if !cursorId}
		<section class="panel">
			<div class="plaque">Rota concluída</div>
			<p class="mu pad">
				Todos os passos da rota principal estão feitos, presumidos ou pulados.
				<button class="btn btn-s" type="button" onclick={() => (pinning = true)}
					>Fixar um passo</button
				>
			</p>
		</section>
	{:else}
		<div class="grid">
			<div class="mainc">
				{#if step}
					<CurrentStep
						{step}
						board={boardById.get(step.id)}
						{prev}
						prevWho={(id) => whoDid(events.data, id, now)}
						{where}
						{others}
						{rotaHref}
						{busy}
						narrow={narrow.value}
						canUndo={undo.length > 0 || !!batch?.length}
						ondone={() => mark('manual_done')}
						onskip={() => mark('skipped')}
						onundo={undoLast}
						onpin={() => (pinning = true)}
					/>
				{:else}
					<p class="su" aria-busy="true">Carregando o passo atual…</p>
				{/if}
				{#if narrow.value}{@render bank()}{/if}
				{@render nextList()}
				{#if narrow.value}{@render footLine()}{/if}
			</div>

			{#if !narrow.value}
				<aside class="side-c">
					{@render bank()}
					{#if pinned.length}
						<section class="panel">
							<div class="plaque">Metas fixadas<span class="r">{pinned.length}</span></div>
							<div class="goals">
								{#each pinned as g (g.id)}
									{@const plan = plans[g.id]}
									{@const icon = iconFor(g.ref)}
									<button class="goal" type="button" onclick={() => openEntity(g.ref)}>
										<span class="gt"
											>{#if icon}<img class="ski" src={icon} alt="" />{/if}<span class="gn"
												>{g.label}</span
											>{#if plan?.requirements}<span class="mono su tiny push"
													>{plan.met} de {plan.total}</span
												>{/if}</span
										>
										{#if plan?.requirements}<span class="xp"
												><span style="width: {pct(plan.met, plan.total)}%"></span></span
											>{/if}
										<span class="su small">{goalLine(g)}</span>
									</button>
								{/each}
							</div>
						</section>
					{/if}
					{#if others.length}
						<section class="panel">
							<div class="plaque">Outras rotas<span class="r">cursor guardado</span></div>
							<div class="recs">
								{#each others as o (o.guide)}
									<button
										class="rec"
										type="button"
										title="Tornar {o.label} a rota ativa"
										onclick={() => activate(o.guide)}
										><span class="st st-cur dotc"></span>{o.label} · {o.route.cursor_ref ??
											'fim'}<span class="w"
											>{o.route.counts.done} / {o.route.counts.total} · tornar ativa</span
										></button
									>
								{/each}
							</div>
						</section>
					{/if}
					{@render footLine()}
				</aside>
			{/if}
		</div>
	{/if}

	<footer class="su tiny">
		{#if route.guide.startsWith('oziris')}Guia por OzirisLoL, versão interativa de ironman.guide.{:else}BRUHsailer
			por So Iron BRUH e ParasailerOSRS.{/if}
		Uso pessoal; atribuição mantida. {ICON_ATTRIBUTION}.
	</footer>
{/if}

{#if pinning && board.data}
	<PinDialog
		guideLabel={label}
		steps={mainSteps}
		{cursorId}
		onpin={pin}
		onclose={() => (pinning = false)}
	/>
{/if}
{#if toast}
	<Toast
		title={toast.title}
		text={toast.text}
		action={toast.undo ? 'desfazer' : undefined}
		onaction={undoLast}
		onclose={() => (toast = null)}
	/>
{/if}

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px 20px;
	}
	.sec {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1 1 320px;
		min-width: 0;
	}
	.stitle {
		font-size: 13px;
	}
	.stitle.clip {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.prog {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.prog .xp {
		flex: 1;
		max-width: 360px;
	}
	.grid {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.mainc {
		flex: 999 1 460px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.side-c {
		flex: 1 1 360px;
		max-width: 440px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.next {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.nh {
		display: flex;
		align-items: baseline;
		gap: 12px;
		padding: 0 12px 4px;
	}
	.id.wid {
		width: 56px;
	}
	.goals {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	button.goal {
		border: 0;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.gt {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.gn {
		font-weight: 600;
	}
	.recs {
		padding: 6px 14px 8px;
		display: flex;
		flex-direction: column;
	}
	button.rec {
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		font: inherit;
		font-size: 13px;
		color: var(--ink);
		cursor: pointer;
		text-align: left;
		width: 100%;
	}
	.dotc {
		width: 12px;
		height: 12px;
		box-shadow: inset 0 0 0 2px var(--surface);
	}
	.foot.col {
		flex-direction: column;
		gap: 6px;
	}
	.foot:not(.col) {
		padding-top: 6px;
		border-top: 1px solid var(--hairline);
	}
	.pad {
		padding: 12px 16px;
		margin: 0;
	}
	.push {
		margin-left: auto;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	@media (max-width: 900px) {
		.mainc {
			gap: 14px;
		}
	}
</style>
