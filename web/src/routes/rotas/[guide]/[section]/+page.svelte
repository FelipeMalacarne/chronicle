<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { itemIcon, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import {
		blockedBadge,
		chipsOf,
		isParagraphStep,
		loadoutItems,
		loadoutLabel,
		metaOf,
		pct,
		pendingBefore,
		plainNotes,
		stepNotes,
		routeLabel,
		sectionLabel,
		segmentsOf,
		statusView,
		type SegmentView
	} from '#lib/progress/view';
	import type {
		Board,
		ContentStep,
		ItemRequirement,
		MarkAction,
		MarkRestore
	} from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import ObjectiveChip from '#lib/v6/ObjectiveChip.svelte';
	import Toast from '#lib/v6/Toast.svelte';

	/*
	 * Rota (V4RotaOziris / V4RotaBruh): uma seção agrupada por trecho de banco. O Oziris mostra
	 * passos curtos com o local como sub-rótulo; o BRUHsailer, passos-parágrafo com o banco como
	 * linha fina entre eles. Concluídos ficam recolhidos; o trecho do cursor abre sozinho.
	 */
	const guideId = $derived(page.params.guide ?? '');
	const sectionId = $derived(page.params.section ?? '');
	const account = $derived(session.account);

	const guides = createResource(() => progressApi.guides());
	const section = createResource(() => progressApi.section(guideId, sectionId));
	const board = createResource(
		() => (account ? progressApi.board(guideId, account) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const route = $derived(session.summary?.routes.find((r) => r.guide === guideId) ?? null);
	const guide = $derived(guides.data?.guides.find((g) => g.id === guideId) ?? null);
	const boardById = $derived(new Map((board.data?.steps ?? []).map((s) => [s.id, s])));
	const cursorId = $derived(board.data?.cursor?.step_id);
	const cursorHere = $derived(!!cursorId && boardById.get(cursorId)?.section_id === sectionId);
	const readiness = createResource(
		() =>
			account && cursorHere && cursorId
				? progressApi.readiness(guideId, account, cursorId).catch(() => null)
				: Promise.resolve(null),
		{ tags: ['progress'] }
	);
	const counts = $derived(board.data?.sections.find((s) => s.id === sectionId)?.counts);
	const segments = $derived(section.data ? segmentsOf(section.data) : []);
	const paragraph = $derived(
		!!section.data &&
			section.data.steps.filter(isParagraphStep).length * 2 > section.data.steps.length
	);
	const hasLayers = $derived(!!section.data?.steps.some((s) => s.layers?.length));
	const label = $derived(
		route
			? routeLabel(route.guide, route.title)
			: guide
				? routeLabel(guide.id, guide.title)
				: guideId
	);
	const AUTHORS: Record<string, string> = {
		'oziris-main': 'OzirisLoL / ironman.guide',
		'bruhsailer-main': 'So Iron BRUH e ParasailerOSRS'
	};
	const sectionsNav = $derived(
		route?.sections.map((s) => ({
			id: s.id,
			title: s.title,
			done: s.counts.done,
			total: s.counts.total
		})) ??
			guide?.sections.map((s) => ({ id: s.id, title: s.title, done: 0, total: s.steps })) ??
			[]
	);

	/* Filtros (V4RotaOziris): pendentes / todos / com objetivo, enhanced, HCIM e busca. */
	let filter = $state<'pending' | 'all' | 'objectives'>('all');
	let enhanced = $state(true);
	let hcim = $state(false);
	let query = $state('');
	function shown(s: ContentStep): boolean {
		const b = boardById.get(s.id);
		if (filter === 'pending' && statusView(b?.status).done) return false;
		if (
			filter === 'objectives' &&
			!(s.objectives ?? []).some((o) => o.role === 'completes' || o.role === 'starts')
		)
			return false;
		if (!enhanced && s.layers?.length === 1 && s.layers[0] === 'enhanced') return false;
		const q = query.trim().toLowerCase();
		if (q) {
			const hay = [s.ref, s.text, s.location ?? '', ...Object.values(s.names)]
				.join(' ')
				.toLowerCase();
			if (!hay.includes(q)) return false;
		}
		return true;
	}

	/*
	 * Com busca ou filtro, a lista se estreita: trechos sem resultado somem, os com resultado abrem
	 * sozinhos (o dono ainda pode recolher) e mostram todos os resultados. Mudar a busca reabre tudo.
	 */
	const narrowed = $derived(query.trim() !== '' || filter !== 'all');
	let searchOpen = $state<Record<string, boolean>>({});
	$effect(() => {
		void query;
		void filter;
		searchOpen = {};
	});

	/* Trechos abertos e o passo expandido: o do cursor por padrão, ou o da âncora #passo-<id>. */
	let open = $state<Record<string, boolean>>({});
	let expanded = $state<string | null>(null);
	let showAll = $state<Record<string, boolean>>({});
	const doneIn = (seg: SegmentView) =>
		seg.steps.filter((s) => statusView(boardById.get(s.id)?.status).done).length;
	function isOpen(seg: SegmentView): boolean {
		if (narrowed) return searchOpen[seg.id] ?? true;
		if (seg.id in open) return open[seg.id];
		if (cursorHere) return seg.steps.some((s) => s.id === cursorId);
		const first = segments.find((g) => doneIn(g) < g.steps.length);
		return first?.id === seg.id;
	}
	const anchor = $derived(
		page.url.hash.startsWith('#passo-') ? decodeURIComponent(page.url.hash.slice(7)) : null
	);
	$effect(() => {
		const target = anchor ?? (cursorHere ? cursorId : null);
		if (!target || !section.data) return;
		const seg = segments.find((g) => g.steps.some((s) => s.id === target));
		if (!seg) return;
		open[seg.id] = true;
		expanded = target;
		if (anchor)
			void tick().then(() =>
				document.getElementById(`passo-${target}`)?.scrollIntoView({ block: 'center' })
			);
	});

	/* Marcas. */
	let busy = $state(false);
	let failure = $state<string | null>(null);
	async function write(run: () => Promise<Board>) {
		busy = true;
		failure = null;
		try {
			board.set(await run());
			void loadSummary(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
	const mark = (id: string, kind: MarkAction) =>
		account && write(() => progressApi.mark(guideId, id, account, kind));
	const confirmAll = () => account && write(() => progressApi.confirmImplied(guideId, account));

	/* Marcar tudo antes deste: confirmação inline com a contagem, toast com desfazer. */
	let confirmBefore = $state<string | null>(null);
	let batch = $state<{ undo: MarkRestore[]; ref: string } | null>(null);
	const beforeCount = (id: string) => pendingBefore(board.data?.steps ?? [], id).length;
	async function markBefore(id: string, ref: string) {
		if (!account) return;
		confirmBefore = null;
		let undo: MarkRestore[] = [];
		await write(async () => {
			const b = await progressApi.markBefore(guideId, account, id);
			undo = b.undo ?? [];
			return b;
		});
		if (undo.length) batch = { undo, ref };
	}
	async function undoBatch() {
		const b = batch;
		batch = null;
		if (b && account) await write(() => progressApi.restoreMarks(guideId, account, b.undo));
	}

	function legCount(seg: SegmentView, shownCount: number): string {
		const total = `${seg.steps.length} ${seg.steps.length === 1 ? 'passo' : 'passos'}`;
		const parts = [narrowed ? `${shownCount} de ${total}` : total];
		const lo = loadoutLabel(seg.loadout);
		if (lo) parts.push(lo);
		const start = seg.bankActions.find((a) => a.position === 'start') ?? seg.bankActions[0];
		if (start) parts.push(`banco: "${start.text}"`);
		return parts.join(' · ');
	}
	function itemsOf(s: ContentStep, seg: SegmentView): ItemRequirement[] {
		const own = s.requirements ?? [];
		return seg.steps[0]?.id === s.id ? [...loadoutItems(seg.loadout), ...own] : own;
	}
	const LIMIT = 30;
	const matches = $derived(segments.reduce((n, g) => n + g.steps.filter(shown).length, 0));
</script>

<svelte:head>
	<title>{section.data?.title ?? 'Rota'} · {label} · Chronicle</title>
</svelte:head>

<div class="wrap rota">
	<aside class="nav-s">
		<a href={resolve('/rotas')} class="su back">← Rotas</a>
		<p class="ey">{label} · seções</p>
		{#each sectionsNav as s (s.id)}
			<a
				class="secn"
				class:secn-on={s.id === sectionId}
				href={resolve('/rotas/[guide]/[section]', { guide: guideId, section: s.id })}
				><span class="i">{sectionLabel(s.id).replace('cap. ', '')}</span>{s.title}<span class="p"
					>{s.done}/{s.total}</span
				></a
			>
		{/each}
		{#if segments.length}
			<p class="ey trechos">Trechos de {sectionLabel(sectionId)}</p>
			<span class="su small pad8"
				>{segments.length} trechos · {segments.filter((g) => g.kind === 'bank_leg').length} com banco.
				Concluídos ficam recolhidos; o do cursor abre sozinho.</span
			>
		{/if}
	</aside>

	<section class="content">
		{#if section.error}
			<div class="band">
				<Glyph name="alert" /><span>Seção não encontrada: {section.error.message}</span>
			</div>
		{:else if !section.data}
			<p class="su" aria-busy="true">Carregando a seção…</p>
		{:else}
			<header class="head">
				<div class="txt">
					<p class="ey">
						{guide?.title ?? label} · seção {sectionLabel(section.data.id)} · {AUTHORS[guideId] ??
							''}
					</p>
					<h1 class="sr">{section.data.title}</h1>
					{#if section.data.description}<p class="mu desc">{section.data.description}</p>{/if}
				</div>
				{#if counts}
					<div class="sum">
						<div class="sumh">
							<span class="su small"
								>{counts.done} feitos · {counts.implied} presumido{counts.implied === 1 ? '' : 's'} ·
								{counts.skipped}
								pulado{counts.skipped === 1 ? '' : 's'}</span
							><span class="mono small">{pct(counts.done, counts.total)}%</span>
						</div>
						<div class="xp"><span style="width: {pct(counts.done, counts.total)}%"></span></div>
						<div class="btns">
							{#if cursorId && route?.cursor_ref}
								{#if route.active}
									<a class="btn btn-p btn-s" href={resolve('/')}
										>Ir ao cursor · {route.cursor_ref}</a
									>
								{:else if route.cursor_section}
									<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + âncora do passo -->
									<a
										class="btn btn-p btn-s"
										href={`${resolve('/rotas/[guide]/[section]', { guide: guideId, section: route.cursor_section })}#passo-${cursorId}`}
										>Ir ao cursor · {route.cursor_ref}</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								{/if}
							{/if}
							{#if (board.data?.counts.implied ?? 0) > 0}
								<button class="btn btn-g btn-s" type="button" disabled={busy} onclick={confirmAll}
									>Confirmar {board.data?.counts.implied} presumido{board.data?.counts.implied === 1
										? ''
										: 's'}</button
								>
							{/if}
						</div>
					</div>
				{/if}
			</header>

			{#if !account}
				<div class="band">
					<Glyph name="alert" /><span
						>Sem conta: os passos aparecem, mas nada é marcado. Abra com <code
							>?account=acc_&lt;rsn&gt;</code
						>.</span
					>
				</div>
			{/if}
			{#if failure}
				<div class="band">
					<Glyph name="alert" /><span>A marca não foi gravada: {failure}</span>
				</div>
			{/if}

			<div class="panel filters">
				<button
					class="chip"
					class:chip-on={filter === 'pending'}
					type="button"
					onclick={() => (filter = 'pending')}>Pendentes</button
				>
				<button
					class="chip"
					class:chip-on={filter === 'all'}
					type="button"
					onclick={() => (filter = 'all')}>Todos</button
				>
				<button
					class="chip"
					class:chip-on={filter === 'objectives'}
					type="button"
					onclick={() => (filter = 'objectives')}>Com objetivo</button
				>
				{#if hasLayers}
					<span class="sep"></span>
					<button
						class="chip"
						class:chip-on={enhanced}
						type="button"
						aria-pressed={enhanced}
						onclick={() => (enhanced = !enhanced)}>Enhanced{enhanced ? ' ✓' : ''}</button
					>
					<button
						class="chip"
						class:chip-on={hcim}
						type="button"
						aria-pressed={hcim}
						onclick={() => (hcim = !hcim)}>HCIM{hcim ? ' ✓' : ''}</button
					>
				{/if}
				<label class="inp"
					><Glyph name="search" size={14} /><input
						type="search"
						bind:value={query}
						placeholder="buscar texto, item, local, quest…"
						aria-label="Buscar passos"
					/></label
				>
			</div>

			<div class="legs">
				{#if narrowed && !matches}
					<p class="su empty">
						Nenhum passo {query.trim() ? `com “${query.trim()}”` : 'com este filtro'} nesta seção.
					</p>
				{/if}
				{#each segments as seg, si (seg.id)}
					{@const done = doneIn(seg)}
					{@const isCur = cursorHere && seg.steps.some((s) => s.id === cursorId)}
					{@const visible = seg.steps.filter(shown)}
					{#if narrowed && !visible.length}
						<!-- trecho sem resultado: some enquanto a busca ou o filtro estiver ativo -->
					{:else if paragraph}
						{#if seg.bankActions.length && si > 0 && !narrowed}
							<div class="bankline">
								banco · {seg.bankActions[0].type} · entre {segments[si - 1].steps.at(-1)?.ref} e {seg
									.steps[0]?.ref}
							</div>
						{/if}
						{#each visible as s (s.id)}
							{@render stepCard(s, seg)}
						{/each}
					{:else}
						<button
							class="leg"
							type="button"
							aria-expanded={isOpen(seg)}
							onclick={() =>
								narrowed ? (searchOpen[seg.id] = !isOpen(seg)) : (open[seg.id] = !isOpen(seg))}
						>
							<Glyph name="chevron" size={12} />Trecho · {seg.range}
							<span class="ct">{legCount(seg, visible.length)}</span>
							<span class="r">
								{#if isCur}<span class="badge badge-gold">cursor</span>{/if}
								{#if isCur && readiness.data?.current}<span
										class="badge {readiness.data.current.status === 'not_ready'
											? 'badge-red'
											: readiness.data.current.status.includes('ready')
												? 'badge-done'
												: 'badge-unk'}">{readiness.data.current.status}</span
									>{/if}
								{#if done === seg.steps.length}<span class="badge badge-done"
										>{done} / {seg.steps.length}{isOpen(seg) ? '' : ' · recolhido'}</span
									>{:else}<span class="ct">{done} / {seg.steps.length}</span>{/if}
							</span>
						</button>
						{#if isOpen(seg)}
							{@const rows = showAll[seg.id] || narrowed ? visible : visible.slice(0, LIMIT)}
							{#each rows as s, i (s.id)}
								{#if s.location && s.location !== rows[i - 1]?.location}<div class="locl">
										{s.location}
									</div>{/if}
								{@render stepRow(s, seg)}
							{/each}
							{#if visible.length > rows.length}
								<div class="row more">
									<span class="st"></span><span class="id">…</span>
									<span class="tx su"
										>mais {visible.length - rows.length} passos deste trecho até {visible.at(-1)
											?.ref}</span
									>
									<button
										class="btn btn-g btn-s mini"
										type="button"
										onclick={() => (showAll[seg.id] = true)}>mostrar</button
									>
								</div>
							{/if}
						{/if}
					{/if}
				{/each}
			</div>
			<p class="su tiny">
				Guia por {AUTHORS[guideId] ?? label}; uso pessoal, atribuição mantida. Objetivos de quest
				ficam "não sabemos" sem leitura do plugin; levels vêm do hiscores ou do plugin. Passos sem
				objetivo detectável são checkbox. {ICON_ATTRIBUTION}.
			</p>
		{/if}
	</section>
</div>

{#snippet expansion(s: ContentStep, seg: SegmentView)}
	{@const b = boardById.get(s.id)}
	{@const chips = chipsOf(s, b)}
	{@const items = itemsOf(s, seg)}
	{@const notes = stepNotes(s).filter((n) => n.kind !== 'hcim')}
	<div class="exp">
		{#if items.length}
			<div class="line">
				<span class="ey lbl">{seg.steps[0]?.id === s.id ? 'Levar · trecho inteiro' : 'Itens'}</span>
				{#each items as it, i (i)}
					{@const icon = itemIcon(it.item_id ?? it.any_of?.[0])}
					{@const unresolved = it.item_id === undefined && !it.any_of?.length && !it.family}
					<span class="tag" class:tag-unk={unresolved}
						>{#if icon}<img class="ski" src={icon} alt="" />{/if}{it.qty && it.qty > 1
							? `${it.qty} `
							: ''}{it.name}{unresolved ? ' · sem id' : ''}</span
					>
				{/each}
			</div>
		{/if}
		<div class="line">
			<span class="ey lbl">Objetivos</span>
			{#each chips as c (c.role + c.key)}<ObjectiveChip chip={c} small />{:else}<span
					class="obj obj-unk obj-s"
					><span class="v v-unk">?</span>nenhum detectável · só você conclui</span
				>{/each}
		</div>
		{#each notes as n, i (i)}<p
				class="note"
				class:note-up={n.kind === 'update'}
				class:mu={n.kind !== 'update'}
			>
				{#if n.label}<span class="nl">{n.label}</span>{/if}{n.text}
			</p>{/each}
		{#if hcim}{#each (s.notes ?? []).filter((n) => n.kind === 'hcim') as n, i (i)}<p
					class="mu note"
				>
					HCIM: {n.text}
				</p>{/each}{/if}
		{#if account}
			<div class="line">
				{#if b?.mark}
					<button
						class="btn btn-s"
						type="button"
						disabled={busy}
						onclick={() => mark(s.id, 'clear')}>Desfazer marca</button
					>
					{#if b.mark.kind === 'implied'}<button
							class="btn btn-p btn-s"
							type="button"
							disabled={busy}
							onclick={() => mark(s.id, 'confirm_implied')}>Confirmar</button
						>{/if}
				{:else}
					<button
						class="btn btn-p btn-s"
						type="button"
						disabled={busy}
						onclick={() => mark(s.id, 'manual_done')}>Feito</button
					>
					<button
						class="btn btn-s"
						type="button"
						disabled={busy}
						onclick={() => mark(s.id, 'skipped')}>Pular</button
					>
				{/if}
				{#if route?.active && s.id === cursorId}<a class="btn btn-g btn-s" href={resolve('/')}
						>Ver no Agora</a
					>{/if}
				{#if beforeCount(s.id) > 0 && confirmBefore !== s.id}
					<button
						class="btn btn-g btn-s"
						type="button"
						disabled={busy}
						onclick={() => (confirmBefore = s.id)}>Marcar tudo antes deste</button
					>
				{/if}
			</div>
			{#if confirmBefore === s.id}
				<div class="line confirm">
					<span class="small"
						>Marcar {beforeCount(s.id)}
						{beforeCount(s.id) === 1 ? 'passo anterior' : 'passos anteriores'} como feitos? Presumidos
						viram feitos; pulados ficam como estão.</span
					>
					<button
						class="btn btn-p btn-s"
						type="button"
						disabled={busy}
						onclick={() => markBefore(s.id, s.ref)}>Marcar</button
					>
					<button class="btn btn-g btn-s" type="button" onclick={() => (confirmBefore = null)}
						>Cancelar</button
					>
				</div>
			{/if}
		{/if}
	</div>
{/snippet}

{#snippet stepRow(s: ContentStep, seg: SegmentView)}
	{@const b = boardById.get(s.id)}
	{@const cur = s.id === cursorId}
	{@const v = statusView(b?.status, cur, !!b?.blocked_by?.length)}
	{@const nItems =
		(s.items?.length ?? 0) +
		(s.requirements?.length ?? 0) +
		(seg.steps[0]?.id === s.id ? loadoutItems(seg.loadout).length : 0)}
	{@const rowChips = chipsOf(s, b).filter((c) => c.role !== 'uses')}
	<button
		id={`passo-${s.id}`}
		class="row"
		class:row-cur={cur}
		class:row-done={v.done}
		class:row-skip={b?.status === 'skipped'}
		type="button"
		aria-expanded={expanded === s.id}
		onclick={() => (expanded = expanded === s.id ? null : s.id)}
	>
		<span class="st {v.st}"
			>{#if v.st === 'st-lock'}<svg
					viewBox="0 0 16 16"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
					><rect x="3.5" y="7" width="9" height="6.5" rx="1.2" /><path
						d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"
					/></svg
				>{/if}</span
		><span class="id" class:gold={cur}>{s.ref}</span>
		<span class="tx" class:strong={cur}>{s.text}</span>
		{#if cur}<span class="badge badge-gold">agora</span>{/if}
		{#if b?.status === 'implied'}<span class="badge">presumido</span>{/if}
		{#if v.st === 'st-lock'}<span class="badge badge-red"
				>{blockedBadge(b?.blocked_by, s.names)}</span
			>{/if}
		{#if s.layers?.length === 1 && s.layers[0] === 'enhanced'}<span class="badge">enhanced</span
			>{/if}
		{#if stepNotes(s).some((n) => n.kind === 'update')}<span class="badge badge-up"
				>atualização</span
			>{:else if plainNotes(s).length}<span class="badge">nota</span>{/if}
		{#each rowChips.slice(0, 3) as c (c.role + c.key)}<ObjectiveChip chip={c} small />{/each}
		{#if rowChips.length > 3}<span class="badge">+{rowChips.length - 3}</span>{/if}
		{#if nItems}<span class="badge">{nItems} {nItems === 1 ? 'item' : 'itens'}</span>{/if}
	</button>
	{#if expanded === s.id}{@render expansion(s, seg)}{/if}
{/snippet}

{#snippet stepCard(s: ContentStep, seg: SegmentView)}
	{@const b = boardById.get(s.id)}
	{@const cur = s.id === cursorId}
	{@const v = statusView(b?.status, cur, !!b?.blocked_by?.length)}
	{@const meta = metaOf(s)}
	<button
		id={`passo-${s.id}`}
		class="card"
		class:card-cur={cur}
		class:card-done={v.done}
		type="button"
		aria-expanded={expanded === s.id}
		onclick={() => (expanded = expanded === s.id ? null : s.id)}
	>
		<span class="tl">
			<span class="st {v.st}"
				>{#if v.st === 'st-lock'}<svg
						viewBox="0 0 16 16"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
						><rect x="3.5" y="7" width="9" height="6.5" rx="1.2" /><path
							d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"
						/></svg
					>{/if}</span
			><span class="ref">{s.ref}</span>
			<span class="tx">{b?.title ?? s.text}</span>
			{#if cur}<span class="badge badge-gold">agora</span>{/if}
			{#if b?.status === 'implied'}<span class="badge">presumido</span>{/if}
			{#if v.st === 'st-lock'}<span class="badge badge-red"
					>{blockedBadge(b?.blocked_by, s.names)}</span
				>{/if}
		</span>
		{#if meta.length}
			<span class="m"
				>{#each meta as m (m.label)}<span>{m.label} {m.value}</span>{/each}</span
			>
		{/if}
	</button>
	{#if expanded === s.id}{@render expansion(s, seg)}{/if}
{/snippet}

{#if batch}
	<Toast
		title="Marcados"
		text={`${batch.undo.length} passos antes de ${batch.ref}`}
		action="desfazer"
		onaction={undoBatch}
		onclose={() => (batch = null)}
	/>
{/if}

<style>
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.nav-s {
		flex: 1 1 200px;
		max-width: 230px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.back {
		font-size: 12px;
		padding: 0 8px 6px;
	}
	.nav-s .ey {
		padding: 6px 8px;
	}
	.nav-s .trechos {
		padding-top: 16px;
	}
	.pad8 {
		padding: 0 8px;
	}
	.content {
		flex: 999 1 560px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px 24px;
	}
	.txt {
		display: flex;
		flex-direction: column;
		gap: 6px;
		flex: 1 1 380px;
		min-width: 0;
	}
	.txt h1 {
		font-size: 28px;
	}
	.desc {
		margin: 0;
		max-width: 70ch;
	}
	.sum {
		display: flex;
		flex-direction: column;
		gap: 8px;
		flex: 0 1 280px;
		min-width: 220px;
	}
	.sumh {
		display: flex;
		justify-content: space-between;
	}
	.btns {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.filters {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 10px 12px;
	}
	.sep {
		width: 1px;
		height: 20px;
		background: var(--border);
		margin: 0 4px;
	}
	.legs {
		display: flex;
		flex-direction: column;
	}
	.legs > :global(.leg:first-child) {
		margin-top: 0;
	}
	.empty {
		padding: 16px 4px;
	}
	.card {
		width: 100%;
		text-align: left;
		background: transparent;
		border-left: 0;
		border-right: 0;
		border-top: 0;
		cursor: pointer;
		font: inherit;
	}
	.line {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 8px;
		align-items: center;
	}
	.lbl {
		margin-right: 4px;
	}
	.note {
		margin: 0;
		font-size: 13px;
	}
	.confirm {
		padding: 8px 10px;
		border-radius: 4px;
		background: var(--gold-tint);
	}
	.gold {
		color: var(--gold-ink);
	}
	.strong {
		font-weight: 500;
	}
	.more {
		border-bottom: 0;
	}
	.mini {
		height: 24px;
		padding: 0 8px;
		font-size: 11px;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	p.tiny {
		margin: 0;
	}
	code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}
	.note-up {
		padding: 6px 10px;
		border-left: 3px solid var(--stale);
		background: var(--stale-tint);
		color: var(--ink);
	}
	.nl {
		margin-right: 8px;
		font:
			600 11px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--stale);
	}
	.badge-up {
		color: var(--stale);
		background: var(--stale-tint);
	}
</style>
