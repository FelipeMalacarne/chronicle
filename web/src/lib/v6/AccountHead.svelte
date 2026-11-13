<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import {
		MicrobotError,
		microbotApi,
		workersApi,
		type ClientStatus,
		type WorkersView
	} from '#lib/api/microbot';
	import { fleet } from '#lib/progress/fleet.svelte';
	import { lane, type Lane } from '#lib/progress/frota';
	import { iconFor } from '#lib/progress/icons';
	import { microbotModule } from '#lib/progress/microbot.svelte';
	import type { AccountState, ProgressSummary } from '#lib/progress/types';
	import { actionLabel, workerLines, type WorkerActionKind } from '#lib/progress/workers';
	import Glyph from './Glyph.svelte';

	/*
	 * O topo das telas de uma conta (página 8 do canvas, V7Conta1440): quem é, o que faz agora, as
	 * ações e as abas. Fatia V1 de decisoes-canvas.md, 7.5: Rota, Personagem e Cliente levam às telas
	 * que já existiam; Histórico é a da V7 (V7Historico1440).
	 */
	let { state: st, summary }: { state: AccountState; summary: ProgressSummary | null } = $props();

	const id = $derived(st.account.id);
	let workers = $state<WorkersView | null>(null);
	let client = $state<ClientStatus | null>(null);
	let scripts = $state<string[]>([]);
	let now = $state(Date.now());
	let armed = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);

	const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

	async function load(account: string) {
		try {
			workers = await workersApi.list();
		} catch (e) {
			if (!(e instanceof MicrobotError && e.status === 404)) failure = String(e);
			workers = null;
		}
		const line = workerLines(workers, Date.now()).find((l) => l.account === account);
		if (line?.cockpit) {
			const [status, plugins] = await Promise.all([
				quiet(microbotApi.statusOf(account)),
				quiet(microbotApi.pluginsOf(account))
			]);
			client = status?.client ?? null;
			scripts = (plugins?.scripts.scripts ?? []).filter((s) => s.active).map((s) => s.name);
		} else {
			client = null;
			scripts = [];
		}
		now = Date.now();
	}

	$effect(() => {
		const account = id;
		void load(account);
		const t = setInterval(() => void load(account), 5000);
		return () => clearInterval(t);
	});

	const l: Lane = $derived(
		lane(
			{
				account: st.account,
				state: st,
				summary: summary?.account === id ? summary : null,
				timers: [],
				worker: workerLines(workers, now).find((w) => w.account === id) ?? null,
				client,
				scripts
			},
			now
		)
	);
	$effect(() => {
		fleet.tones[id] = l.tone;
	});
	const face = $derived(l.face ? iconFor(`skill:${l.face.skill}`) : undefined);
	const meta = $derived(
		[l.meta, st.quest_points !== undefined ? `${st.quest_points} QP` : '']
			.filter(Boolean)
			.join(' · ')
	);

	async function act(action: WorkerActionKind) {
		const key = action;
		if (armed !== key) {
			armed = key;
			return;
		}
		armed = null;
		busy = key;
		failure = null;
		try {
			await workersApi.action(id, action);
			await load(id);
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = null;
		}
	}

	const path = $derived(page.url.pathname);
	const onRoute = $derived(
		path.startsWith('/agora') || path.startsWith('/rotas') || path.startsWith('/compare')
	);
	const views = $derived([
		{ href: resolve('/agora'), label: 'Passo', on: path.startsWith('/agora') },
		{ href: resolve('/rotas'), label: 'Rota inteira', on: path.startsWith('/rotas') },
		{ href: resolve('/compare'), label: 'Comparar', on: path.startsWith('/compare') }
	]);
	const tabs = $derived([
		{ href: resolve('/visao'), label: 'Visão geral', on: path.startsWith('/visao') },
		{ href: resolve('/agora'), label: 'Rota', on: onRoute },
		{ href: resolve('/conta'), label: 'Personagem', on: path === '/conta' },
		// O cliente só existe com o módulo do Microbot ligado no servidor (doc 08).
		...(microbotModule.enabled
			? [{ href: resolve('/automacao'), label: 'Cliente', on: path.startsWith('/automacao') }]
			: []),
		{ href: resolve('/historico'), label: 'Histórico', on: path.startsWith('/historico') }
	]);
</script>

