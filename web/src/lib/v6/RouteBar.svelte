<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { quietDetection } from '#lib/api/events.svelte';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { loadMe } from '#lib/progress/me.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { pct, routeLabel } from '#lib/progress/view';
	import Glyph from './Glyph.svelte';
	import Toast from './Toast.svelte';

	/**
	 * A barra da aba Rota da conta (V7Rota1440, V7Rota640, V7RotaLista1440): a rota (um seletor com
	 * as rotas da conta, cada uma com o próprio cursor), o capítulo ou seção com o andamento, Passo |
	 * Rota inteira | Comparar e Sincronizar. A 640 o trio vira um botão para a outra vista.
	 */
	let {
		view,
		guide,
		section = null,
		onsynced
	}: {
		view: 'passo' | 'lista' | 'compare';
		/** A rota na tela; sem ela, a ativa. */
		guide?: string;
		/** "cap. 1.3 · Unlocking bird runs · 1 de 31". */
		section?: { label: string; title: string; done: number; total: number } | null;
		onsynced?: () => void;
	} = $props();

	const ORDER = ['oziris-main', 'bruhsailer-main'];
	const rank = (g: string) => (ORDER.includes(g) ? ORDER.indexOf(g) : ORDER.length);
	const routes = $derived(
		[...(session.summary?.routes ?? [])].sort((a, b) => rank(a.guide) - rank(b.guide))
	);
	const route = $derived(
		routes.find((r) => r.guide === guide) ?? routes.find((r) => r.active) ?? null
	);
	const label = $derived(route ? routeLabel(route.guide, route.title) : 'Rota');
	const listHref = $derived(
		route ? resolve('/rotas/[guide]', { guide: route.guide }) : resolve('/rotas')
	);
	const views = $derived([
		{ id: 'passo', label: 'Passo', href: resolve('/agora') },
		{ id: 'lista', label: 'Rota inteira', href: listHref },
		{ id: 'compare', label: 'Comparar', href: resolve('/compare') }
	]);
	const other = $derived(view === 'passo' ? views[1] : views[0]);

	let menu = $state(false);
	let failure = $state<string | null>(null);
	async function activate(id: string) {
		const account = session.account;
		if (!account) return;
		menu = false;
		try {
			await progressApi.setActiveGuide(account, id);
			await loadSummary(account);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		}
	}

	let syncing = $state(false);
	let toast = $state<string | null>(null);
	async function syncNow() {
		const account = session.account;
		if (!account || syncing) return;
		syncing = true;
		// Este sync mostra o próprio toast; o evento dele não repete a detecção.
		quietDetection('hiscores');
		try {
			const run = await progressApi.sync(account);
			toast =
				run.status === 'ok'
					? `${run.steps_changed} passos mudaram`
					: run.status === 'unchanged'
						? 'nada mudou desde o último sync'
						: (run.error ?? 'falhou');
			void loadSummary(account);
			void loadMe(account);
			onsynced?.();
		} catch (e) {
			const tooSoon =
				e instanceof Error && 'status' in e && (e as { status: number }).status === 429;
			toast = tooSoon ? 'aguarde: um sync por minuto, e nunca durante o recuo' : String(e);
		} finally {
			syncing = false;
		}
	}
</script>

<svelte:window
	onclick={(e) => menu && !(e.target as HTMLElement).closest('.rpick') && (menu = false)}
/>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs vêm de resolve() -->
<div class="rbar">
	<div class="rpick">
		<button
			class="sel"
			type="button"
			aria-haspopup="listbox"
			aria-expanded={menu}
			title="Cada rota guarda o próprio lugar; o que você fez vale nas duas"
			onclick={() => (menu = !menu)}
			>{label}{#if !narrow.value}<small
					>{route?.active ? 'rota ativa' : route?.cursor_ref ? `em ${route.cursor_ref}` : ''}</small
				>{/if}<span class="chev"><Glyph name="chevron" size={12} /></span></button
		>
		{#if menu}
			<div class="menu panel" role="listbox" aria-label="Rotas da conta">
				{#each routes as r (r.guide)}
					<div class="mi" class:on={r.guide === route?.guide}>
						<a
							role="option"
							aria-selected={r.guide === route?.guide}
							href={resolve('/rotas/[guide]', { guide: r.guide })}
							onclick={() => (menu = false)}
							><b>{routeLabel(r.guide, r.title)}</b><span class="su mono"
								>{r.counts.done} / {r.counts.total}{r.active
									? ' · ativa'
									: r.cursor_ref
										? ` · em ${r.cursor_ref}`
										: ''}</span
							></a
						>
						{#if !r.active}
							<button class="btn btn-s" type="button" onclick={() => activate(r.guide)}
								>Tornar ativa</button
							>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
	{#if section}
		<div class="rcap">
			<span
				><span class="mono su tiny">{section.label}</span> <b>{section.title}</b>
				<span class="mu">· {section.done} de {section.total}{narrow.value ? '' : ' feitos'}</span
				></span
			>
			<div class="xp"><span style="width: {pct(section.done, section.total)}%"></span></div>
		</div>
	{:else}
		<span class="grow"></span>
	{/if}
	{#if narrow.value}
		<!-- V7Rota640: a outra vista num botão; Sincronizar fica na 1440. -->
		<a class="btn btn-s" href={other.href}>{other.label}</a>
	{:else}
		<div class="seg" role="group" aria-label="Como ver a rota">
			{#each views as v (v.id)}
				<a class:on={v.id === view} href={v.href} aria-current={v.id === view ? 'page' : undefined}
					>{v.label}</a
				>
			{/each}
		</div>
		<button class="btn btn-s" type="button" disabled={syncing || !session.account} onclick={syncNow}
			><Glyph name="sync" size={14} />{syncing ? 'Sincronizando…' : 'Sincronizar'}</button
		>
	{/if}
</div>
{#if failure}
	<div class="band" role="alert"><Glyph name="alert" /><span>{failure}</span></div>
{/if}
{#if toast}
	<Toast title="Hiscores" text={toast} onclose={() => (toast = null)} />
{/if}

<style>
	.rpick {
		position: relative;
	}
	.chev {
		display: inline-flex;
		transform: rotate(90deg);
	}
	.grow {
		flex: 1 1 0;
	}
	.tiny {
		font-size: 11px;
	}
	.menu {
		position: absolute;
		z-index: 8;
		top: calc(100% + 4px);
		left: 0;
		min-width: 300px;
		max-width: calc(100vw - 28px);
		display: flex;
		flex-direction: column;
		padding: 4px;
		box-shadow:
			var(--bevel),
			0 8px 24px rgba(0, 0, 0, 0.25);
	}
	.mi {
		display: flex;
		align-items: center;
		gap: 8px;
		padding-right: 6px;
		border-bottom: 1px solid var(--hairline);
	}
	.mi:last-child {
		border-bottom: 0;
	}
	.mi.on {
		background: var(--gold-tint);
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.mi a {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 44px;
		padding: 0 10px;
		color: var(--ink);
		text-decoration: none;
		font-size: 13px;
	}
	.mi a b {
		font-weight: 600;
	}
	.mi a .su {
		margin-left: auto;
		font-size: 11px;
	}
</style>
