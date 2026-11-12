<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { microbotModule, probeMicrobot } from '#lib/progress/microbot.svelte';
	import { forgetAccount } from '#lib/progress/account.svelte';
	import { fleet, loadFleet } from '#lib/progress/fleet.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { setTheme, theme } from '#lib/progress/theme.svelte';
	import { ago, STALE_SOURCE_MS } from '#lib/progress/view';
	import type { ProgressSummary } from '#lib/progress/types';

	/**
	 * Navegação: sidebar A (madeira inteira) e, na janela estreita ao lado do jogo, a barra de topo
	 * de V6Agora640. As duas ficam no DOM e o CSS mostra uma; a busca (⌘K) abre a paleta do Mundo.
	 * Da página 8 do canvas (V7Frota1440) vêm a Frota no topo e a lista de contas; as telas da conta
	 * escolhida ficam logo abaixo dela até virarem as abas da conta.
	 */
	let {
		summary,
		account,
		name,
		sub,
		skills
	}: {
		summary: ProgressSummary | null;
		account?: string;
		name?: string;
		sub?: string;
		skills?: { skill: string; level: number; xp: number }[];
	} = $props();

	const path = $derived(page.url.pathname);
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});
	const LABEL: Record<string, string> = {
		plugin: 'plugin',
		hiscores: 'hiscores',
		wom: 'wise old man',
		wikisync: 'wikisync'
	};
	const sources = $derived(
		(summary?.sources ?? []).map((s) => {
			const at = Date.parse(s.last_observed_at);
			return {
				key: s.source,
				label: LABEL[s.source] ?? s.source,
				age: ago(at, now).replace('há ', ''),
				stale: now - at > STALE_SOURCE_MS
			};
		})
	);
	const newest = $derived(
		[...sources].sort((a, b) => Number(a.stale) - Number(b.stale))[0] ?? null
	);
	const sourcesTitle = $derived(sources.map((s) => `${s.label} há ${s.age}`).join(' · '));

	const items = $derived([
		{ href: resolve('/'), label: 'Agora', icon: 'agora' as const, on: path === '/' },
		{
			href: resolve('/rotas'),
			label: 'Rotas',
			icon: 'rotas' as const,
			on: path.startsWith('/rotas') || path.startsWith('/compare'),
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
			on: path === '/conta' || path.startsWith('/conta/')
		},
		// Automação só existe com o módulo do Microbot ligado no servidor (doc 08).
		...(microbotModule.enabled
			? [
					{
						href: resolve('/automacao'),
						label: 'Automação',
						icon: 'automacao' as const,
						on: path.startsWith('/automacao')
					}
				]
			: [])
	]);
	$effect(() => {
		void probeMicrobot();
		void loadFleet();
	});
	const onFleet = $derived(path.startsWith('/frota'));
	const others = $derived((fleet.accounts ?? []).filter((a) => a.id !== account));
	const top = $derived(
		iconFor(
			`skill:${[...(skills ?? [])].sort((a, b) => b.level - a.level || b.xp - a.xp)[0]?.skill}`
		)
	);
	const initial = $derived((name ?? account ?? '?').replace(/^acc_/, '').charAt(0).toUpperCase());

	/** Sair: em oidc encerra a sessão; em dev (sem /auth) esquece a conta e volta ao setup. */
	async function signOut() {
		try {
			const res = await fetch('/auth/logout', {
				method: 'POST',
				headers: { 'X-Ironman-CSRF': '1' },
				redirect: 'manual'
			});
			if (res.status !== 404) {
				window.location.assign('/');
				return;
			}
		} catch {
			/* sem servidor de auth: segue como dev */
		}
		forgetAccount();
		void goto(resolve('/frota'));
	}
</script>

