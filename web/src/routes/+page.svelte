<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { ICON_ATTRIBUTION } from '#lib/progress/icons';
	import {
		chipsOf,
		eventLines,
		pct,
		routeLabel,
		routesLine,
		sectionLabel,
		sourceBand,
		segmentsOf,
		statusView
	} from '#lib/progress/view';
	import type { Board, BoardStep, Mark, MarkAction } from '#lib/progress/types';
	import BankPanel from '#lib/v4/BankPanel.svelte';
	import CurrentStep from '#lib/v4/CurrentStep.svelte';
	import Glyph from '#lib/v4/Glyph.svelte';
	import ObjectiveChip from '#lib/v4/ObjectiveChip.svelte';
	import PinDialog from '#lib/v4/PinDialog.svelte';
	import Toast from '#lib/v4/Toast.svelte';

	/*
	 * Agora (V4AgoraOziris / V4AgoraBruh): o cursor da rota ativa como card, o banco do trecho
	 * em três colunas, os próximos passos do trecho e o que mudou na sessão. Painéis sem fonte
	 * de dados (metas fixadas, fora da rota ativa, recorrentes, ⌘K) entram nos itens seguintes.
	 */
	const account = $derived(session.account);
	const route = $derived(
		session.summary?.routes.find((r) => r.active) ?? session.summary?.routes[0] ?? null
	);
	const guideId = $derived(route?.guide);

	const board = createResource(() =>
		account && guideId ? progressApi.board(guideId, account) : Promise.resolve(null)
	);
	const cursorId = $derived(board.data?.cursor?.step_id);
	const boardById = $derived(new Map((board.data?.steps ?? []).map((s) => [s.id, s])));
	const sectionId = $derived(cursorId ? boardById.get(cursorId)?.section_id : undefined);
	const section = createResource(() =>
		guideId && sectionId ? progressApi.section(guideId, sectionId) : Promise.resolve(null)
	);
	const readiness = createResource(() =>
		account && guideId && cursorId
			? progressApi.readiness(guideId, account, cursorId).catch(() => null)
			: Promise.resolve(null)
	);
	const goals = createResource(() =>
		account ? progressApi.goals(account) : Promise.resolve(null)
	);
	const runs = createResource(() =>
		account ? progressApi.syncRuns(account, 1).catch(() => null) : Promise.resolve(null)
	);
	const hiscoresAt = $derived.by(() => {
		const s = session.summary?.sources.find((x) => x.source === 'hiscores');
		return s ? Date.parse(s.last_observed_at) : undefined;
	});
	const band = $derived(sourceBand(runs.data?.runs[0], hiscoresAt, Date.now()));
	let syncing = $state(false);
	async function syncNow() {
		if (!account || syncing) return;
		syncing = true;
		failure = null;
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
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));
	const events = createResource(() =>
		account && guideId ? progressApi.events(guideId, account, 30) : Promise.resolve(null)
	);

	const step = $derived(section.data?.steps.find((s) => s.id === cursorId) ?? null);
	const segments = $derived(section.data ? segmentsOf(section.data) : []);
	const segment = $derived(segments.find((g) => g.steps.some((s) => s.id === cursorId)) ?? null);
	const mainSteps = $derived((board.data?.steps ?? []).filter((s) => s.main && s.visible));
	const cursorIndex = $derived(mainSteps.findIndex((s) => s.id === cursorId));
	const prev = $derived(
		cursorIndex > 0
			? mainSteps
					.slice(Math.max(0, cursorIndex - 2), cursorIndex)
					.filter((s) => s.status !== 'pending')
			: []
	);
	const next = $derived.by(() => {
		if (!section.data || !step) return [];
		const pool = segment && segment.steps.length > 1 ? segment.steps : section.data.steps;
		const i = pool.findIndex((s) => s.id === step.id);
		return pool.slice(i + 1, i + 7);
	});
	const sectionCounts = $derived(board.data?.sections.find((s) => s.id === sectionId)?.counts);
	const others = $derived(
		(session.summary?.routes ?? [])
			.filter((r) => r.guide !== guideId)
			.map((r) => ({ guide: r.guide, label: routeLabel(r.guide, r.title) }))
	);
	const where = $derived.by(() => {
		if (!step) return '';
		const steps = section.data?.steps ?? [];
		const parts = [`${steps.findIndex((s) => s.id === step.id) + 1} de ${steps.length}`];
		if (step.location) parts.push(step.location);
		if (segment?.kind === 'bank_leg') parts.push(`trecho de banco ${segment.range}`);
		return parts.join(' · ');
	});
	const lines = $derived(
		events.data
			? eventLines(events.data, (id) => boardById.get(id)?.ref ?? id, section.data?.names ?? {})
			: []
	);
	const rotaBase = $derived(
		guideId && sectionId
			? resolve('/rotas/[guide]/[section]', { guide: guideId, section: sectionId })
			: resolve('/rotas')
	);
	const rotaHref = $derived(cursorId ? `${rotaBase}#passo-${cursorId}` : rotaBase);

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
			const ids = batch;
			batch = null;
			toast = { title: 'Desfeito', text: `${ids.length} passos voltaram`, undo: false };
			void write(() => progressApi.clearMarks(guideId, account, ids));
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

	let batch = $state<string[] | null>(null);
	function pin(stepId: string, before: boolean) {
		if (!account || !guideId) return;
		pinning = false;
		const ref = boardById.get(stepId)?.ref ?? stepId;
		if (!before) {
			toast = { title: 'Fixado', text: `${ref} é o passo atual`, undo: false };
			void write(() => progressApi.pin(guideId, account, stepId));
			return;
		}
		void write(async () => {
			const b = await progressApi.markBefore(guideId, account, stepId);
			batch = b.marked ?? [];
			toast = {
				title: 'Marcados',
				text: `${batch.length} passos antes de ${ref}`,
				undo: batch.length > 0
			};
			return b;
		});
	}

	function keys(e: KeyboardEvent) {
		if (pinning || e.metaKey || e.ctrlKey || e.altKey) return;
		const t = e.target as HTMLElement | null;
		if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
		if (e.key === 'j' || e.key === 'J') mark('manual_done');
		else if (e.key === 'k' || e.key === 'K') mark('skipped');
		else if (e.key === 'u' || e.key === 'U') undoLast();
	}