<a class="crumb mu" href={resolve('/')}>Frota<Glyph name="chevron" size={12} />{st.account.rsn}</a>
<header class="hero">
	<div class="stage" class:live={l.tone === 'ok'}>
		{#if face}<img src={face} alt={l.face?.skill} />{:else}<span class="ini" aria-hidden="true"
				>{st.account.rsn.charAt(0).toUpperCase()}</span
			>{/if}
		{#if l.face}<span class="lv" title="{l.face.skill} {l.face.level}">{l.face.level}</span>{/if}
	</div>
	<div class="ident">
		<div class="rsn">
			<h1 class="sr">{st.account.rsn}</h1>
			<span class="mu">{meta}</span>
		</div>
		<div class="state {l.tone}">
			<span class="lamp {l.tone}"></span><b>{l.activity}</b>
			{#if l.detail}<span class="mu" title={l.detail}>{l.detail}</span>{/if}
			{#if l.where}<span class="tag" title={l.worker?.runtime}>{l.where}</span>{/if}
		</div>
		{#if failure}<p class="err">{failure}</p>{/if}
	</div>
	<div class="ctl">
		{#if l.client && !path.startsWith('/automacao')}
			<a class="btn btn-p" href={resolve('/automacao')}><Glyph name="eye" size={14} />Ver jogo</a>
		{/if}
		{#each l.worker?.actions ?? [] as a (a)}
			<button
				type="button"
				class="btn"
				class:btn-d={a === 'stop' || (a !== 'start' && armed === a)}
				disabled={!!busy || (a === 'start' && !!l.worker?.blocked)}
				title={a === 'start'
					? (l.worker?.blocked ?? 'Sobe o pod; o cliente para na tela de login')
					: a === 'restart' && l.worker?.outdated
						? 'Reinicia na versão nova; sai do jogo'
						: 'Sai do jogo'}
				onclick={() => act(a)}
			>
				{busy === a
					? '…'
					: armed === a
						? 'Confirmar'
						: a === 'restart' && l.worker?.outdated
							? 'Atualizar'
							: actionLabel(l.worker ?? { outdated: false }, a)}
			</button>
		{/each}
	</div>
</header>

<nav class="atabs" aria-label="Telas da conta">
	<!-- eslint-disable svelte/no-navigation-without-resolve -- hrefs das abas vêm de resolve() -->
	{#each tabs as t (t.label)}
		<a class:on={t.on} href={t.href} aria-current={t.on ? 'page' : undefined}>{t.label}</a>
	{/each}
	{#if onRoute}
		<span class="views" role="group" aria-label="Como ver a rota">
			{#each views as v (v.label)}
				<a class:here={v.on} href={v.href} aria-current={v.on ? 'page' : undefined}>{v.label}</a>
			{/each}
		</span>
	{/if}
	<!-- eslint-enable svelte/no-navigation-without-resolve -->
</nav>

<style>
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		width: max-content;
		font-size: 12px;
		text-decoration: none;
		margin-bottom: -8px;
	}
	.hero {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr) auto;
		gap: 18px;
		align-items: center;
	}
	@keyframes bob {
		0%,
		100% {
			transform: translateY(0);
		}
		50% {
			transform: translateY(-3px);
		}
	}
	.stage {
		position: relative;
		width: 76px;
		height: 76px;
		border-radius: 4px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		display: flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
	}
	.stage img {
		width: 44px;
		height: 44px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.stage.live img {
		animation: bob 1.8s ease-in-out infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.stage.live img {
			animation: none;
		}
	}
	.lv {
		position: absolute;
		right: -6px;
		bottom: -6px;
		min-width: 26px;
		height: 22px;
		padding: 0 5px;
		box-sizing: border-box;
		border-radius: 3px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--gold);
		font:
			700 14px/22px Alegreya,
			Georgia,
			serif;
		font-variant-numeric: lining-nums;
		text-align: center;
	}
	.ini {
		font:
			700 30px Alegreya,
			Georgia,
			serif;
		color: var(--ink-subtle);
	}
	.ident {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
	.rsn {
		display: flex;
		align-items: baseline;
		gap: 12px;
		flex-wrap: wrap;
	}
	.rsn h1 {
		font-size: 28px;
	}
	.rsn .mu {
		font-size: 13px;
	}
	.state {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		font-size: 13px;
	}
	.state b {
		font-weight: 600;
		white-space: nowrap;
	}
	.state .mu {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.state.up b {
		color: var(--auto);
	}
	.state.bad b {
		color: var(--not-started);
	}
	.lamp {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		box-sizing: border-box;
		background: var(--ink-subtle);
	}
	.lamp.ok {
		background: var(--done);
		box-shadow: 0 0 0 3px var(--done-tint);
	}
	.lamp.up {
		background: var(--auto);
	}
	.lamp.bad {
		background: var(--not-started);
	}
	.lamp.off {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	.tag {
		display: inline-flex;
		align-items: center;
		height: 18px;
		padding: 0 6px;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		white-space: nowrap;
		flex: 0 0 auto;
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.ctl {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		justify-content: flex-end;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.atabs {
		display: flex;
		gap: 2px;
		border-bottom: 2px solid var(--frame);
		overflow-x: auto;
	}
	.atabs a {
		display: inline-flex;
		align-items: center;
		height: 38px;
		padding: 0 14px;
		border-radius: 5px 5px 0 0;
		font-size: 13px;
		font-weight: 500;
		color: var(--ink-muted);
		text-decoration: none;
		white-space: nowrap;
	}
	.atabs a:hover {
		color: var(--ink);
	}
	.atabs a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.views {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 2px;
	}
	.atabs .views a {
		height: 30px;
		padding: 0 10px;
		border-radius: 4px;
		font-size: 12px;
	}
	.atabs .views a.here {
		background: var(--gold-tint);
		color: var(--ink);
	}
	@media (max-width: 900px) {
		.hero {
			grid-template-columns: 56px minmax(0, 1fr);
			gap: 12px;
		}
		.stage {
			width: 56px;
			height: 56px;
		}
		.stage img {
			width: 34px;
			height: 34px;
		}
		.rsn h1 {
			font-size: 22px;
		}
		.state {
			flex-wrap: wrap;
		}
		.ctl {
			grid-column: 1 / -1;
			justify-content: flex-start;
		}
		.ctl .btn {
			height: 36px;
		}
	}
</style>
