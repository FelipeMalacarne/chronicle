<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { theme, toggleTheme } from '#lib/progress/theme.svelte';
	import { ago } from '#lib/progress/view';
	import type { ProgressSummary } from '#lib/progress/types';

	let { summary, account }: { summary: ProgressSummary | null; account?: string } = $props();

	const path = $derived(page.url.pathname);
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});
	const STALE_MS = 24 * 3600_000;
	const LABEL: Record<string, string> = {
		plugin: 'plugin',
		hiscores: 'hiscores',
		wom: 'wise old man',
		wikisync: 'wikisync'
	};
	const sources = $derived(summary?.sources ?? []);
	/* Na barra estreita só a fonte mais recente; as outras no title e na Conta (v6). */
	const latest = $derived(
		[...sources].sort((a, b) => Date.parse(b.last_observed_at) - Date.parse(a.last_observed_at))[0]
	);
	const allSources = $derived(
		sources
			.map((s) => `${LABEL[s.source] ?? s.source} há ${ago(Date.parse(s.last_observed_at), now)}`)
			.join(' · ')
	);

	const items = $derived([
		{ href: resolve('/'), label: 'Agora', icon: 'agora' as const, on: path === '/' },
		{
			href: resolve('/rotas'),
			label: 'Rotas',
			icon: 'rotas' as const,
			on: path.startsWith('/rotas'),
			n: summary?.routes.length
		},
		{
			href: resolve('/mundo'),
			label: 'Mundo',
			icon: 'mundo' as const,
			on: path.startsWith('/mundo')
		},
		{
			href: resolve('/conta'),
			label: 'Conta',
			icon: 'conta' as const,
			on: path.startsWith('/conta')
		}
	]);
</script>

<nav class="side" aria-label="Principal">
	<div class="brand">
		<a href={resolve('/')} class="sr">Ironman Planner</a>
		<a class="mono acct" href={resolve('/setup')} title="Trocar ou criar conta"
			>{account ?? 'sem conta'} · trocar</a
		>
	</div>
	{#each items as it (it.label)}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- hrefs dos itens vêm de resolve() -->
		<a class="nav" class:nav-on={it.on} href={it.href} aria-current={it.on ? 'page' : undefined}>
			<Glyph name={it.icon} />{it.label}
			{#if it.n}<span class="n">{it.n}</span>{/if}
		</a>
	{/each}
	<div class="grow"></div>
	{#if latest}
		{@const at = Date.parse(latest.last_observed_at)}
		<a class="nav one-src" href={resolve('/conta')} title={allSources}
			><span class="dot" class:dot-w={now - at > STALE_MS}></span>{LABEL[latest.source] ??
				latest.source}
			{ago(at, now)}</a
		>
	{/if}
	<p class="ey fontes">Fontes</p>
	{#each sources as s (s.source)}
		{@const at = Date.parse(s.last_observed_at)}
		<div class="src">
			<span class="dot" class:dot-w={now - at > STALE_MS}></span>{LABEL[s.source] ?? s.source} · {ago(
				at,
				now
			)}
		</div>
	{:else}
		<div class="src"><span class="dot dot-off"></span>nenhuma fonte ainda</div>
	{/each}
	<button class="sbtn tema" type="button" onclick={toggleTheme}>
		<Glyph name="moon" size={14} />
		<span>Tema: {theme.value}</span><span class="small">trocar</span>
	</button>
</nav>

<style>
	.side {
		position: sticky;
		top: 0;
		height: 100vh;
	}
	.brand {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 4px 10px 14px;
	}
	.brand a {
		font-size: 19px;
		color: var(--gold);
		text-decoration: none;
	}
	.brand .acct {
		font-size: 11px;
		color: var(--on-frame-muted);
		text-decoration: none;
	}
	.brand .acct:hover {
		color: var(--on-frame);
	}
	.grow {
		flex: 1;
	}
	.fontes {
		padding: 8px 10px 4px;
	}
	.tema {
		margin-top: 10px;
	}
	.tema span:first-of-type {
		flex: 1;
	}
	.small {
		font-size: 11px;
	}
	.one-src {
		display: none;
		gap: 6px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
	}
	@media (max-width: 1100px) {
		.one-src {
			display: flex;
			margin-left: auto;
		}
		.side {
			position: static;
			height: auto;
			width: 100%;
			flex-basis: 100%;
			flex-direction: row;
			flex-wrap: wrap;
			align-items: center;
			padding: 10px 12px;
		}
		.brand {
			padding: 0 10px 0 0;
		}
		.grow,
		.fontes {
			display: none;
		}
		.tema {
			margin: 0;
		}
	}
</style>
