<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { session } from '#lib/progress/session.svelte';
	import { ago, entityVerdict, KIND_LABEL, parseKey, routeLabel } from '#lib/progress/view';
	import type { Goal, WorldEntity } from '#lib/progress/types';
	import EntityPanel from '#lib/v6/EntityPanel.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import MundoHead from '#lib/v6/MundoHead.svelte';

	/*
	 * Mundo › Entidades (V6Mundo1440 e V6Mundo640): a lista "no seu caminho agora" (o que a rota
	 * ativa toca a partir do cursor) e "feitas há pouco"; à direita o mesmo painel de entidade das
	 * outras telas. No estreito a entidade abre como folha de baixo. Metas vazias = uma linha.
	 */
	const account = $derived(session.account);
	const world = createResource(
		() =>
			account ? progressApi.world(account) : Promise.resolve({ entities: [] as WorldEntity[] }),
		{ tags: ['progress'] }
	);
	const goals = createResource(
		() => (account ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);
	const routes = $derived(session.summary?.routes ?? []);
	const active = $derived(routes.find((r) => r.active));
	const events = createResource(
		() =>
			account && active
				? progressApi.events(active.guide, account, 80).catch(() => null)
				: Promise.resolve(null),
		{ tags: ['progress'] }
	);
	const label = (guide: string) =>
		routeLabel(guide, routes.find((r) => r.guide === guide)?.title ?? guide);

	const KINDS: [string, string][] = [
		['', 'Tudo'],
		['quest', 'Quest'],
		['item', 'Item'],
		['skill', 'Skill'],
		['diary', 'Diary'],
		['unlock', 'Unlock']
	];
	let kind = $state('');
	// ?q= vem da busca das Metas da frota.
	let query = $state(page.url.searchParams.get('q') ?? '');
	const all = $derived(world.data?.entities ?? []);
	const byRef = $derived(new Map(all.map((e) => [e.ref, e])));
	const searching = $derived(query.trim() !== '' || kind !== '');

	/* No seu caminho agora: o que a rota ativa toca na seção do cursor, a partir dele. */
	const path = $derived.by(() => {
		if (!active?.cursor_section) return [];
		const cur = active.cursor?.step_id;
		let from = 0;
		for (const e of all)
			for (const r of e.routes)
				if (r.guide === active.guide && r.step === cur && r.position) from = r.position;
		return all
			.map((e) => {
				const pos = e.routes
					.filter(
						(r) =>
							r.guide === active.guide &&
							r.section === active.cursor_section &&
							(r.position ?? 0) >= from
					)
					.map((r) => r.position ?? 0);
				return { e, pos: pos.length ? Math.min(...pos) : -1 };
			})
			.filter((x) => x.pos >= 0)
			.sort((a, b) => a.pos - b.pos)
			.slice(0, 14)
			.map((x) => x.e);
	});
	/* Feitas há pouco: objetivos que viraram met nas últimas 24 h. */
	const now = Date.now();
	const recent = $derived.by(() => {
		const out: { e: WorldEntity; source: string; at: number }[] = [];
		for (const ev of events.data?.objectives ?? []) {
			const at = Date.parse(ev.at);
			if (ev.to !== 'met' || now - at > 24 * 3600_000) continue;
			const ref = parseKey(ev.key)?.ref;
			const e = ref ? byRef.get(ref) : undefined;
			if (!e || out.some((x) => x.e.ref === e.ref)) continue;
			out.push({ e, source: ev.source ?? 'fonte', at });
		}
		return out.slice(0, 8);
	});
	const results = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return all
			.filter((e) => !kind || e.kind === kind)
			.filter((e) => !q || e.name.toLowerCase().includes(q) || e.ref.includes(q))
			.slice(0, 60);
	});

	const selectedRef = $derived(
		page.url.searchParams.get('e') ?? (searching ? results[0]?.ref : path[0]?.ref) ?? null
	);
	function pick(ref: string) {
		if (narrow.value) {
			openEntity(ref);
			return;
		}
		const url = new URL(page.url.href);
		url.searchParams.set('e', ref);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- mesma página, só a query muda
		void goto(url, { replace: true, reset: false });
	}
	const pinned = $derived((goals.data?.goals ?? []).filter((g) => g.status === 'active'));

	const V_CLS = { met: 'v-met', prog: 'v-prog', unmet: 'v-unmet', unknown: 'v-unk' } as const;
	function hint(e: WorldEntity): string {
		const guides = new Set(e.routes.map((r) => r.guide));
		const here = active && e.routes.some((r) => r.step === active.cursor?.step_id);
		const n =
			guides.size > 1 ? `${guides.size} rotas` : guides.size === 1 ? label([...guides][0]) : '';
		return [n, here ? 'você está aqui' : ''].filter(Boolean).join(' · ');
	}
