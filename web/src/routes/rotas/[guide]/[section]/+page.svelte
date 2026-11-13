<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { rowChip, whoDid } from '#lib/progress/agora';
	import { ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { highlight, legSummary, snippet, stepMatches } from '#lib/progress/rota';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import {
		blockedBadge,
		chipsOf,
		isParagraphStep,
		loadoutItems,
		looseLinks,
		markRanges,
		loadoutLabel,
		metaOf,
		paragraphOf,
		pct,
		piecesOf,
		routeLabel,
		routeTitle,
		sectionLabel,
		segmentsOf,
		statusView,
		stepNotes,
		type SegmentView
	} from '#lib/progress/view';
	import type {
		Board,
		ContentSection,
		ContentStep,
		ItemRequirement,
		Link,
		MarkAction
	} from '#lib/progress/types';
	import ChipGroups from '#lib/v6/ChipGroups.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import ObjectiveChip from '#lib/v6/ObjectiveChip.svelte';
	import RouteTabs from '#lib/v6/RouteTabs.svelte';
	import { boardIn, type Scope } from '#lib/progress/undo';

	/*
	 * Rota (V6Rota1440 e V6Rota640; BRUHsailer em V6RotaBruh1440): abas de rota no topo, seções à
	 * esquerda (no estreito, um seletor), uma barra de filtros, a faixa "Você está em" enquanto o
	 * cursor não está na tela e a seção agrupada por trecho com nome. O BRUHsailer não tem trecho:
	 * os passos-parágrafo vêm em linhas com o banco como linha fina entre elas.
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
	const title = $derived(routeTitle(guideId, guide?.title ?? label));
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
	const secHref = (id: string) =>
		resolve('/rotas/[guide]/[section]', { guide: guideId, section: id });

	/* Filtros: Pendentes / Todos / Com objetivo num controle, Enhanced e HCIM como caixas, busca. */
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
	const total = $derived(section.data?.steps.length ?? 0);
	const matches = $derived(segments.reduce((n, g) => n + g.steps.filter(shown).length, 0));

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

	/* Trechos abertos e o passo expandido: o do cursor por padrão, ou o da âncora #passo-<id>. */
	let open = $state<Record<string, boolean>>({});
	let expanded = $state<string | null>(null);
	let showAll = $state<Record<string, boolean>>({});
	const doneIn = (seg: SegmentView) =>
		seg.steps.filter((s) => statusView(boardById.get(s.id)?.status).done).length;
	function isOpen(seg: SegmentView): boolean {
		if (narrowed) return true;
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
		showAll[seg.id] = showAll[seg.id] || (!!anchor && anchor !== cursorId);
		expanded = target;
		if (anchor) void tick().then(() => scrollTo(target));
	});
	function scrollTo(id: string) {
		document.getElementById(`passo-${id}`)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
	}

	/* A faixa "Você está em" aparece enquanto a linha do cursor não está na tela. */
	let cursorVisible = $state(false);
	$effect(() => {
		const id = cursorId;
		void expanded;
		void open;
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
		const seg = segments.find((g) => g.steps.some((s) => s.id === cursorId));
		if (seg) open[seg.id] = true;
		expanded = cursorId;
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

	const clip = (s: string, n = 28) => (s.length > n ? `${s.slice(0, n - 1)}…` : s);

	/* Trecho do cursor: a janela em volta dele, com "mostrar tudo". */
	const BEFORE = 3;
	const AFTER = 10;
	const LIMIT = 30;
	function windowOf(seg: SegmentView, visible: ContentStep[]) {
		if (showAll[seg.id] || narrowed) return { rows: visible, before: 0, after: 0 };
		const ci = visible.findIndex((s) => s.id === cursorId);
		if (ci < 0) {
			const rows = visible.slice(0, LIMIT);
			return { rows, before: 0, after: visible.length - rows.length };
		}
		const from = Math.max(0, ci - BEFORE);
		const to = Math.min(visible.length, ci + AFTER + 1);
		return { rows: visible.slice(from, to), before: from, after: visible.length - to };
	}
	function itemsOf(s: ContentStep, seg: SegmentView): ItemRequirement[] {
		const own = s.requirements ?? [];
		const all = seg.steps[0]?.id === s.id ? [...loadoutItems(seg.loadout), ...own] : own;
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
	function bankLine(prev: SegmentView, seg: SegmentView): string {
		const parts = [`banco · entre ${prev.steps.at(-1)?.ref} e ${seg.steps[0]?.ref}`];
		const lo = loadoutItems(seg.loadout);
		if (seg.loadout?.status === 'partial') parts.push(`loadout parcial (${lo.length} itens)`);
		else if (lo.length) parts.push(`${lo.length} itens`);
		return parts.join(' · ');
	}
	/** "GP 1.719 · 4 h 30" do BRUHsailer; no Oziris, "+2 · 3 itens". */
	function moreOf(
		s: ContentStep,
		seg: SegmentView,
		extraChips: number,
		cur: boolean
	): { text: string; tip: string } {
		const meta = metaOf(s);
		const items = itemsOf(s, seg);
		const list = items.map(itemLabel).join(', ');
		const count = items.length ? `${items.length} ${items.length === 1 ? 'item' : 'itens'}` : '';
		if (paragraph && !cur) {
			const gp = meta.find((m) => m.label === 'GP após');
			const time = meta.find((m) => m.label === 'tempo acumulado');
			return {
				text: [gp ? `GP ${clip(gp.value.replace(/\s*\(.*$/, ''), 16)}` : '', time?.value ?? '']
					.filter(Boolean)
					.join(' · '),
				tip: [meta.map((m) => `${m.label} ${m.value}`).join(' · '), list]
					.filter(Boolean)
					.join(' · ')
			};
		}
		return {
			text: [extraChips ? `+${extraChips}` : '', count].filter(Boolean).join(' · '),
			tip: list ? `usa ${list}` : ''
		};
	}
	const now = Date.now();

	/* Seletores do estreito (V6Rota640). */
	let menu = $state<'route' | 'section' | null>(null);
	const curSection = $derived(sectionsNav.find((s) => s.id === sectionId));
</script>

<svelte:head>
	<title>{section.data?.title ?? 'Rota'} · {label} · Chronicle</title>
</svelte:head>

<svelte:window
	onclick={(e) => menu && !(e.target as HTMLElement).closest('.pop') && (menu = null)}
/>

<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs internos vêm de resolve() (secHref, resolve + âncora) -->

{#snippet parts(text: string)}{#each highlight(text, query) as p, i (i)}{#if p.m}<mark>{p.t}</mark
			>{:else}{p.t}{/if}{/each}{/snippet}

{#snippet expansion(s: ContentStep, seg: SegmentView)}
	{@const b = boardById.get(s.id)}
	{@const chips = chipsOf(s, b)}
	{@const items = itemsOf(s, seg).filter((it) => !chips.some((c) => c.label === it.name))}
	{@const notes = stepNotes(s).filter((n) => n.kind !== 'hcim')}
	{@const meta = metaOf(s).filter((m) => m.label !== 'itens')}
	{@const para = paragraph ? paragraphOf(s.text) : null}
	{@const shown = para
		? para.tldr
			? para.rest.map((t, i) => ({ t, at: para.restAt[i] }))
			: [{ t: para.head, at: para.headAt }, ...para.rest.map((t, i) => ({ t, at: para.restAt[i] }))]
		: []}
	{@const ranges = markRanges(s.text, s.marks)}
	{@const loose = looseLinks(shown.map((p) => p.t).join('\n'), s.links)}
	<div class="exp">
		{#if meta.length || (paragraph && s.bank_actions?.length)}
			<div class="metas">
				{#each meta as m (m.label)}<span class="meta">{m.label} <b>{m.value}</b></span>{/each}
				{#if s.bank_actions?.length}<span class="meta">banco <b>dentro do passo</b></span>{/if}
			</div>
		{/if}
		{#each notes as n, i (i)}
			<p class="mu note" class:note-up={n.kind === 'update'}>
				<span class="lbl nl">{n.label || 'Nota do autor'}</span>{n.text}
			</p>
		{/each}
		{#if hcim}{#each (s.notes ?? []).filter((n) => n.kind === 'hcim') as n, i (i)}<p
					class="mu note"
				>
					<span class="lbl nl">HCIM</span>{n.text}
				</p>{/each}{/if}
		{#if chips.length}<ChipGroups {chips} />{/if}
		{#if items.length}
			<p class="su small its">
				<span class="lbl">Itens</span>
				{items.map(itemLabel).join(', ')}
			</p>
		{/if}
		{#if para}
			<div class="prose box">
				{#each shown as p, i (i)}<p>
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
			<p class="su small its">
				<span class="lbl">Links</span>
				{#each loose as l, i (l.url + i)}{#if i},
					{/if}<a href={l.url} target="_blank" rel="noreferrer">{linkLabel(l)}</a>{/each}
			</p>
		{/if}
		{#if account}
			<div class="acts">
				{#if b?.mark}
					<button class="btn" type="button" disabled={busy} onclick={() => mark(s.id, 'clear')}
						>Desfazer marca</button
					>
					{#if b.mark.kind === 'implied'}<button
							class="btn btn-p"
							type="button"
							disabled={busy}
							onclick={() => mark(s.id, 'confirm_implied')}>Confirmar</button
						>{/if}
				{:else}
					<button
						class="btn btn-p"
						type="button"
						disabled={busy}
						onclick={() => mark(s.id, 'manual_done')}><Glyph name="check" size={14} />Feito</button
					>
					<button class="btn" type="button" disabled={busy} onclick={() => mark(s.id, 'skipped')}
						>Pular</button
					>
				{/if}
				<span class="grow"></span>
				{#if route?.active && s.id === cursorId}<a class="lnk" href={resolve('/agora')}
						>Ver no Agora</a
					>{/if}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet stepRow(s: ContentStep, seg: SegmentView)}
	{@const b = boardById.get(s.id)}
	{@const cur = s.id === cursorId}
	{@const v = statusView(b?.status, cur, !!b?.blocked_by?.length)}
	{@const rc = rowChip(chipsOf(s, b))}
	{@const more = moreOf(s, seg, rc.more, cur)}
	{@const showChip = !paragraph || cur}
	<div
		id={`passo-${s.id}`}
		class="row"
		class:row-cur={cur}
		class:row-done={v.done}
		class:row-skip={b?.status === 'skipped'}
	>
		<button
			class="hit"
			type="button"
			aria-expanded={expanded === s.id}
			title={v.st === 'st-lock' ? blockedBadge(b?.blocked_by, s.names) : undefined}
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
			><span class="id" class:gold={cur} class:wid={paragraph}>{s.ref}</span>
			<span class="tx" class:strong={cur}
				>{@render parts(paragraph ? (b?.title ?? s.text) : s.text)}</span
			>
		</button>
		{#if showChip && rc.chip}<ObjectiveChip chip={rc.chip} small />{/if}
		{#if paragraph && v.done && !cur}
			<span class="fr">{whoDid(events.data, s.id, now)}</span>
		{:else if more.text}
			<span class="more" title={more.tip || undefined}>{more.text}</span>
		{/if}
	</div>
	{#if expanded === s.id}{@render expansion(s, seg)}{/if}
{/snippet}

<div class="wrap">
	{#if !narrow.value}
		<div class="full"><RouteTabs current={guideId} /></div>
		<aside class="nav-s">
			<p class="lbl pad">{paragraph ? 'Capítulos' : 'Seções'}</p>
			{#each sectionsNav as s (s.id)}
				<a class="secn" class:secn-on={s.id === sectionId} href={secHref(s.id)}
					><span class="i">{sectionLabel(s.id)}</span>{s.title}<span class="p"
						>{s.done}/{s.total}</span
					></a
				>
			{/each}
		</aside>
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
				<header class="nhead">
					<div class="pop">
						<button
							class="sel"
							type="button"
							aria-haspopup="menu"
							aria-expanded={menu === 'route'}
							onclick={() => (menu = menu === 'route' ? null : 'route')}
							>{label} <small>▾{route?.active ? ' ativa' : ''}</small></button
						>
						{#if menu === 'route'}
							<div class="menu panel" role="menu">
								{#each session.summary?.routes ?? [] as r (r.guide)}
									<a
										role="menuitem"
										href={resolve('/rotas/[guide]', { guide: r.guide })}
										onclick={() => (menu = null)}
										>{routeLabel(r.guide, r.title)}<span class="su mono"
											>{r.counts.done} / {r.counts.total}{r.active
												? ' · ativa'
												: r.cursor_ref
													? ` · em ${r.cursor_ref}`
													: ''}</span
										></a
									>
								{/each}
								<a role="menuitem" href={resolve('/compare')} onclick={() => (menu = null)}
									>Comparar</a
								>
							</div>
						{/if}
					</div>
					<div class="pop grow1">
						<button
							class="sel sel-s"
							type="button"
							aria-haspopup="listbox"
							aria-expanded={menu === 'section'}
							onclick={() => (menu = menu === 'section' ? null : 'section')}
							><span class="mono su tiny">{sectionLabel(sectionId)}</span><span class="stt"
								>{curSection?.title ?? section.data.title}</span
							><small>▾</small></button
						>
						{#if menu === 'section'}
							<div class="menu panel" role="listbox" aria-label="Seção">
								{#each sectionsNav as s (s.id)}
									<a
										role="option"
										aria-selected={s.id === sectionId}
										class:on={s.id === sectionId}
										href={secHref(s.id)}
										onclick={() => (menu = null)}
										><span class="mono su tiny">{sectionLabel(s.id)}</span>{s.title}<span
											class="su mono">{s.done}/{s.total}</span
										></a
									>
								{/each}
							</div>
						{/if}
					</div>
					{#if counts}
						<div class="nprog">
							<div class="xp"><span style="width: {pct(counts.done, counts.total)}%"></span></div>
							<span class="mono su tiny">{counts.done} / {counts.total}</span>
						</div>
					{/if}
				</header>
			{:else}
				<header class="head">
					<div class="txt">
						<p class="su small">
							{title} · {paragraph ? 'capítulo' : 'seção'}
							{sectionLabel(section.data.id).replace('cap. ', '')} · {AUTHORS[guideId] ?? ''}
						</p>
						<h1 class="sr">{section.data.title}</h1>
						{#if section.data.description}<p class="mu desc">{section.data.description}</p>{/if}
					</div>
					{#if counts}
						<div class="sum">
							<div class="sumh">
								<span class="su"
									>{counts.done} de {counts.total}{#if counts.implied}
										· {counts.implied} presumido{counts.implied === 1 ? '' : 's'}{/if}</span
								><span class="mono">{pct(counts.done, counts.total)}%</span>
							</div>
							<div class="xp"><span style="width: {pct(counts.done, counts.total)}%"></span></div>
							{#if (boardData?.counts.implied ?? 0) > 0}
								<button
									class="lnk sm"
									type="button"
									disabled={busy}
									title="Presumidos são passos que um passo posterior implica; confirmar vira feito à mão."
									onclick={confirmAll}>confirmar {boardData?.counts.implied} presumidos</button
								>
							{/if}
						</div>
					{/if}
				</header>
			{/if}

			{#if failure}
				<div class="band" role="alert">
					<Glyph name="alert" /><span>A marca não foi gravada: {failure}</span>
				</div>
			{/if}

			<div class="bar-f">
				<label class="inp" class:fullw={narrow.value}
					><Glyph name="search" size={14} /><input
						type="search"
						bind:value={query}
						placeholder={`buscar ${narrow.value ? '' : 'na rota: '}texto, item, local, quest`}
						aria-label="Buscar passos na rota"
					/>{#if searching}<span class="cnt"
							>{matches} de {total}{#if otherCount}
								· +{otherCount} em {elsewhere}{/if}</span
						>{/if}</label
				>
				<div class="seg" role="group" aria-label="Quais passos">
					{#each [['pending', 'Pendentes'], ['all', 'Todos'], ['objectives', 'Com objetivo']] as [k, l] (k)}
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
				{:else if !query.trim()}
					<span class="cnt">{matches} / {total}</span>
				{/if}
			</div>

			{#if cursorStep && route && !route.active}
				<div class="strip">
					<span class="st st-cur sd" aria-hidden="true"></span><span class="nw"
						>Você estaria em</span
					>
					<b>{cursorStep.ref}</b>
					· <span class="stx">{cursorStep.title}</span>
					<span class="push">
						{#if activeRoute}<span class="om"
								>o cursor da {routeLabel(activeRoute.guide, activeRoute.title)} fica guardado</span
							>{/if}
						<button class="btn btn-s btn-p" type="button" onclick={activate}>Tornar ativa</button>
					</span>
				</div>
			{:else if cursorStep && !cursorVisible}
				<button class="strip" type="button" aria-label="Ir ao passo atual" onclick={goCursor}>
					<span class="st st-cur sd" aria-hidden="true"></span><span class="nw">Você está em</span>
					<b>{cursorStep.ref}</b>
					· <span class="stx">{cursorStep.title}</span>
					<span class="push gold">ir ao passo ↓</span>
				</button>
			{/if}

			<div class="legs ruled">
				{#if narrowed && !matches}
					<p class="su empty">
						Nenhum passo {searching ? `com “${query.trim()}”` : 'com este filtro'}
						{paragraph ? 'neste capítulo' : 'nesta seção'}{otherCount
							? `; ${otherCount} em ${elsewhere}`
							: ''}.
					</p>
				{/if}
				{#each segments as seg, si (seg.id)}
					{@const visible = seg.steps.filter(shown)}
					{#if narrowed && !visible.length}
						<!-- trecho sem resultado: some enquanto a busca ou o filtro estiver ativo -->
					{:else if paragraph}
						{#if si > 0 && seg.kind === 'bank_leg' && !narrowed}
							<div class="bankline">{bankLine(segments[si - 1], seg)}</div>
						{/if}
						{#each visible as s (s.id)}{@render stepRow(s, seg)}{/each}
					{:else}
						{@const done = doneIn(seg)}
						{@const sum = legSummary(seg.steps, seg.range)}
						{@const o = isOpen(seg)}
						{@const start =
							seg.bankActions.find((a) => a.position === 'start') ?? seg.bankActions[0]}
						<button
							class="leg"
							class:leg-done={done === seg.steps.length}
							type="button"
							aria-expanded={o}
							onclick={() => (open[seg.id] = !o)}
						>
							<Glyph name="chevron" size={12} />{sum.title}
							<span class="ct" title={sum.full}
								><span class="rng"
									>{seg.range} · {seg.steps.length}
									{seg.steps.length === 1 ? 'passo' : 'passos'}</span
								>{#if sum.quests}<span class="sep">·</span
									>{sum.quests}{/if}{#if sum.places && !narrow.value}<span class="sep">·</span
									>{sum.places}{/if}</span
							>
							<span class="r"
								>{#if done < seg.steps.length && done > 0}<span class="pr"
										><span style="width: {pct(done, seg.steps.length)}%"></span></span
									>{/if}{narrowed
									? `${visible.length} de ${seg.steps.length}${query.trim() ? ` com “${query.trim()}”` : ''}`
									: `${done} / ${seg.steps.length}${done === seg.steps.length ? ' · feito' : ''}`}{#if start && !narrowed && !narrow.value}
									· banco no início: “{clip(
										start.text
									)}”{/if}{#if loadoutLabel(seg.loadout) && !narrow.value && !narrowed}
									· {loadoutLabel(seg.loadout)}{/if}</span
							>
						</button>
						{#if o}
							{@const w = windowOf(seg, visible)}
							{#each w.rows as s, i (s.id)}
								{#if s.location && (i === 0 || s.location !== w.rows[i - 1]?.location)}<div
										class="locl"
									>
										{s.location}
									</div>{/if}
								{@render stepRow(s, seg)}
							{/each}
							{#if w.before || w.after}
								<div class="row last">
									<span class="st hid"></span><span class="id">…</span>
									<span class="tx su"
										>{[
											w.before ? `${w.before} antes de ${w.rows[0]?.ref}` : '',
											w.after ? `mais ${w.after} passos até ${visible.at(-1)?.ref}` : ''
										]
											.filter(Boolean)
											.join(' e ')}{visible.at(-1)?.location
											? ` · ${visible.at(-1)?.location}`
											: ''}</span
									>
									<button class="lnk more" type="button" onclick={() => (showAll[seg.id] = true)}
										>mostrar tudo</button
									>
								</div>
							{/if}
						{/if}
					{/if}
				{/each}
				{#if othersLoading}
					<p class="su tiny pad">buscando em {elsewhere}…</p>
				{:else if otherHits.length}
					<p class="lbl pad oth">Em {elsewhere} · {otherCount}</p>
					{#each otherHits as h (h.sec.id)}
						<a class="leg" href={secHref(h.sec.id)}
							><span class="mono rng">{sectionLabel(h.sec.id)}</span>{h.sec.title}<span class="r"
								>{h.steps.length} com “{query.trim()}”</span
							></a
						>
						{#each h.steps.slice(0, OTHER_LIMIT) as s (s.id)}
							{@const b = boardById.get(s.id)}
							{@const v = statusView(b?.status, s.id === cursorId, !!b?.blocked_by?.length)}
							<a
								class="row"
								class:row-done={v.done}
								class:row-skip={b?.status === 'skipped'}
								href={`${secHref(h.sec.id)}#passo-${s.id}`}
								><span class="st {v.st}"
									>{#if v.done}<Glyph name="check" size={10} />{/if}</span
								><span class="id" class:wid={paragraph}>{s.ref}</span><span class="tx"
									>{@render parts(snippet(s.text, query))}</span
								></a
							>
						{/each}
						{#if h.steps.length > OTHER_LIMIT}
							<div class="row last">
								<span class="st hid"></span><span class="id">…</span>
								<a class="lnk more" href={secHref(h.sec.id)}
									>mais {h.steps.length - OTHER_LIMIT} em {sectionLabel(h.sec.id)}</a
								>
							</div>
						{/if}
					{/each}
				{/if}
			</div>
			<p class="su tiny">
				{#if guideId.startsWith('bruhsailer')}BRUHsailer por So Iron BRUH e ParasailerOSRS.{:else}Guia
					por OzirisLoL, versão interativa de ironman.guide.{/if} Uso pessoal; atribuição mantida.
				{ICON_ATTRIBUTION}.
			</p>
		{/if}
	</section>
</div>

<style>
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.full {
		flex: 1 1 100%;
	}
	.nav-s {
		flex: 1 1 200px;
		max-width: 230px;
		display: flex;
		flex-direction: column;
		gap: 2px;
		position: sticky;
		top: 16px;
	}
	.pad {
		padding: 6px 8px;
	}
	.content {
		flex: 999 1 560px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 10px 24px;
	}
	.txt {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1 1 380px;
		min-width: 0;
	}
	.txt p,
	.desc {
		margin: 0;
	}
	.txt h1 {
		font-size: 28px;
	}
	.desc {
		max-width: 70ch;
	}
	.sum {
		display: flex;
		flex-direction: column;
		gap: 6px;
		flex: 0 1 260px;
		min-width: 220px;
	}
	.sumh {
		display: flex;
		justify-content: space-between;
		font-size: 12px;
	}
	.lnk.sm {
		min-height: 0;
		padding: 0;
		font-size: 12px;
		border: 0;
		background: none;
		cursor: pointer;
		align-self: flex-start;
	}
	.bar-f {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
	}
	.bar-f .inp {
		flex: 1 1 280px;
	}
	.fullw {
		flex: 1 1 100%;
	}
	button.strip {
		width: 100%;
		border: 0;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.sd {
		width: 12px;
		height: 12px;
		box-shadow: inset 0 0 0 2px var(--frame);
	}
	.nw {
		white-space: nowrap;
	}
	.stx {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.push {
		margin-left: auto;
		display: inline-flex;
		gap: 14px;
		align-items: center;
		flex: 0 0 auto;
	}
	.gold {
		color: var(--gold);
	}
	.om {
		color: var(--on-frame-muted);
		font-size: 12px;
	}
	.legs {
		display: flex;
		flex-direction: column;
	}
	button.leg {
		width: 100%;
		border: 0;
		text-align: left;
		cursor: pointer;
	}
	.leg :global(svg) {
		transition: transform 0.15s;
	}
	.leg[aria-expanded='true'] :global(svg) {
		transform: rotate(90deg);
	}
	.rng {
		color: var(--on-frame-muted);
	}
	.ruled .row {
		border-bottom: 1px solid var(--hairline);
		border-radius: 0;
		gap: 10px;
	}
	.ruled .row .tx {
		white-space: normal;
		line-height: 1.35;
		padding: 6px 0;
	}
	.hit {
		display: flex;
		align-items: center;
		gap: 12px;
		flex: 1 1 auto;
		min-width: 0;
		min-height: 44px;
		padding: 0;
		border: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.id.gold {
		color: var(--gold-ink);
	}
	.id.wid {
		width: 56px;
	}
	.tx.strong {
		font-weight: 500;
	}
	.row-skip .tx {
		color: var(--ink-subtle);
	}
	.hid {
		visibility: hidden;
	}
	.lnk.more {
		border: 0;
		background: none;
		cursor: pointer;
		min-height: 0;
		font:
			500 11px 'JetBrains Mono',
			monospace;
	}
	.last {
		border-bottom: 0 !important;
	}
	.metas {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.note {
		margin: 0;
	}
	.nl {
		margin-right: 6px;
	}
	.note-up .nl {
		color: var(--stale);
	}
	.its {
		margin: 0;
	}
	.its .lbl {
		margin-right: 6px;
	}
	/* Cores do autor no texto (BRUHsailer: "cheapest in red", rota da Phoenix em laranja). */
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
	.box {
		padding: 10px 14px;
		border-radius: 4px;
		border: 1px solid var(--hairline);
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px 4px;
	}
	.acts .btn {
		height: 40px;
	}
	.grow {
		flex: 1;
	}
	.empty {
		margin: 12px;
	}
	.oth {
		margin: 18px 0 0;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	/* Estreito (V6Rota640): rota e seção como seletores no topo. */
	.nhead {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
	}
	.pop {
		position: relative;
	}
	.grow1 {
		flex: 1 1 200px;
		min-width: 0;
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
		min-width: 260px;
		max-width: calc(100vw - 28px);
		max-height: 60vh;
		overflow: auto;
		display: flex;
		flex-direction: column;
		padding: 4px;
		box-shadow:
			var(--bevel),
			0 8px 24px rgba(0, 0, 0, 0.25);
	}
	.menu a {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 40px;
		padding: 0 10px;
		color: var(--ink);
		text-decoration: none;
		font-size: 13px;
		border-bottom: 1px solid var(--hairline);
	}
	.menu a .su {
		margin-left: auto;
		font-size: 11px;
	}
	.menu a.on {
		background: var(--gold-tint);
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.nprog {
		display: flex;
		align-items: center;
		gap: 10px;
		flex: 1 1 100%;
	}
	.nprog .xp {
		flex: 1;
	}
</style>
