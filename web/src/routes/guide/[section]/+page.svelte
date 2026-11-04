<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { api } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { groupByLocation, hasAutoRule, ruleBadges } from '#lib/api/rules';
	import type { Mode, Now, Section, Step } from '#lib/api/types';
	import PageHead from '#lib/components/PageHead.svelte';
	import ChecklistRow from '#lib/components/ChecklistRow.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import Icon from '#lib/components/Icon.svelte';

	const slug = $derived(page.params.section ?? '');
	const res = createResource<Section>(() => api.section(slug));
	const nowRes = createResource<Now>(() => api.now());
	const currentId = $derived(nowRes.data?.current.id ?? null);

	// Filtros (lembrados neste navegador).
	type LayerFilter = 'all' | 'classic';
	type ModeFilter = Extract<Mode, 'ironman' | 'hcim'>;
	const FKEY = 'ironman-planner:guide-filters';
	type SavedFilters = { layer?: LayerFilter; mode?: ModeFilter; onlyAuto?: boolean };
	function loadFilters(): SavedFilters {
		try {
			return (JSON.parse(localStorage.getItem(FKEY) ?? 'null') as SavedFilters | null) ?? {};
		} catch {
			return {};
		}
	}
	const saved = loadFilters();
	let layer = $state<LayerFilter>(saved.layer ?? 'all');
	let mode = $state<ModeFilter>(saved.mode ?? 'ironman');
	let onlyAuto = $state(saved.onlyAuto ?? false);
	let q = $state('');
	$effect(() => {
		const v = JSON.stringify({ layer, mode, onlyAuto });
		try {
			localStorage.setItem(FKEY, v);
		} catch {
			/* ignora */
		}
	});

	function matches(s: Step, needle: string): boolean {
		if (!needle) return true;
		const hay = [
			s.id,
			s.text,
			s.location ?? '',
			s.note ?? '',
			...(s.items ?? []),
			...ruleBadges(s).map((b) => b.label),
			s.quest?.name ?? ''
		]
			.join(' ')
			.toLowerCase();
		return hay.includes(needle);
	}

	const filtered = $derived.by(() => {
		const steps = res.data?.steps ?? [];
		const needle = q.trim().toLowerCase();
		return steps.filter(
			(s) =>
				(layer === 'all' || s.layer === 'classic') &&
				s.modes.includes(mode) &&
				(!onlyAuto || hasAutoRule(s)) &&
				matches(s, needle)
		);
	});
	const groups = $derived(groupByLocation(filtered));
	const total = $derived(res.data?.steps.length ?? 0);

	// Rolar até #step-<id> quando vier do Agora.
	$effect(() => {
		if (!res.data || !page.url.hash) return;
		const hash = page.url.hash;
		tick().then(() => {
			const el = document.querySelector(hash);
			el?.scrollIntoView({ block: 'center' });
		});
	});
</script>

<svelte:head>
	<title>{res.data ? `${res.data.id} ${res.data.title}` : 'Seção'} · Ironman Planner</title>
</svelte:head>

{#if res.error}
	<ErrorState error={res.error} retry={res.reload} />
{:else if res.loading || !res.data}
	<div class="card"><Skeleton lines={6} height={16} /></div>
{:else}
	<PageHead
		eyebrow={`Guia · seção ${res.data.id}${res.data.track ? ' · track' : ''}`}
		title={res.data.title}
		description={res.data.description}
	>
		{#snippet actions()}
			<a class="btn btn-ghost btn-compact" href={resolve('/guide')}>Todas as seções</a>
		{/snippet}
	</PageHead>

	<form class="filters card" role="search" onsubmit={(e) => e.preventDefault()}>
		<fieldset class="seg">
			<legend class="visually-hidden">Camada</legend>
			<label class:on={layer === 'all'}>
				<input type="radio" name="layer" value="all" bind:group={layer} />
				Clássico + Enhanced
			</label>
			<label class:on={layer === 'classic'}>
				<input type="radio" name="layer" value="classic" bind:group={layer} />
				Só clássico
			</label>
		</fieldset>

		<fieldset class="seg">
			<legend class="visually-hidden">Modo</legend>
			<label class:on={mode === 'ironman'}>
				<input type="radio" name="mode" value="ironman" bind:group={mode} />
				Ironman
			</label>
			<label class:on={mode === 'hcim'}>
				<input type="radio" name="mode" value="hcim" bind:group={mode} />
				HCIM
			</label>
		</fieldset>

		<label class="check">
			<input type="checkbox" bind:checked={onlyAuto} />
			só com regra automática
		</label>

		<label class="search">
			<span class="visually-hidden">Buscar passos</span>
			<Icon name="search" size={14} />
			<input class="input" type="search" placeholder="buscar texto, item, local…" bind:value={q} />
		</label>

		<span class="data count" aria-live="polite">{filtered.length}/{total}</span>
	</form>

	{#if groups.length === 0}
		<p class="muted empty">Nenhum passo com esses filtros.</p>
	{:else}
		{#each groups as g, gi (g.location + gi)}
			<section class="group" aria-labelledby="g-{gi}">
				<div class="ghead">
					<h2 id="g-{gi}" class="eyebrow gtitle">
						<Icon name="pin" size={11} />
						{g.location}
					</h2>
					<span class="data gcount">{g.steps.length}</span>
				</div>
				<hr class="divider" />
				<ul class="rows">
					{#each g.steps as s (s.id)}
						<ChecklistRow step={s} current={s.id === currentId} />
					{/each}
				</ul>
			</section>
		{/each}
	{/if}
{/if}

<style>
	.filters {
		position: sticky;
		top: calc(var(--topbar-h) + 8px);
		z-index: 5;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		margin-bottom: var(--sp-5);
	}
	.seg {
		display: inline-flex;
		gap: 2px;
		margin: 0;
		padding: 3px;
		border: 1px solid var(--border);
		border-radius: var(--r-button);
		background: var(--bg);
	}
	.seg label {
		display: inline-flex;
		align-items: center;
		height: 26px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-chip);
		font-size: var(--fs-control);
		font-weight: 500;
		color: var(--muted);
		cursor: pointer;
	}
	.seg label.on {
		background: var(--elevated);
		color: var(--text);
	}
	.seg label:focus-within {
		box-shadow: var(--focus-ring);
	}
	.seg input {
		position: absolute;
		opacity: 0;
		width: 1px;
		height: 1px;
	}
	.check {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: var(--fs-control);
		color: var(--muted);
		cursor: pointer;
	}
	.check input {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
		margin: 0;
	}
	.search {
		position: relative;
		flex: 1 1 200px;
		display: flex;
		align-items: center;
		color: var(--subtle);
	}
	.search :global(svg) {
		position: absolute;
		left: 10px;
		pointer-events: none;
	}
	.search .input {
		width: 100%;
		padding-left: 30px;
	}
	.count {
		color: var(--subtle);
	}
	.empty {
		padding: var(--sp-6) 0;
	}
	.group {
		margin-bottom: var(--sp-6);
	}
	.ghead {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--sp-4) 6px;
	}
	.gtitle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-weight: 400;
	}
	.gcount {
		color: var(--subtle);
	}
	.rows {
		list-style: none;
		margin: 6px 0 0;
		padding: 0;
		display: grid;
		gap: 2px;
	}
	@media (max-width: 719px) {
		.filters {
			position: static;
		}
	}
</style>