</script>

<svelte:head>
	<title>Mundo · Chronicle</title>
</svelte:head>

{#snippet ent(e: WorldEntity, w: string)}
	{@const v = entityVerdict(e.objectives)}
	{@const icon = iconFor(e.ref)}
	<button
		class="ent"
		class:ent-on={!narrow.value && e.ref === selectedRef}
		type="button"
		aria-current={!narrow.value && e.ref === selectedRef ? 'true' : undefined}
		onclick={() => pick(e.ref)}
	>
		<span class="v {V_CLS[v]}"
			>{#if v === 'met'}<Glyph name="check" size={9} />{:else if v === 'unknown'}?{/if}</span
		><span
			>{#if icon}<img class="ski" src={icon} alt="" />{/if}</span
		><span class="nm">{e.name}<span class="su k">{KIND_LABEL[e.kind] ?? e.kind}</span></span><span
			class="w">{w}</span
		>
	</button>
{/snippet}

<MundoHead title="Entidades e metas" current="entidades">
	<label class="inp grow"
		><Glyph name="search" size={14} /><input
			type="search"
			bind:value={query}
			placeholder="quest, item, skill, diary, unlock…"
			aria-label="Buscar entidade"
		/></label
	>
	<div class="seg" role="group" aria-label="Tipo">
		{#each KINDS as [k, l] (k)}
			<button
				type="button"
				class:on={kind === k}
				aria-pressed={kind === k}
				onclick={() => (kind = k)}>{l}</button
			>
		{/each}
	</div>
</MundoHead>

<div class="foot gl">
	{#if pinned.length}
		<a href={resolve('/mundo/metas')}
			>{pinned.length}
			{pinned.length === 1 ? 'meta fixada' : 'metas fixadas'}: {pinned
				.map((g) => g.label)
				.join(', ')}</a
		>
	{:else}
		<a href={resolve('/mundo/metas')}>Nenhuma meta fixada</a>
	{/if}
</div>

{#if world.error}
	<div class="band" role="alert">
		<Glyph name="alert" /><span>O Mundo não carregou: {world.error.message}</span>
	</div>
{/if}

<div class="split">
	<section class="panel listp">
		{#if searching}
			<div class="gh2">
				<p class="lbl">Resultados</p>
				<span class="su small">{results.length}{results.length === 60 ? '+' : ''}</span>
			</div>
			{#each results as e (e.ref)}{@render ent(e, hint(e))}{:else}
				<p class="su small pad">Nenhuma entidade com esse nome.</p>
			{/each}
		{:else}
			<div class="gh2">
				<p class="lbl">No seu caminho agora</p>
				{#if active}<span class="su small"
						>{label(active.guide)}{active.cursor_ref
							? `, a partir de ${active.cursor_ref}`
							: ''}</span
					>{/if}
			</div>
			{#each path as e (e.ref)}{@render ent(e, hint(e))}{:else}
				<p class="su small pad">{world.loading ? 'Carregando…' : 'Nada no caminho do cursor.'}</p>
			{/each}
			{#if recent.length}
				<div class="gh2">
					<p class="lbl">Feitas há pouco</p>
					<span class="su small">últimas 24 h</span>
				</div>
				{#each recent as r (r.e.ref)}{@render ent(
						r.e,
						`${r.source} · ${ago(r.at, now).replace('há ', '')}`
					)}{/each}
			{/if}
		{/if}
	</section>
	{#if !narrow.value}
		<section class="detail">
			{#if selectedRef}
				{#key selectedRef}<EntityPanel inline refOverride={selectedRef} />{/key}
			{:else}
				<p class="su small">Escolha uma entidade na lista.</p>
			{/if}
		</section>
	{/if}
</div>

<p class="su tiny">{ICON_ATTRIBUTION}.</p>

<style>
	.grow {
		flex: 1 1 260px;
	}
	.gl {
		padding: 0;
	}
	.split {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.listp {
		flex: 1 1 340px;
		max-width: 460px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		padding: 4px 8px 8px;
	}
	@media (max-width: 900px) {
		.listp {
			max-width: none;
		}
	}
	.detail {
		flex: 999 1 520px;
		min-width: 0;
	}
	button.ent {
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
	button.ent-on {
		background: var(--gold-tint);
	}
	.nm {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.k {
		font-size: 11px;
		margin-left: 6px;
	}
	.pad {
		padding: 8px 10px;
		margin: 0;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
</style>
