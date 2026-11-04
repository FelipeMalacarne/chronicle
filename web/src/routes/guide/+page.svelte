<script lang="ts">
	import { resolve } from '$app/paths';
	import { api } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import type { SectionSummary } from '#lib/api/types';
	import PageHead from '#lib/components/PageHead.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import Icon from '#lib/components/Icon.svelte';

	const res = createResource<SectionSummary[]>(() => api.guide());
	// Progresso por seção depende do banco; por enquanto 0.
	const done = () => 0;
</script>

<svelte:head>
	<title>Guia · Ironman Planner</title>
</svelte:head>

<PageHead
	eyebrow="Guia · seções"
	title="Guia"
	description="Oziris Ironman Efficiency Guide, seção a seção. O progresso aparece quando o banco entrar."
/>

{#if res.error}
	<ErrorState error={res.error} retry={res.reload} />
{:else if res.loading}
	<div class="card"><Skeleton lines={5} height={16} /></div>
{:else if res.data}
	<ul class="sections">
		{#each res.data as s (s.id)}
			{@const d = done()}
			{@const pct = s.steps ? Math.round((d / s.steps) * 100) : 0}
			<li>
				<a class="tile" href={resolve('/guide/[section]', { section: s.slug })}>
					<div class="top">
						<span class="data sid">{s.id}</span>
						{#if s.track}<span class="badge">track</span>{/if}
						<span class="data count">{d}/{s.steps}</span>
					</div>
					<h2 class="heading stitle">{s.title}</h2>
					<div
						class="bar"
						role="progressbar"
						aria-valuemin="0"
						aria-valuemax={s.steps}
						aria-valuenow={d}
						aria-label="Progresso da seção {s.id}"
					>
						<span style:width="{pct}%"></span>
					</div>
					<span class="go caption">abrir checklist <Icon name="chevron" size={12} /></span>
				</a>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.sections {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: var(--sp-4);
	}
	.tile {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-5);
		border-radius: var(--r-tile);
		background: var(--surface);
		border: 1px solid var(--border);
		color: var(--text);
		height: 100%;
	}
	.tile:hover {
		text-decoration: none;
		border-color: var(--overlay);
		background: var(--elevated);
	}
	.tile:focus-visible {
		border-radius: var(--r-tile);
	}
	.top {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}
	.sid {
		color: var(--accent);
	}
	.count {
		margin-left: auto;
		color: var(--muted);
	}
	.stitle {
		font-size: 19px;
	}
	.bar {
		height: 6px;
		border-radius: 3px;
		background: var(--bg);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
		border-radius: 3px;
	}
	.go {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
</style>
