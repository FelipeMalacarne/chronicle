<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { rowChip } from '#lib/progress/agora';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { bankMarks, headerFacts } from '#lib/progress/rota';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import {
		chipsOf,
		routeLabel,
		sectionLabel,
		segmentsOf,
		sourceBand,
		statusView
	} from '#lib/progress/view';
	import type { Board, BoardStep, Goal, GoalPlan, MarkAction } from '#lib/progress/types';
	import {
		boardIn,
		popUndo,
		restoreAction,
		undoable,
		type Scope,
		type UndoEntry
	} from '#lib/progress/undo';
	import BankPanel from '#lib/v6/BankPanel.svelte';
	import ClocksCard from '#lib/v6/ClocksCard.svelte';
	import CurrentStep from '#lib/v6/CurrentStep.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import PinDialog from '#lib/v6/PinDialog.svelte';
	import RouteBar from '#lib/v6/RouteBar.svelte';
	import Toast from '#lib/v6/Toast.svelte';

	/*
	 * Conta › Rota › Passo (página 8 do canvas, V7Rota1440 e V7Rota640): a barra da rota, o passo
	 * atual com os objetivos que mostram quanto falta e o passo a passo com caixas, "Em seguida" com
	 * a marca de banco na linha e, ao lado, Levar do banco, Relógios, Aqui perto e Metas. A 640 o
	 * banco vem logo depois do passo e os relógios no fim; Aqui perto e Metas ficam na 1440, como
	 * no artboard.
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
	/* Conta e rota atuais. O board de outra rota (ainda na tela enquanto o novo carrega) nunca
	 * alimenta seção, banco nem escritas: ids de passo se repetem entre guias. */
	const scope = $derived(account && guideId ? { account, guide: guideId } : null);
	const boardData = $derived(boardIn(board.data, scope));
	const cursorId = $derived(boardData?.cursor?.step_id);
	const boardById = $derived(new Map((boardData?.steps ?? []).map((s) => [s.id, s])));
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
	/* Relógios (decisão 31 do doc 01): os do Time Tracking que o plugin manda. */
	const timers = createResource(
		() => (account ? progressApi.timers(account).catch(() => null) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	/* Aqui perto (item 16): os passos abertos da rota perto da última posição que o plugin mandou
	 * (opt-in "Send position" no Chronicle Link 1.8). Sem posição, o cartão não aparece. */
	const nearby = createResource(
		() =>
			account && guideId
				? progressApi.nearby(guideId, account).catch(() => null)
				: Promise.resolve(null),
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

	const nearRows = $derived(
		guideId
			? (nearby.data?.steps ?? []).map((s) => ({
					...s,
					href: `${resolve('/rotas/[guide]/[section]', { guide: guideId, section: s.section_id })}#passo-${s.id}`
				}))
			: []
	);

	/* Metas fixadas com o plano: onde esta rota chega e a quantos passos. */
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
	const mainSteps = $derived((boardData?.steps ?? []).filter((s) => s.main && s.visible));
	const cursorIndex = $derived(mainSteps.findIndex((s) => s.id === cursorId));
	function goalLine(g: Goal): string {
		const plan = plans[g.id];
		const here =
			plan?.routes?.find((r) => r.guide_id === guideId && r.goal) ??
			g.routes.find((r) => r.guide === guideId);
		if (!here) return 'Esta rota não chega lá.';
		const at = mainSteps.findIndex((s) => s.ref === here.ref);
		const dist = at > cursorIndex && cursorIndex >= 0 ? at - cursorIndex : 0;
		return `Esta rota chega lá em ${here.ref}${dist ? `, daqui a ${dist} passos` : ''}.`;
	}

	const step = $derived(section.data?.steps.find((s) => s.id === cursorId) ?? null);
	const segments = $derived(section.data ? segmentsOf(section.data) : []);
	const marks = $derived(bankMarks(segments));
	const segOf = $derived(new Map(segments.flatMap((g) => g.steps.map((s) => [s.id, g] as const))));
	const segment = $derived(cursorId ? (segOf.get(cursorId) ?? null) : null);
	const paragraphRoute = $derived(guideId?.startsWith('bruhsailer') ?? false);
	const NEXT = 3;
	const after = $derived.by(() => {
		if (!section.data || !step) return [];
		const steps = section.data.steps;
		return steps.slice(steps.findIndex((s) => s.id === step.id) + 1);
	});
	const next = $derived(after.slice(0, paragraphRoute ? NEXT : NEXT + 2));
	const sectionCounts = $derived(boardData?.sections.find((s) => s.id === sectionId)?.counts);
	const others = $derived(
		(session.summary?.routes ?? [])
			.filter((r) => r.guide !== guideId)
			.map((r) => ({ guide: r.guide, label: routeLabel(r.guide, r.title) }))
	);
	const where = $derived.by(() => {
		if (!step || !section.data) return '';
		const steps = section.data.steps;
		const n = `${steps.findIndex((s) => s.id === step.id) + 1} de ${steps.length}`;
		return narrow.value ? '' : `${n} ${paragraphRoute ? 'no capítulo' : 'na seção'}`;
	});
	// Sem as notas do BRUHsailer (a Oziris), a 1440 diz onde o passo é.
	const facts = $derived(
		step ? headerFacts(step, narrow.value) || (narrow.value ? '' : (step.location ?? '')) : ''
	);
	const rotaBase = $derived(
		guideId && sectionId
			? resolve('/rotas/[guide]/[section]', { guide: guideId, section: sectionId })
			: resolve('/rotas')
	);
	const bankActions = $derived(
		readiness.data?.current?.bank_actions?.length
			? readiness.data.current.bank_actions
			: (segment?.bankActions ?? [])
	);
	const bankFirst = $derived(segment?.steps[0]?.ref ?? '');
	const capOf = $derived(
		section.data && sectionCounts
			? {
					label: sectionLabel(section.data.id),
					title: section.data.title,
					done: sectionCounts.done,
					total: sectionCounts.total
				}
			: null
	);
	const reloadAll = () => {
		board.reload();
		goals.reload();
		timers.reload();
	};

	/* Feito / Pular / Desfazer: marcas do dono pela API; o motor nunca as sobrescreve.
	 * Uma escrita por vez, sempre com a conta e a rota em que foi pedida; o desfazer só entra na
	 * pilha depois que a escrita deu certo, só vale nesse mesmo escopo e revoga o comando que
	 * desfaz (mark_commands, docs/10 seção 6). */
	let busy = $state(false);
	let undo = $state<UndoEntry[]>([]);
	let toast = $state<{ title: string; text: string; undo: boolean } | null>(null);
	let pinning = $state(false);
	let failure = $state<string | null>(null);
	const canUndo = $derived(undoable(undo, scope) > 0);

	/** Roda uma escrita do dono; devolve o board novo, ou null se recusada ou falhou. */
	async function write(at: Scope, run: () => Promise<Board>): Promise<Board | null> {
		if (busy) return null;
		busy = true;
		failure = null;
		try {
			const b = await run();
			// A rota pode ter mudado enquanto a escrita ia: o board dela não entra na tela atual.
			if (boardIn(b, scope)) board.set(b);
			else board.reload();
			void loadSummary(at.account);
			return b;
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
			return null;
		} finally {
			busy = false;
		}
	}

	async function mark(kind: MarkAction) {
		const at = scope;
		if (!at || !step || busy) return;
		const id = step.id;
		const ref = step.ref;
		const prev: BoardStep['mark'] = boardById.get(id)?.mark;
		const done = await write(at, () => progressApi.mark(at.guide, id, at.account, kind));
		if (!done) return;
		undo = [...undo, { kind: 'mark', scope: at, step: id, ref, prev, command: done.command_id }];
		toast = {
			title: kind === 'manual_done' ? 'Feito' : 'Pulado',
			text: `${ref} marcado`,
			undo: true
		};
	}

	async function undoLast() {
		const at = scope;
		if (!at || busy) return;
		const { entry, rest } = popUndo(undo, at);
		if (!entry) return;
		const done =
			entry.kind === 'batch'
				? await write(at, () =>
						progressApi.restoreMarks(at.guide, at.account, entry.restore, entry.command)
					)
				: await write(at, () => {
						const r = restoreAction(entry.prev);
						return progressApi.mark(
							at.guide,
							entry.step,
							at.account,
							r.kind,
							r.note,
							entry.command
						);
					});
		if (!done) return;
		undo = rest;
		toast = {
			title: 'Desfeito',
			text:
				entry.kind === 'batch' ? `${entry.restore.length} passos voltaram` : `${entry.ref} voltou`,
			undo: false
		};
	}

	async function pin(stepId: string, before: boolean) {
		const at = scope;
		if (!at) return;
		pinning = false;
		const ref = refOf(stepId);
		if (!before) {
			if (await write(at, () => progressApi.pin(at.guide, at.account, stepId)))
				toast = { title: 'Fixado', text: `${ref} é o passo atual`, undo: false };
			return;
		}
		const b = await write(at, () => progressApi.markBefore(at.guide, at.account, stepId));
		if (!b) return;
		const restore = b.undo ?? [];
		if (restore.length)
			undo = [...undo, { kind: 'batch', scope: at, restore, command: b.command_id }];
		toast = {
			title: 'Marcados',
			text: `${restore.length} passos antes de ${ref}`,
			undo: restore.length > 0
		};
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
	<title>Rota{label ? ` · ${label}` : ''} · Chronicle</title>
</svelte:head>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs internos vêm de resolve() + âncora -->

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

{#snippet clocks()}
	<ClocksCard timers={timers.data?.timers ?? []} {now} short={narrow.value} />
{/snippet}

{#snippet nextList()}
	{#if next.length}
		<section aria-label="Em seguida">
			<div class="rh">
				<h2>Em seguida</h2>
				<span>{next.length} de {after.length}</span>
			</div>
			{#each next as s (s.id)}
				{@const b = boardById.get(s.id)}
				{@const v = statusView(b?.status)}
				{@const rc = rowChip(chipsOf(s, b))}
				{@const bm = marks.get(s.id)}
				{@const icon = rc.chip ? iconFor(rc.chip.ref) : undefined}
				<a class="nx7" href={`${rotaBase}#passo-${s.id}`}>
					<span class="st {v.st}"
						>{#if v.done}<Glyph name="check" size={10} />{/if}</span
					><span class="id">{s.ref}</span><span class="tx">{b?.title ?? s.text}</span><span
						class="rt"
						>{#if bm}<span class="bank7" title={bm.title}
								><Glyph name="bank" size={11} />{bm.label}</span
							>{/if}{#if icon}<img
								src={icon}
								alt={rc.chip?.label}
								title={rc.chip?.label}
							/>{/if}{#if rc.more}+{rc.more}{/if}</span
					>
				</a>
			{/each}
			<!-- V7Rota640: o botão da barra já leva à rota inteira. -->
			{#if !narrow.value}<a class="lnk more" href={`${rotaBase}#passo-${cursorId}`}
					>ver a rota inteira</a
				>{/if}
		</section>
	{/if}
{/snippet}

{#if !account}
	<div class="band">
		<Glyph name="alert" /><span>Sem conta, nada é verificado nem gravado.</span>
		<a class="btn btn-s push" href={resolve('/contas/nova')}>Nova conta</a>
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
{:else if !route || !boardData}
	<p class="su" aria-busy="true">Carregando a rota ativa…</p>
{:else}
	<RouteBar view="passo" guide={guideId} section={capOf} onsynced={reloadAll} />

	{#if band}
		<div class="band" role="alert">
			<Glyph name={band.kind === 'failed' ? 'alert' : 'clock'} />
			<span>{band.text}</span>
			<a class="btn btn-g btn-s push" href={resolve('/conta')}>Personagem</a>
		</div>
	{/if}

	{#if failure}
		<div class="band" role="alert">
			<Glyph name="alert" /><span>Não foi gravado: {failure}</span>
		</div>
	{/if}

	{#if !cursorId}
		<div class="empty7">
			<h3>Rota concluída</h3>
			<p>Todos os passos da rota principal estão feitos, presumidos ou pulados.</p>
			<button class="btn btn-s" type="button" onclick={() => (pinning = true)}
				>Fixar um passo</button
			>
		</div>
	{:else}
		<div class="grid">
			<div class="mainc">
				{#if step}
					<CurrentStep
						{step}
						board={boardById.get(step.id)}
						{where}
						{facts}
						{others}
						{busy}
						narrow={narrow.value}
						{canUndo}
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
				{#if narrow.value}{@render clocks()}{/if}
			</div>

			{#if !narrow.value}
				<aside class="side7">
					{@render bank()}
					{@render clocks()}
					{#if nearby.data?.position}
						{@const pos = nearby.data.position}
						<section class="card">
							<div class="h">
								<h2>Aqui perto</h2>
								<span title="{pos.x}, {pos.y}, {pos.plane} · região {pos.region}"
									>mundo {pos.world}</span
								>
							</div>
							<div class="b">
								{#if nearRows.length}
									<div>
										{#each nearRows as s (s.id)}
											<a class="cl7 near7" href={s.href} title={s.title}
												><span class="mono su">{s.ref}</span><span>{s.title}</span><span class="w"
													>{s.distance} tiles</span
												></a
											>
										{/each}
									</div>
								{:else}
									<p class="mu">Nada aberto da rota a {nearby.data.radius} tiles daqui.</p>
								{/if}
							</div>
						</section>
					{/if}
					{#if pinned.length}
						<section class="card">
							<div class="h">
								<h2>Metas</h2>
								<span>{pinned.length} {pinned.length === 1 ? 'fixada' : 'fixadas'}</span>
							</div>
							<div class="b metas">
								{#each pinned as g (g.id)}
									{@const icon = iconFor(g.ref)}
									<button class="meta7" type="button" onclick={() => openEntity(g.ref)}
										>{#if icon}<img src={icon} alt="" />{/if}<b>{g.label}</b></button
									>
									<p class="mu small">{goalLine(g)}</p>
								{/each}
								<a class="lnk lnk0" href={resolve('/mundo/metas')}>ver o que falta</a>
							</div>
						</section>
					{/if}
				</aside>
			{/if}
		</div>
	{/if}

	<p class="foot attr">
		{#if route.guide.startsWith('oziris')}Guia por OzirisLoL, versão interativa de ironman.guide.{:else}BRUHsailer
			por So Iron BRUH e ParasailerOSRS.{/if}
		Uso pessoal; atribuição mantida. {ICON_ATTRIBUTION}.
	</p>
{/if}

{#if pinning && boardData}
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
	.grid {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
	}
	.mainc {
		flex: 999 1 600px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.side7 {
		flex: 1 1 300px;
		max-width: 340px;
		min-width: 0;
	}
	.more {
		min-height: 36px;
	}
	.near7 {
		grid-template-columns: 54px minmax(0, 1fr) auto;
	}
	.metas {
		gap: 6px;
	}
	.meta7 {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 0;
		border: 0;
		background: transparent;
		font: inherit;
		font-size: 14px;
		color: var(--ink);
		text-align: left;
		cursor: pointer;
	}
	.meta7 img {
		width: 22px;
		height: 22px;
		image-rendering: pixelated;
	}
	.meta7 b {
		font-weight: 600;
	}
	.small {
		font-size: 12px;
	}
	.lnk0 {
		min-height: 32px;
		padding: 0;
	}
	.attr {
		font-size: 11px;
	}
	.push {
		margin-left: auto;
	}
	@media (max-width: 900px) {
		.mainc {
			gap: 14px;
		}
	}
</style>