</script>

<svelte:window onkeydown={keys} />
<svelte:head>
	<title>Agora{route ? ` · ${routeLabel(route.guide, route.title)}` : ''} · Ironman Planner</title>
</svelte:head>

{#if !account}
	<div class="band">
		<Glyph name="alert" />
		<span>
			Sem conta, nada é verificado nem gravado. Crie uma com <code
				>just account-create &lt;rsn&gt;</code
			>
			e abra com <code>?account=acc_&lt;rsn&gt;</code> (modo dev).
		</span>
		<a class="btn btn-s push" href={resolve('/rotas')}>Ver as rotas</a>
	</div>
{:else if session.error || board.error}
	<div class="band">
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
	<header class="top">
		<a class="sel" href={resolve('/rotas')}
			>{route.title} <small>▾ {route.active ? 'rota ativa' : 'rota'}</small></a
		>
		<div class="sec">
			{#if section.data}
				<span class="small"
					><span class="mono su tiny">{sectionLabel(section.data.id)}</span> &nbsp;{section.data
						.title}</span
				>
			{/if}
			{#if sectionCounts}
				<div class="prog">
					<div class="xp">
						<span style="width: {pct(sectionCounts.done, sectionCounts.total)}%"></span>
					</div>
					<span class="mono su tiny">{sectionCounts.done} / {sectionCounts.total} feitos</span>
				</div>
			{/if}
		</div>
		<div class="acts">
			<button class="btn btn-s" type="button" disabled={syncing} onclick={syncNow}
				>{syncing ? 'Sincronizando…' : 'Sincronizar'}</button
			>
			<a class="btn btn-g btn-s" href={resolve('/rotas')}>Trocar rota</a>
		</div>
	</header>

	{#if band}
		<div class="band" class:failed={band.kind === 'failed'}>
			<Glyph name={band.kind === 'failed' ? 'alert' : 'clock'} />
			<span>{band.text}</span>
			<button class="btn btn-s push" type="button" disabled={syncing} onclick={syncNow}
				>{band.kind === 'failed' ? 'Tentar agora' : 'Sincronizar agora'}</button
			>
		</div>
	{/if}

	{#if failure}
		<div class="band"><Glyph name="alert" /><span>A marca não foi gravada: {failure}</span></div>
	{/if}

	{#if !cursorId}
		<section class="panel done">
			<div class="plaque">Rota concluída</div>
			<p class="mu pad">
				Todos os passos da rota principal estão feitos, presumidos ou pulados. Fixe um passo na rota
				para voltar a ele.
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
						{where}
						{others}
						{rotaHref}
						{busy}
						canUndo={undo.length > 0 || !!batch?.length}
						ondone={() => mark('manual_done')}
						onskip={() => mark('skipped')}
						onundo={undoLast}
						onpin={() => (pinning = true)}
					/>
				{:else}
					<p class="su" aria-busy="true">Carregando o passo atual…</p>
				{/if}

				{#if segment && (segment.bankActions.length || segment.loadout?.inventory?.length || segment.loadout?.equipment?.length)}
					<BankPanel
						readiness={readiness.data?.current ?? null}
						range={segment.range}
						loadout={segment.loadout}
						bankActions={segment.bankActions}
						firstRef={segment.steps[0]?.ref ?? ''}
					/>
				{/if}

				{#if next.length}
					<section class="next">
						<div class="nh">
							<h2 class="ey">
								Em seguida{segment && segment.steps.length > 1 ? ' · mesmo trecho' : ''}
							</h2>
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- rotaHref = resolve() + âncora -->
							<a href={rotaHref} class="push small">abrir a seção</a>
						</div>
						<div class="rows">
							{#each next as s (s.id)}
								{@const b = boardById.get(s.id)}
								{@const v = statusView(b?.status)}
								{@const rowChips = chipsOf(s, b).filter((c) => c.role !== 'uses')}
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- rotaBase vem de resolve() -->
								<a class="row" href={`${rotaBase}#passo-${s.id}`}>
									<span class="st {v.st}"></span><span class="id">{s.ref}</span>
									<span class="tx">{b?.title ?? s.text}</span>
									{#if s.layers?.length === 1 && s.layers[0] === 'enhanced'}<span class="badge"
											>enhanced</span
										>{/if}
									{#each rowChips.slice(0, 2) as c (c.role + c.key)}
										<ObjectiveChip chip={c} small hint={false} link={false} />
									{/each}
									{#if rowChips.length > 2}<span class="badge">+{rowChips.length - 2}</span>{/if}
									{#if s.location}<span class="loc">{s.location}</span>{/if}
								</a>
							{/each}
						</div>
					</section>
				{/if}
			</div>

			<aside class="side-c">
				<section class="panel">
					<div class="plaque">Metas fixadas<span class="r">{pinned.length}</span></div>
					<div class="pad list">
						{#each pinned as g (g.id)}
							{@const here = g.routes.find((r) => r.guide === guideId)}
							{@const elsewhere = g.routes.filter((r) => r.guide !== guideId)}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da entidade -->
							<a class="goal" href={`${resolve('/mundo')}?e=${encodeURIComponent(g.ref)}`}>
								<span class="sr gname">{g.label}</span>
								<span class="su small"
									>{here ? `nesta rota: ${here.ref}` : 'não está nesta rota'}{elsewhere.length
										? ` · ${routesLine(elsewhere, (id) => routeLabel(id, id))}`
										: ''}</span
								>
							</a>
						{:else}
							<span class="su small">Nenhuma meta. Fixe uma entidade no Mundo.</span>
						{/each}
					</div>
				</section>
				<section class="panel">
					<div class="plaque">Nesta sessão<span class="r">{lines.length} mudanças</span></div>
					<div class="pad list">
						{#each lines as l, i (i)}
							<div class="rec">
								{#if l.st === 'badge'}
									<span class="badge badge-auto">{l.right.split(' · ')[0] || 'fonte'}</span>
								{:else}
									<span class="st {l.st} dot12"></span>
								{/if}
								{l.text}<span class="w"
									>{l.st === 'badge' ? l.right.split(' · ').slice(1).join(' · ') : l.right}</span
								>
							</div>
						{:else}
							<span class="su small"
								>Nada mudou ainda nesta rota. Marque um passo ou sincronize uma fonte.</span
							>
						{/each}
					</div>
				</section>
				<section class="panel">
					<div class="plaque off">Microbot<span class="r">desligado</span></div>
					<div class="pad">
						<span class="su small"
							>Controle do cliente desligado (MICROBOT_CONTROL=off). O plugin de leitura continua
							enviando dados.</span
						>
					</div>
				</section>
			</aside>
		</div>
	{/if}

	<footer class="su tiny">
		{#if route.guide.startsWith('oziris')}Guia por OzirisLoL, versão interativa de ironman.guide.{:else}Guia
			por So Iron BRUH e ParasailerOSRS.{/if}
		Uso pessoal; atribuição mantida. {ICON_ATTRIBUTION}.
	</footer>
{/if}

{#if pinning && board.data}
	<PinDialog
		guideLabel={route ? routeLabel(route.guide, route.title) : ''}
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
	.prog {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.prog .xp {
		flex: 1;
		max-width: 360px;
	}
	.acts {
		display: flex;
		gap: 8px;
		align-items: center;
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
		flex: 1 1 300px;
		max-width: 340px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.next {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.nh {
		display: flex;
		align-items: baseline;
		gap: 12px;
		padding: 0 12px;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.pad {
		padding: 10px 12px;
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.band.failed {
		border-color: var(--not-started);
	}
	.band.failed :global(svg) {
		color: var(--not-started);
	}
	.goal {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 10px;
		border-radius: 4px;
		background: var(--elevated);
		box-shadow: var(--bevel);
		text-decoration: none;
		color: var(--ink);
	}
	.gname {
		font-size: 15px;
	}
	.dot12 {
		width: 12px;
		height: 12px;
	}
	.off {
		color: var(--on-frame-muted);
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
	.done {
		display: flex;
		flex-direction: column;
	}
	.done p {
		margin: 0;
	}
	code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}
</style>
