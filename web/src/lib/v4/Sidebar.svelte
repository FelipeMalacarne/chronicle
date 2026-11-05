<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import Menu from './Menu.svelte';
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
	// Compact bar (below 1100 px): the sources become dots with one accessible summary.
	const sourceLine = $derived(
		sources.length
			? sources
					.map(
						(s) =>
							`${LABEL[s.source] ?? s.source} ${ago(Date.parse(s.last_observed_at), now)}${now - Date.parse(s.last_observed_at) > STALE_MS ? ' (velha)' : ''}`
					)
					.join(' · ')
			: 'nenhuma fonte ainda'
	);
	const menuItems = $derived([
		{ label: `Conta: ${account ?? 'sem conta'}`, hint: 'trocar', href: resolve('/setup') },
		{ label: 'Tema', hint: theme.value, onselect: toggleTheme }
	]);

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
	<!-- Compact bar only: sources as dots (filled = fresh, ring = stale, dotted = none). -->
	<a
		class="dots bar"
		href={resolve('/conta')}
		title={`Fontes: ${sourceLine}`}
		aria-label={`Fontes: ${sourceLine}`}
	>
		{#each sources as s (s.source)}
			<span class="dot" class:dot-w={now - Date.parse(s.last_observed_at) > STALE_MS}></span>
		{:else}
			<span class="dot dot-off"></span>
		{/each}
	</a>
	<span class="bar"><Menu label="Conta e tema" items={menuItems} cls="sbtn more" /></span>
	<p class="ey fontes">Fontes</p>
	{#each sources as s (s.source)}
		{@const at = Date.parse(s.last_observed_at)}
		<div class="src wide">
			<span class="dot" class:dot-w={now - at > STALE_MS}></span>{LABEL[s.source] ?? s.source} · {ago(
				at,
				now
			)}
		</div>
	{:else}
		<div class="src wide"><span class="dot dot-off"></span>nenhuma fonte ainda</div>
	{/each}
	<button class="sbtn tema wide" type="button" onclick={toggleTheme}>
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
	.bar {
		display: none;
	}
	/* Stale is a ring, not only a color: status by shape. */
	.dots .dot-w {
		background: transparent;
		border: 2px solid var(--stale);
		box-sizing: border-box;
	}
	.dots {
		align-items: center;
		gap: 4px;
		min-height: 36px;
		padding: 0 8px;
		border-radius: 4px;
		text-decoration: none;
	}
	.dots:hover {
		background: var(--frame-2);
	}
	.dots .dot {
		width: 9px;
		height: 9px;
	}
	:global(.sbtn.more) {
		width: 36px;
		justify-content: center;
		padding: 0;
	}
	@media (max-width: 1100px) {
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
		.side {
			flex-wrap: nowrap;
			gap: 2px;
			padding: 6px 12px;
		}
		.brand .acct,
		.wide,
		.fontes {
			display: none;
		}
		.bar {
			display: inline-flex;
		}
		.brand a.sr {
			font-size: 17px;
			white-space: nowrap;
		}
	}
	@media (max-width: 719px) {
		.side {
			padding: 4px 8px;
		}
		.side .nav {
			height: 44px;
			gap: 6px;
			padding: 0 8px;
		}
		.side .nav :global(.n) {
			display: none;
		}
		.dots,
		:global(.sbtn.more) {
			min-height: 44px;
			min-width: 44px;
			justify-content: center;
		}
	}
	@media (max-width: 560px) {
		.brand {
			display: none;
		}
	}
</style>
