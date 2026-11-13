<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { rowChips, whoDid } from '#lib/progress/agora';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { bankMarks, highlight, snippet, stepFacts, stepMatches } from '#lib/progress/rota';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import {
		blockedBadge,
		chipsOf,
		isParagraphStep,
		loadoutItems,
		looseLinks,
		markRanges,
		metaOf,
		paragraphOf,
		pct,
		piecesOf,
		routeLabel,
		sectionLabel,
		segmentsOf,
		statusView,
		stepNotes
	} from '#lib/progress/view';
	import type {
		Board,
		ContentSection,
		ContentStep,
		ItemRequirement,
		Link,
		MarkAction
	} from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import GoalGrid from '#lib/v6/GoalGrid.svelte';
	import RouteBar from '#lib/v6/RouteBar.svelte';
	import { boardIn, type Scope } from '#lib/progress/undo';

	/*
	 * Conta › Rota › Rota inteira (página 8 do canvas, V7RotaLista1440): a barra da rota, os
	 * capítulos (ou seções) à esquerda com o andamento de cada um, e o capítulo aberto com busca e
	 * filtro numa lista só, o passo atual destacado e o banco como marca na linha do passo. A linha
	 * abre por dentro, como na v6, com os objetivos, as notas, o texto do autor e as marcas. A 640
	 * os capítulos viram um seletor. A busca vale para a rota inteira (página 2b da v6).
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
	const events = createResource(
		() =>
			account ? progressApi.events(guideId, account, 60).catch(() => null) : Promise.resolve(null),
		{ tags: ['progress'] }
	);
	const route = $derived(session.summary?.routes.find((r) => r.guide === guideId) ?? null);
	const activeRoute = $derived(session.summary?.routes.find((r) => r.active) ?? null);
	const guide = $derived(guides.data?.guides.find((g) => g.id === guideId) ?? null);
	/* O board de outra rota (ainda na tela enquanto o novo carrega) nunca alimenta esta seção nem
	 * as escritas: ids de passo se repetem entre guias (regra 7). */
	const scope = $derived(account && guideId ? { account, guide: guideId } : null);
	const boardData = $derived(boardIn(board.data, scope));
	const boardById = $derived(new Map((boardData?.steps ?? []).map((s) => [s.id, s])));
	const cursorId = $derived(boardData?.cursor?.step_id);
	const cursorStep = $derived(cursorId ? boardById.get(cursorId) : undefined);
	const cursorHere = $derived(!!cursorStep && cursorStep.section_id === sectionId);
	const counts = $derived(boardData?.sections.find((s) => s.id === sectionId)?.counts);
	const segments = $derived(section.data ? segmentsOf(section.data) : []);
	const marks = $derived(bankMarks(segments));
	const segOf = $derived(new Map(segments.flatMap((g) => g.steps.map((s) => [s.id, g] as const))));
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
	const unit = $derived(paragraph ? 'capítulo' : 'seção');
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
	const secHref = (id: string) =>
		resolve('/rotas/[guide]/[section]', { guide: guideId, section: id });
	const curSection = $derived(sectionsNav.find((s) => s.id === sectionId));
	const cap = $derived(
		section.data && counts
			? {
					label: sectionLabel(section.data.id),
					title: section.data.title,
					done: counts.done,
					total: counts.total
				}
			: null
	);

	/* Filtros: Todos / Pendentes / Com objetivo num controle, Enhanced e HCIM como caixas, busca. */
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
		return stepMatches(s, query);
	}
	const searching = $derived(query.trim() !== '');
	const narrowed = $derived(searching || filter !== 'all');
	const visible = $derived((section.data?.steps ?? []).filter(shown));

	/*
	 * A busca vale para a rota inteira: as outras seções vêm uma vez por rota, quando a primeira
	 * busca começa, e os passos delas que batem aparecem abaixo dos desta seção, com link.
	 */
	const OTHER_LIMIT = 5;
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- cache de requisições, fora do grafo reativo
	const sectionsOf = new Map<string, Promise<ContentSection[]>>();
	let others = $state<{ guide: string; sections: ContentSection[] } | null>(null);
	$effect(() => {
		if (!searching || !sectionsNav.length) return;
		const g = guideId;
		let p = sectionsOf.get(g);
		if (!p) {
			p = Promise.all(sectionsNav.map((sec) => progressApi.section(g, sec.id)));
			sectionsOf.set(g, p);
		}
		p.then(
			(sections) => (others = { guide: g, sections }),
			() => sectionsOf.delete(g)
		);
	});
	const othersLoading = $derived(searching && others?.guide !== guideId);
	const otherHits = $derived(
		!searching || !others || others.guide !== guideId
			? []
			: others.sections
					.filter((sec) => sec.id !== sectionId)
					.map((sec) => ({ sec, steps: sec.steps.filter(shown) }))
					.filter((h) => h.steps.length)
	);
	const otherCount = $derived(otherHits.reduce((n, h) => n + h.steps.length, 0));
	const elsewhere = $derived(paragraph ? 'outros capítulos' : 'outras seções');

	/* A janela em volta do passo atual (3 antes, 10 depois), com "mais N passos". */
	const BEFORE = 3;
	const AFTER = 10;
	const LIMIT = 30;
	let showAll = $state(false);
	let expanded = $state<string | null>(null);
	$effect(() => {
		void sectionId;
		showAll = false;
	});
	const win = $derived.by(() => {
		if (showAll || narrowed) return { rows: visible, before: 0, after: 0 };
		const ci = visible.findIndex((s) => s.id === cursorId);
		if (ci < 0) {
			const rows = visible.slice(0, LIMIT);
			return { rows, before: 0, after: visible.length - rows.length };
		}
		const from = Math.max(0, ci - BEFORE);
		const to = Math.min(visible.length, ci + AFTER + 1);
		return { rows: visible.slice(from, to), before: from, after: visible.length - to };
	});
	const anchor = $derived(
		page.url.hash.startsWith('#passo-') ? decodeURIComponent(page.url.hash.slice(7)) : null
	);
	/* O passo atual fica destacado e fechado (V7RotaLista1440), também quando a âncora aponta para
	 * ele ("ver a rota inteira" do Passo); a âncora de outro passo o abre. Espera o board, que diz
	 * qual é o atual. */
	$effect(() => {
		const target = anchor;
		if (!target || !section.data || !boardData) return;
		if (target !== cursorId) {
			showAll = true;
			expanded = target;
		}
		void tick().then(() => scrollTo(target));
	});
	function scrollTo(id: string) {
		document.getElementById(`passo-${id}`)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
	}

	/* A faixa "Você está em" aparece enquanto a linha do cursor não está na tela. */
	let cursorVisible = $state(false);
	$effect(() => {
		const id = cursorId;
		void expanded;
		void win;
		if (!id || !cursorHere) {
			cursorVisible = false;
			return;
		}
		let obs: IntersectionObserver | undefined;
		void tick().then(() => {
			const el = document.getElementById(`passo-${id}`);
			if (!el) {
				cursorVisible = false;
				return;
			}
			obs = new IntersectionObserver(([e]) => (cursorVisible = e.isIntersecting));
			obs.observe(el);
		});
		return () => obs?.disconnect();
	});
	async function goCursor() {
		if (!cursorId || !cursorStep) return;
		if (!cursorHere) {
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + âncora do passo
			await goto(`${secHref(cursorStep.section_id)}#passo-${cursorId}`);
			return;
		}
		await tick();
		scrollTo(cursorId);
	}

	/* Marcas. */
	let busy = $state(false);
	let failure = $state<string | null>(null);
	/** Uma escrita por vez, presa à conta e à rota em que foi pedida. */
	async function write(run: (at: Scope) => Promise<Board>) {
		const at = scope;
		if (!at || busy) return;
		busy = true;
		failure = null;
		try {
			const b = await run(at);
			if (boardIn(b, scope)) board.set(b);
			else board.reload();
			events.reload();
			void loadSummary(at.account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
	const mark = (id: string, kind: MarkAction) =>
		write((at) => progressApi.mark(at.guide, id, at.account, kind));
	const confirmAll = () => write((at) => progressApi.confirmImplied(at.guide, at.account));
	async function activate() {
		if (!account) return;
		try {
			await progressApi.setActiveGuide(account, guideId);
			await loadSummary(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		}
	}

	function itemsOf(s: ContentStep): ItemRequirement[] {
		const seg = segOf.get(s.id);
		const own = s.requirements ?? [];
		const all = seg && seg.steps[0]?.id === s.id ? [...loadoutItems(seg.loadout), ...own] : own;
		// O loadout do trecho e os requisitos do passo repetem itens: um por nome e quantidade.
		return all.filter(
			(it, i) => all.findIndex((x) => x.name === it.name && x.qty === it.qty) === i
		);
	}
	/** Nome do link como o autor escreveu; sem texto, o host. */
	const linkLabel = (l: Link) => {
		if (l.text?.trim()) return l.text.trim();
		try {
			return new URL(l.url).hostname;
		} catch {
			return l.url;
		}
	};
	const itemLabel = (it: ItemRequirement) =>
		it.qty && it.qty > 1 ? `${it.name} ×${it.qty}` : it.name;
	/** A segunda linha da direita: quem fez, ou o GP e o tempo do guia, ou o local, ou os itens. */
	function factsOf(s: ContentStep, done: boolean): string {
		if (done) return whoDid(events.data, s.id, now);
		const f = stepFacts(s);
		if (f) return f;
		const n = itemsOf(s).length;
		return [s.location ?? '', n ? `${n} ${n === 1 ? 'item' : 'itens'}` : '']
			.filter(Boolean)
			.join(' · ');
	}
	const now = Date.now();
	let picking = $state(false);
</script>

<svelte:head>
	<title>{section.data?.title ?? 'Rota'} · {label} · Chronicle</title>
</svelte:head>

<svelte:window
	onclick={(e) => picking && !(e.target as HTMLElement).closest('.cpick') && (picking = false)}
/>

<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs internos vêm de resolve() (secHref, resolve + âncora) -->

{#snippet parts(text: string)}{#each highlight(text, query) as p, i (i)}{#if p.m}<mark>{p.t}</mark
			>{:else}{p.t}{/if}{/each}{/snippet}

{#snippet expansion(s: ContentStep)}
	{@const b = boardById.get(s.id)}
	{@const chips = chipsOf(s, b)}
	{@const items = itemsOf(s).filter((it) => !chips.some((c) => c.label === it.name))}
	{@const notes = stepNotes(s).filter((n) => n.kind !== 'hcim')}
	{@const meta = metaOf(s).filter((m) => m.label !== 'itens')}
	{@const para = paragraph ? paragraphOf(s.text) : null}
	{@const prose = para
		? para.tldr
			? para.rest.map((t, i) => ({ t, at: para.restAt[i] }))
			: [{ t: para.head, at: para.headAt }, ...para.rest.map((t, i) => ({ t, at: para.restAt[i] }))]
		: []}
	{@const ranges = markRanges(s.text, s.marks)}
	{@const loose = looseLinks(prose.map((p) => p.t).join('\n'), s.links)}
	<div class="exp">
		{#if meta.length || (paragraph && s.bank_actions?.length)}
			<div class="metas">
				{#each meta as m (m.label)}<span class="meta">{m.label} <b>{m.value}</b></span>{/each}
				{#if s.bank_actions?.length}<span class="meta">banco <b>dentro do passo</b></span>{/if}
			</div>
		{/if}
		{#if chips.length}<GoalGrid {chips} objectives={b?.objectives} />{/if}
		{#each notes as n, i (i)}
			<p class="note" class:note-up={n.kind === 'update'}>
				<span class="lbl">{n.label || 'Nota do autor'}</span>{n.text}
			</p>
		{/each}
		{#if hcim}{#each (s.notes ?? []).filter((n) => n.kind === 'hcim') as n, i (i)}<p class="note">
					<span class="lbl">HCIM</span>{n.text}
				</p>{/each}{/if}
		{#if items.length}
			<p class="note"><span class="lbl">Itens</span>{items.map(itemLabel).join(', ')}</p>
		{/if}
		{#if para}
			<div class="prose">
				{#each prose as p, i (i)}<p>
						{#each piecesOf(p.t, p.at, ranges, (s.bank_actions ?? [])[0]?.text ?? '', s.links) as x, j (j)}{#if x.href}<a
									class={x.c ? `mk mk-${x.c}` : undefined}
									href={x.href}
									target="_blank"
									rel="noreferrer">{x.t}</a
								>{:else if x.m}<mark class={x.c ? `mk mk-${x.c}` : undefined}>{x.t}</mark
								>{:else if x.c}<span class="mk mk-{x.c}">{x.t}</span>{:else}{x.t}{/if}{/each}
					</p>{/each}
			</div>
		{/if}
		{#if loose.length}
			<p class="note">
				<span class="lbl">Links</span>
				{#each loose as l, i (l.url + i)}{#if i},
					{/if}<a href={l.url} target="_blank" rel="noreferrer">{linkLabel(l)}</a>{/each}
			</p>
		{/if}
		{#if account}
			<div class="acts">
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
						onclick={() => mark(s.id, 'manual_done')}><Glyph name="check" size={14} />Feito</button
					>
					<button
						class="btn btn-s"
						type="button"
						disabled={busy}
						onclick={() => mark(s.id, 'skipped')}>Pular</button
					>
				{/if}
				<span class="grow"></span>
				{#if route?.active && s.id === cursorId}<a class="lnk" href={resolve('/agora')}
						>Abrir o passo</a
					>{/if}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet stepRow(s: ContentStep)}
	{@const b = boardById.get(s.id)}
	{@const cur = s.id === cursorId}
	{@const v = statusView(b?.status, cur, !!b?.blocked_by?.length)}
	{@const rc = rowChips(v.done ? [] : chipsOf(s, b), cur ? 3 : 1)}
	{@const bm = marks.get(s.id)}
	{@const facts = factsOf(s, v.done)}
	<div id={`passo-${s.id}`} class="steprow">
		<button
			class="st7"
			class:cur
			class:dn={v.done}
			class:skip={b?.status === 'skipped'}
			type="button"
			aria-expanded={expanded === s.id}
			title={v.st === 'st-lock' ? blockedBadge(b?.blocked_by, s.names) : v.label || undefined}
			onclick={() => (expanded = expanded === s.id ? null : s.id)}
		>
			<span class="st {v.st}"
				>{#if v.done}<Glyph name="check" size={10} />{:else if v.st === 'st-lock'}<svg
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
			><span class="id">{s.ref}</span><span class="tx"
				>{@render parts(paragraph ? (b?.title ?? s.text) : s.text)}</span
			><span class="rt"
				><span class="ch"
					>{#if bm}<span class="bank7" title={bm.title}
							><Glyph name="bank" size={11} />{bm.label}</span
						>{/if}{#each rc.shown as c (c.role + c.key)}{@const icon = iconFor(c.ref)}{#if icon}<img
								src={icon}
								alt={c.label}
								title={c.label}
							/>{/if}{/each}{#if rc.more}+{rc.more}{/if}</span
				>{#if facts}<span>{facts}</span>{/if}</span
			>
		</button>
		{#if expanded === s.id}{@render expansion(s)}{/if}
	</div>
{/snippet}

<RouteBar view="lista" guide={guideId} section={cap} />

{#if failure}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>A marca não foi gravada: {failure}</span>
	</div>
{/if}

<div class="wrap">
	{#if !narrow.value}
		<nav class="chap" aria-label={paragraph ? 'Capítulos' : 'Seções'}>
			<p class="lbl chl">
				{paragraph ? 'Capítulos' : 'Seções'}{#if route}
					· {route.counts.done} de {route.counts.total}{/if}
			</p>
			{#each sectionsNav as s (s.id)}
				<a
					class="ch7"
					class:on={s.id === sectionId}
					class:dn={s.total > 0 && s.done === s.total}
					href={secHref(s.id)}
					aria-current={s.id === sectionId ? 'page' : undefined}
					><span class="i">{sectionLabel(s.id).replace(/^cap\. /, '')}</span><span>{s.title}</span
					><span class="p">{s.done}/{s.total}</span></a
				>
			{/each}
		</nav>
	{/if}

	<section class="content">
		{#if section.error}
			<div class="band" role="alert">
				<Glyph name="alert" /><span>Seção não encontrada: {section.error.message}</span>
			</div>
		{:else if !section.data}
			<p class="su" aria-busy="true">Carregando a seção…</p>
		{:else}
			{#if narrow.value}
				<div class="cpick">
					<button
						class="sel sel-s"
						type="button"
						aria-haspopup="listbox"
						aria-expanded={picking}
						onclick={() => (picking = !picking)}
						><span class="mono su tiny">{sectionLabel(sectionId)}</span><span class="stt"
							>{curSection?.title ?? section.data.title}</span
						><small>▾</small></button
					>
					{#if picking}
						<div class="menu panel" role="listbox" aria-label={unit}>
							{#each sectionsNav as s (s.id)}
								<a
									class="ch7"
									class:on={s.id === sectionId}
									class:dn={s.total > 0 && s.done === s.total}
									role="option"
									aria-selected={s.id === sectionId}
									href={secHref(s.id)}
									onclick={() => (picking = false)}
									><span class="i">{sectionLabel(s.id).replace(/^cap\. /, '')}</span><span
										>{s.title}</span
									><span class="p">{s.done}/{s.total}</span></a
								>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			<div class="lh">
				<div class="ttl">
					<h2 class="sr">{section.data.title}</h2>
					<span class="mu small"
						>{[
							section.data.description ?? '',
							counts
								? `${counts.done} de ${counts.total} · ${pct(counts.done, counts.total)}%`
								: '',
							counts?.implied ? `${counts.implied} presumido${counts.implied === 1 ? '' : 's'}` : ''
						]
							.filter(Boolean)
							.join(' · ')}</span
					>
				</div>
				<label class="field fsearch"
					><Glyph name="search" size={14} /><input
						type="search"
						bind:value={query}
						placeholder="buscar na rota"
						aria-label="Buscar passos na rota"
					/>{#if searching}<span class="cnt"
							>{visible.length} de {section.data.steps.length}{#if otherCount}
								· +{otherCount}{/if}</span
						>{/if}</label
				>
				<div class="seg" role="group" aria-label="Quais passos">
					{#each [['all', 'Todos'], ['pending', 'Pendentes'], ['objectives', 'Com objetivo']] as [k, l] (k)}
						<button
							type="button"
							class:on={filter === k}
							aria-pressed={filter === k}
							onclick={() => (filter = k as typeof filter)}>{l}</button
						>
					{/each}
				</div>
				{#if hasLayers}
					<label class="chk"><input type="checkbox" bind:checked={enhanced} />Enhanced</label>
					<label class="chk"><input type="checkbox" bind:checked={hcim} />HCIM</label>
				{/if}
			</div>

			{#if (boardData?.counts.implied ?? 0) > 0}
				<button
					class="lnk sm"
					type="button"
					disabled={busy}
					title="Presumidos são passos que um passo posterior implica; confirmar vira feito à mão."
					onclick={confirmAll}>confirmar {boardData?.counts.implied} presumidos</button
				>
			{/if}

			{#if cursorStep && route && !route.active}
				<div class="note7 cstrip">
					<span class="st st-cur sd" aria-hidden="true"></span><span
						>Você estaria em <b>{cursorStep.ref}</b> · {cursorStep.title}</span
					>
					<span class="push">
						{#if activeRoute}<span class="mu small"
								>o lugar na {routeLabel(activeRoute.guide, activeRoute.title)} fica guardado</span
							>{/if}
						<button class="btn btn-s btn-p" type="button" onclick={activate}>Tornar ativa</button>
					</span>
				</div>
			{:else if cursorStep && !cursorVisible}
				<button class="cstrip go" type="button" aria-label="Ir ao passo atual" onclick={goCursor}>
					<span class="st st-cur sd" aria-hidden="true"></span><span
						>Você está em <b>{cursorStep.ref}</b> · {cursorStep.title}</span
					>
					<span class="push gold">ir ao passo ↓</span>
				</button>
			{/if}

			<div class="card list">
				{#if narrowed && !visible.length}
					<p class="su empty">
						Nenhum passo {searching ? `com “${query.trim()}”` : 'com este filtro'}
						{paragraph ? 'neste capítulo' : 'nesta seção'}{otherCount
							? `; ${otherCount} em ${elsewhere}`
							: ''}.
					</p>
				{/if}
				{#if win.before}
					<button class="lnk more" type="button" onclick={() => (showAll = true)}
						>mais {win.before} {win.before === 1 ? 'passo' : 'passos'} antes</button
					>
				{/if}
				{#each win.rows as s (s.id)}{@render stepRow(s)}{/each}
				{#if win.after}
					<button class="lnk more" type="button" onclick={() => (showAll = true)}
						>mais {win.after}
						{win.after === 1 ? 'passo' : 'passos'}
						{paragraph ? 'neste capítulo' : 'nesta seção'}</button
					>
				{/if}
			</div>

			{#if othersLoading}
				<p class="su small">buscando em {elsewhere}…</p>
			{:else if otherHits.length}
				<div class="rh">
					<h2>Em {elsewhere}</h2>
					<span>{otherCount}</span>
				</div>
				{#each otherHits as h (h.sec.id)}
					<section class="card list">
						<div class="h">
							<h2>
								<a class="sh" href={secHref(h.sec.id)}
									><span class="mono su tiny">{sectionLabel(h.sec.id)}</span> {h.sec.title}</a
								>
							</h2>
							<span>{h.steps.length} com “{query.trim()}”</span>
						</div>
						<div class="hits">
							{#each h.steps.slice(0, OTHER_LIMIT) as s (s.id)}
								{@const b = boardById.get(s.id)}
								{@const v = statusView(b?.status, s.id === cursorId, !!b?.blocked_by?.length)}
								<a class="st7" class:dn={v.done} href={`${secHref(h.sec.id)}#passo-${s.id}`}
									><span class="st {v.st}"
										>{#if v.done}<Glyph name="check" size={10} />{/if}</span
									><span class="id">{s.ref}</span><span class="tx"
										>{@render parts(snippet(s.text, query))}</span
									><span class="rt"></span></a
								>
							{/each}
							{#if h.steps.length > OTHER_LIMIT}
								<a class="lnk more" href={secHref(h.sec.id)}
									>mais {h.steps.length - OTHER_LIMIT} em {sectionLabel(h.sec.id)}</a
								>
							{/if}
						</div>
					</section>
				{/each}
			{/if}
		{/if}
	</section>
</div>

<p class="foot attr">
	{#if guideId.startsWith('bruhsailer')}BRUHsailer por So Iron BRUH e ParasailerOSRS.{:else}Guia por
		OzirisLoL, versão interativa de ironman.guide.{/if} Uso pessoal; atribuição mantida.
	{ICON_ATTRIBUTION}.
</p>

<style>
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
	}
	.chap {
		flex: 1 1 220px;
		max-width: 260px;
		min-width: 0;
		position: sticky;
		top: 16px;
		max-height: calc(100vh - 32px);
		overflow: auto;
	}
	.chl {
		padding: 0 10px 6px;
	}
	.content {
		flex: 999 1 600px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.lh {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: auto;
		min-width: 0;
	}
	.ttl h2 {
		font-size: 22px;
	}
	.fsearch {
		height: 36px;
		flex: 0 1 260px;
	}
	.fsearch input {
		font-size: 13px;
	}
	.cnt {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.chk {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	.sm {
		align-self: flex-start;
		min-height: 32px;
		font-size: 12px;
		background: none;
		border: 0;
		cursor: pointer;
	}
	.list {
		overflow: hidden;
	}
	.steprow:last-of-type .st7 {
		border-bottom: 0;
	}
	.st7 .st {
		width: 18px;
		height: 18px;
	}
	.st7.skip .tx {
		color: var(--ink-subtle);
	}
	.st7 :global(mark),
	.hits :global(mark) {
		background: var(--gold-tint);
		color: inherit;
		box-shadow: inset 0 -1px 0 var(--gold-ink);
	}
	.more {
		min-height: 40px;
		padding: 0 12px;
		background: none;
		border: 0;
		cursor: pointer;
		font: inherit;
		font-size: 13px;
		align-self: flex-start;
	}
	.empty {
		margin: 0;
		padding: 16px;
		font-size: 13px;
	}
	.cstrip {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		font-size: 13px;
		text-align: left;
	}
	.cstrip.go {
		position: sticky;
		top: 8px;
		z-index: 3;
		padding: 8px 12px;
		border-radius: 5px;
		border: 1px solid var(--gold-ink);
		background: var(--surface);
		box-shadow: var(--bevel);
		color: var(--ink);
		font-family: inherit;
		cursor: pointer;
	}
	.cstrip b {
		font-weight: 600;
	}
	.sd {
		width: 14px;
		height: 14px;
	}
	.push {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 10px;
	}
	.gold {
		color: var(--gold-ink);
		font-weight: 500;
	}
	.exp {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 4px 16px 16px 46px;
		border-bottom: 1px solid var(--hairline);
		background: var(--gold-tint);
	}
	.exp :global(.goals) {
		padding: 4px 0;
	}
	.metas {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.note {
		margin: 0;
		font-size: 13px;
		color: var(--ink-muted);
		max-width: 80ch;
	}
	.note .lbl {
		margin-right: 6px;
	}
	.note-up {
		padding: 6px 10px;
		border-left: 3px solid var(--stale);
		color: var(--ink);
	}
	.prose {
		max-width: 80ch;
		padding: 10px 14px;
		border-radius: 4px;
		border: 1px solid var(--hairline);
		background: var(--surface);
	}
	.mk {
		font-weight: 600;
	}
	.mk-red {
		color: var(--not-started);
	}
	.mk-orange {
		color: var(--stale);
	}
	.mk-green {
		color: var(--done);
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.grow {
		flex: 1;
	}
	.sh {
		color: var(--ink);
		text-decoration: none;
	}
	.hits {
		display: flex;
		flex-direction: column;
		padding-top: 8px;
	}
	.attr {
		font-size: 11px;
	}
	/* 640: os capítulos num seletor no topo (V6Rota640, mantido na v7). */
	.cpick {
		position: relative;
	}
	.sel-s {
		width: 100%;
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--bevel);
		border: 1px solid var(--border);
		justify-content: flex-start;
	}
	.sel-s small {
		color: var(--ink-subtle);
		margin-left: auto;
	}
	.stt {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font:
			500 13px Inter,
			system-ui,
			sans-serif;
	}
	.menu {
		position: absolute;
		z-index: 8;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		max-height: 60vh;
		overflow: auto;
		display: flex;
		flex-direction: column;
		padding: 4px;
		box-shadow:
			var(--bevel),
			0 8px 24px rgba(0, 0, 0, 0.25);
	}
</style>
