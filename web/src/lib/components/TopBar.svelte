<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { sync, relativeTime } from '#lib/api/client';
	import { guideSwitch } from '#lib/planner/header.svelte';
	import GuideSelector from './GuideSelector.svelte';
	import Icon, { type IconName } from './Icon.svelte';

	const nav = [
		{ href: '/', label: 'Agora', icon: 'clock', phone: true },
		{ href: '/plan', label: 'Plano', icon: 'note', phone: true },
		{ href: '/guide', label: 'Guia', icon: 'book', phone: true },
		{ href: '/compare', label: 'Comparar', icon: 'columns', phone: true },
		{ href: '/account', label: 'Conta', icon: 'user', phone: true },
		{ href: '/goals', label: 'Metas', icon: 'target', phone: false },
		{ href: '/graph', label: 'Grafo', icon: 'graph', phone: false }
	] as const satisfies readonly { href: string; label: string; icon: IconName; phone: boolean }[];

	function isActive(href: string): boolean {
		const p = page.url.pathname;
		return href === '/' ? p === '/' : p === href || p.startsWith(href + '/');
	}

	// Relógio para o "há X min" andar sem nova requisição.
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 15_000);
		return () => clearInterval(t);
	});

	const syncText = $derived.by(() => {
		switch (sync.state) {
			case 'ok':
				return `sincronizado ${relativeTime(sync.lastOkAt, now)}`;
			case 'stale':
				return `desatualizado · último ${relativeTime(sync.lastOkAt, now)}`;
			case 'offline':
				return 'sem conexão com o servidor';
			case 'loading':
				return 'sincronizando…';
			default:
				return 'aguardando';
		}
	});
</script>

<header class="topbar">
	<a class="brand" href={resolve('/')}>IRONMAN PLANNER</a>

	{#if guideSwitch.onchange && guideSwitch.guides.length > 1}
		<GuideSelector
			guides={guideSwitch.guides}
			value={guideSwitch.value}
			label="Guia ativo"
			onchange={guideSwitch.onchange}
		/>
	{/if}

	<nav class="pills" aria-label="Principal">
		{#each nav as item (item.href)}
			<a
				class="pill"
				href={resolve(item.href)}
				aria-current={isActive(item.href) ? 'page' : undefined}
			>
				{item.label}
			</a>
		{/each}
	</nav>

	<div class="sync" data-state={sync.state} title={sync.lastError ?? undefined}>
		<span class="dot" aria-hidden="true"></span>
		<span class="sync-text">{syncText}</span>
	</div>
</header>

<nav class="bottombar" aria-label="Principal (celular)">
	{#each nav.filter((n) => n.phone) as item (item.href)}
		<a
			class="tab"
			href={resolve(item.href)}
			aria-current={isActive(item.href) ? 'page' : undefined}
		>
			<Icon name={item.icon} size={18} />
			<span>{item.label}</span>
		</a>
	{/each}
</nav>

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 10;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px var(--sp-6);
		min-height: var(--topbar-h);
		padding: 6px var(--sp-6);
		background: var(--surface);
		border-bottom: 1px solid var(--divider);
	}
	.brand {
		font-family: var(--font-mono);
		font-size: var(--fs-eyebrow);
		letter-spacing: 1px;
		color: var(--accent);
	}
	.brand:hover {
		text-decoration: none;
		color: var(--text);
	}
	.pills {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.pill {
		display: inline-flex;
		align-items: center;
		height: 30px;
		padding: 0 var(--sp-3);
		border-radius: 15px;
		font-size: var(--fs-control);
		font-weight: 500;
		color: var(--muted);
	}
	.pill:hover {
		text-decoration: none;
		color: var(--text);
	}
	.pill[aria-current='page'] {
		background: var(--elevated);
		color: var(--text);
	}
	.sync {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		font-family: var(--font-mono);
		font-size: var(--fs-data);
		color: var(--muted);
		white-space: nowrap;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--subtle);
	}
	.sync[data-state='ok'] .dot {
		background: var(--success);
	}
	.sync[data-state='stale'] .dot {
		background: var(--warning);
	}
	.sync[data-state='offline'] .dot {
		background: var(--error);
	}
	.sync[data-state='loading'] .dot {
		background: var(--accent);
	}

	.bottombar {
		display: none;
	}

	@media (max-width: 719px) {
		.topbar {
			padding: 6px var(--sp-4);
			gap: 6px var(--sp-3);
		}
		.pills {
			display: none;
		}
		.sync-text {
			max-width: 40vw;
			overflow: hidden;
			text-overflow: ellipsis;
		}
		.bottombar {
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			z-index: 10;
			display: flex;
			justify-content: space-around;
			align-items: center;
			height: calc(var(--bottombar-h) + env(safe-area-inset-bottom));
			padding: 0 var(--sp-2) env(safe-area-inset-bottom);
			background: var(--surface);
			border-top: 1px solid var(--divider);
		}
		.tab {
			width: 64px;
			height: 44px;
			border-radius: var(--r-tile);
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			gap: 2px;
			font-size: var(--fs-micro);
			color: var(--muted);
		}
		.tab:hover {
			text-decoration: none;
		}
		.tab[aria-current='page'] {
			background: var(--elevated);
			color: var(--text);
		}
		.tab[aria-current='page'] :global(svg) {
			color: var(--accent);
		}
	}
</style>