<nav class="side" aria-label="Principal">
	<div class="brand">
		<a href={resolve('/frota')} class="sr wm"
			><span class="logo"><Glyph name="book" size={26} /></span>Chronicle</a
		>
	</div>
	<a class="srch" href={resolve('/mundo')} aria-label="Buscar entidade, passo ou rota"
		><Glyph name="search" size={14} /><span>Buscar…</span><span class="kbd">⌘K</span></a
	>
	<a
		class="nav"
		class:nav-on={onFleet}
		href={resolve('/frota')}
		aria-current={onFleet ? 'page' : undefined}
	>
		<Glyph name="frota" />Frota
		{#if fleet.accounts?.length}<span class="n">{fleet.accounts.length}</span>{/if}
	</a>
	<p class="sec">Contas</p>
	<a
		class="acn acn-on"
		href={resolve('/conta')}
		title={[name ?? account, sub].filter(Boolean).join(' · ')}
	>
		<span class="lamp {(account && fleet.tones[account]) || 'unk'}"></span>
		<span class="who">
			<b>{name ?? account ?? 'sem conta'}</b>
			{#if sub}<span>{sub}</span>{/if}
		</span>
		{#if top}<img src={top} alt="" />{:else}<span class="av" aria-hidden="true">{initial}</span
			>{/if}
	</a>
	<div class="sub">
		{#each items as it (it.label)}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs dos itens vêm de resolve() -->
			<a
				class="nav"
				class:nav-on={it.on && !onFleet}
				href={it.href}
				aria-current={it.on && !onFleet ? 'page' : undefined}
			>
				<Glyph name={it.icon} />{it.label}
				{#if it.n}<span class="n">{it.n}</span>{/if}
			</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{/each}
	</div>
	{#each others as a (a.id)}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da conta -->
		<a class="acn" href={`${resolve('/')}?account=${encodeURIComponent(a.id)}`} title={a.id}>
			<span class="lamp {fleet.tones[a.id] ?? 'unk'}"></span>
			<span class="who"><b>{a.rsn}</b></span>
		</a>
	{/each}
	<a class="acn acn-new" href={resolve('/contas/nova')}
		><Glyph name="plus" size={14} /><span>Nova conta</span></a
	>
	<div class="grow"></div>
	<div class="srcs">
		<p class="lbl">Fontes</p>
		{#each sources as s (s.key)}
			<a class="srow" href={resolve('/conta')}
				><span class="dot" class:dot-w={s.stale}></span><span>{s.label}</span><span class="t"
					>{s.age}</span
				></a
			>
		{:else}
			<span class="srow off"><span class="dot dot-off"></span><span>nenhuma ainda</span></span>
		{/each}
	</div>
	<div class="sfoot">
		<button
			class="tb"
			class:tb-on={theme.value === 'pergaminho'}
			type="button"
			aria-label="Tema pergaminho (claro)"
			aria-pressed={theme.value === 'pergaminho'}
			onclick={() => setTheme('pergaminho')}><Glyph name="sun" size={15} /></button
		>
		<button
			class="tb"
			class:tb-on={theme.value === 'pedra'}
			type="button"
			aria-label="Tema pedra (escuro)"
			aria-pressed={theme.value === 'pedra'}
			onclick={() => setTheme('pedra')}><Glyph name="moon" size={15} /></button
		>
		<button class="out" type="button" onclick={signOut}>sair</button>
	</div>
</nav>

<nav class="bar" aria-label="Principal">
	<a href={resolve('/frota')} class="sr wm wm-s"
		><span class="logo"><Glyph name="book" size={22} /></span>Chronicle</a
	>
	<a
		class="nav"
		class:nav-on={onFleet}
		href={resolve('/frota')}
		aria-current={onFleet ? 'page' : undefined}
		><Glyph name="frota" /><span class="nl">Frota</span></a
	>
	{#each items as it (it.label)}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- hrefs dos itens vêm de resolve() -->
		<a class="nav" class:nav-on={it.on} href={it.href} aria-current={it.on ? 'page' : undefined}
			><Glyph name={it.icon} /><span class="nl">{it.label}</span></a
		>
	{/each}
	<span class="grow"></span>
	{#if newest}
		<a class="nav src" href={resolve('/conta')} title={sourcesTitle}
			><span class="dot" class:dot-w={newest.stale}></span>{newest.label} {newest.age}</a
		>
	{/if}
	<button
		class="tb"
		type="button"
		aria-label={theme.value === 'pergaminho' ? 'Tema pedra (escuro)' : 'Tema pergaminho (claro)'}
		onclick={() => setTheme(theme.value === 'pergaminho' ? 'pedra' : 'pergaminho')}
		><Glyph name={theme.value === 'pergaminho' ? 'moon' : 'sun'} size={15} /></button
	>
</nav>

<style>
	.side {
		position: sticky;
		top: 0;
		height: 100vh;
		overflow-y: auto;
	}
	.wm {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		font-size: 20px;
		color: var(--gold);
		text-decoration: none;
		letter-spacing: 0.01em;
	}
	.wm-s {
		font-size: 17px;
		gap: 8px;
		white-space: nowrap;
		padding: 0 8px 0 4px;
	}
	.logo {
		display: inline-flex;
	}
	.brand {
		padding-bottom: 6px;
	}
	.sec {
		margin: 0;
		padding: 14px 8px 4px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--on-frame-muted);
	}
	.acn {
		display: grid;
		grid-template-columns: 10px minmax(0, 1fr) auto;
		align-items: center;
		gap: 9px;
		min-height: 44px;
		padding: 4px 8px;
		border-radius: 4px;
		color: var(--on-frame);
		text-decoration: none;
		font-size: 13px;
	}
	.acn:hover {
		background: var(--frame-2);
	}
	.acn-on {
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
	}
	.acn img {
		width: 18px;
		height: 18px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.acn .av {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--gold);
		color: var(--frame);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font:
			700 11px Alegreya,
			Georgia,
			serif;
	}
	.acn-new {
		grid-template-columns: 14px minmax(0, 1fr);
		margin-top: 4px;
		color: var(--on-frame-muted);
		border: 1px dashed var(--frame-2);
	}
	.who {
		display: flex;
		flex-direction: column;
		min-width: 0;
		line-height: 1.25;
	}
	.who b {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.who span {
		font-size: 11px;
		color: var(--on-frame-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sub {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin: 2px 0 6px 12px;
		padding-left: 8px;
		border-left: 1px solid var(--frame-2);
	}
	.sub .nav {
		height: 34px;
		font-size: 13px;
	}
	.lamp {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		box-sizing: border-box;
		background: var(--done);
	}
	.lamp.up {
		background: var(--auto);
	}
	.lamp.bad {
		background: var(--not-started);
	}
	.lamp.off,
	.lamp.unk {
		background: transparent;
		border: 1.5px solid var(--on-frame-muted);
	}
	.grow {
		flex: 1;
	}
	.dot-off {
		background: var(--on-frame-muted);
		opacity: 0.5;
	}
	.out {
		margin-left: auto;
		border: 0;
		background: transparent;
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
		color: var(--on-frame-muted);
		cursor: pointer;
		padding: 8px 4px;
	}
	.out:hover {
		color: var(--on-frame);
	}
	.bar {
		display: none;
		position: sticky;
		top: 0;
		z-index: 4;
		width: 100%;
		flex-basis: 100%;
		overflow-x: auto;
	}
	/* Na barra de 640 a navegação é a da primeira regra do canvas (sem a barra dourada). */
	.bar .nav {
		height: 44px;
		padding: 0 10px;
		gap: 8px;
		font-size: 13px;
	}
	.bar .nav-on::before {
		display: none;
	}
	.bar .tb {
		flex: 0 0 auto;
	}
	.bar .src {
		gap: 6px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		white-space: nowrap;
	}
	@media (max-width: 900px) {
		.side {
			display: none;
		}
		.bar {
			display: flex;
		}
	}
	@media (max-width: 560px) {
		.bar .nl,
		/* Na barra de 640 a navegação é a da primeira regra do canvas (sem a barra dourada). */
	.bar .nav {
			height: 44px;
			padding: 0 10px;
			gap: 8px;
			font-size: 13px;
		}
		.bar .nav-on::before {
			display: none;
		}
		.bar .tb {
			flex: 0 0 auto;
		}
		.bar .src {
			display: none;
		}
	}
</style>
