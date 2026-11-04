<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { sync, relativeTime } from '#lib/api/client';
	import Icon, { type IconName } from './Icon.svelte';

	const nav = [
		{ href: '/', label: 'Agora', icon: 'home', phone: true },
		{ href: '/guide', label: 'Guia', icon: 'book', phone: true },
		{ href: '/account', label: 'Conta', icon: 'user', phone: true },
		{ href: '/goals', label: 'Metas', icon: 'target', phone: true },
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
	<a class="brand eyebrow" href={resolve('/')}>Ironman Planner</a>

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
			<Icon name={item.icon} size={20} />
			<span>{item.label}</span>
		</a>
	{/each}
</nav>

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 10;
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		gap: var(--sp-4);
		height: var(--topbar-h);
		padding: 0 var(--sp-6);
		background: color-mix(in srgb, var(--bg) 88%, transparent);
		backdrop-filter: blur(10px);
		border-bottom: 1px solid var(--divider);
	}
	.brand {
		color: var(--muted);
		justify-self: start;
	}
	.brand:hover {
		text-decoration: none;
		color: var(--text);
	}
	.pills {
		display: flex;
		gap: 4px;
		padding: 3px;
		border-radius: var(--r-button);
		background: var(--surface);
		border: 1px solid var(--border);
	}
	.pill {
		display: inline-flex;
		align-items: center;
		height: 28px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-chip);
		font-size: var(--fs-control);
		font-weight: 500;
		color: var(--muted);
	}
	.pill:hover {
		text-decoration: none;
		color: var(--text);
		background: var(--elevated);
	}
	.pill[aria-current='page'] {
		background: var(--accent);
		color: var(--accent-text);
	}
	.sync {
		justify-self: end;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		font-size: var(--fs-caption);
		color: var(--muted);
		white-space: nowrap;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--subtle);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--subtle) 20%, transparent);
	}
	.sync[data-state='ok'] .dot {
		background: var(--success);
		box-shadow: 0 0 0 3px var(--success-tint);
	}
	.sync[data-state='stale'] .dot {
		background: var(--warning);
		box-shadow: 0 0 0 3px var(--warning-tint);
	}
	.sync[data-state='offline'] .dot {
		background: var(--error);
		box-shadow: 0 0 0 3px var(--error-tint);
	}
	.sync[data-state='loading'] .dot {
		background: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-tint);
	}

	.bottombar {
		display: none;
	}

	@media (max-width: 719px) {
		.topbar {
			grid-template-columns: 1fr auto;
			padding: 0 var(--sp-4);
		}
		.pills {
			display: none;
		}
		.sync-text {
			max-width: 46vw;
			overflow: hidden;
			text-overflow: ellipsis;
		}
		.bottombar {
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			z-index: 10;
			display: grid;
			grid-template-columns: repeat(4, 1fr);
			height: calc(var(--bottombar-h) + env(safe-area-inset-bottom));
			padding-bottom: env(safe-area-inset-bottom);
			background: var(--surface);
			border-top: 1px solid var(--border);
		}
		.tab {
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			gap: 3px;
			font-size: var(--fs-caption);
			color: var(--muted);
		}
		.tab:hover {
			text-decoration: none;
		}
		.tab[aria-current='page'] {
			color: var(--accent);
		}
	}
</style>
