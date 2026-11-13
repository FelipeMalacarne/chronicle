<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { workersApi } from '#lib/api/microbot';
	import { rememberAccount } from '#lib/progress/account.svelte';
	import { loadFleet } from '#lib/progress/fleet.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { routeLabel } from '#lib/progress/view';
	import type { AccountMode, AccountView, SetupLookup } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Nova conta (página 8 do canvas, V7NovaConta1440): no lugar do primeiro acesso em seis passos.
	 * Diverge do artboard em dois pontos, pelo que o servidor aceita hoje: a rota é obrigatória (não
	 * há "Nenhuma") e a sessão do jogo vem depois de criar a conta, porque `accounts connect` procura
	 * a conta pelo personagem. "Onde ela joga" fica na raia da Frota (Iniciar).
	 */
	const guides = createResource(() => progressApi.guides());

	let rsn = $state('');
	let lookup = $state<SetupLookup | null>(null);
	let looking = $state(false);
	let mode = $state<AccountMode>('ironman');
	let guide = $state('oziris-main');
	let enhanced = $state(true);
	let busy = $state(false);
	let failure = $state<string | null>(null);
	let created = $state<AccountView | null>(null);
	let syncNote = $state<string | null>(null);
	/** undefined: o servidor não guarda sessões (sem workers); false: ainda não chegou. */
	let hasSession = $state<boolean | undefined>(false);
	let copied = $state(false);

	const MODES: { id: AccountMode; label: string }[] = [
		{ id: 'ironman', label: 'Ironman' },
		{ id: 'hcim', label: 'Hardcore' },
		{ id: 'uim', label: 'Ultimate' },
		{ id: 'gim', label: 'Group' }
	];
	const AUTHORS: Record<string, string> = {
		'oziris-main': 'OzirisLoL · ironman.guide',
		'bruhsailer-main': 'So Iron BRUH e ParasailerOSRS'
	};

	const name = $derived(rsn.trim());
	const top = $derived([...(lookup?.skills ?? [])].sort((a, b) => b.level - a.level).slice(0, 3));
	const face = $derived(top[0] && top[0].level > 1 ? top[0] : undefined);
	const command = $derived(`chronicle accounts connect --character "${created?.rsn ?? name}"`);
	const sorted = $derived(
		[...(guides.data?.guides ?? [])].sort((a, b) =>
			a.id.startsWith('oziris') ? -1 : b.id.startsWith('oziris') ? 1 : 0
		)
	);

	async function find() {
		if (!name || looking) return;
		looking = true;
		failure = null;
		lookup = null;
		try {
			lookup = await progressApi.setupLookup(name);
			if (lookup.mode) mode = lookup.mode;
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			looking = false;
		}
	}

	function enter(id: string) {
		rememberAccount(id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da conta
		void goto(`${resolve('/visao')}?account=${encodeURIComponent(id)}`);
	}

	async function create() {
		busy = true;
		failure = null;
		try {
			created = await progressApi.createAccount({
				rsn: name,
				mode,
				guide_id: guide,
				layer: enhanced ? 'enhanced' : 'classic'
			});
			rememberAccount(created.id);
			void loadFleet(true);
			try {
				await progressApi.sync(created.id);
				syncNote = 'Primeira leitura do hiscores feita.';
			} catch (e) {
				syncNote = `A primeira leitura do hiscores falhou: ${e instanceof Error ? e.message : String(e)}`;
			}
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	// Depois de criada, esta tela avisa sozinha quando a sessão do jogo chega ao servidor.
	$effect(() => {
		const id = created?.id;
		if (!id) return;
		const check = async () => {
			try {
				const view = await workersApi.list();
				hasSession = !!view.workers.find((w) => w.account === id)?.session;
			} catch {
				hasSession = undefined;
			}
		};
		void check();
		const t = setInterval(() => void check(), 4000);
		return () => clearInterval(t);
	});

	async function copy() {
		try {
			await navigator.clipboard.writeText(command);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			/* sem permissão de área de transferência: o comando segue selecionável */
		}
	}
</script>

<svelte:head><title>Nova conta · Chronicle</title></svelte:head>

<a class="crumb mu" href={resolve('/')}>Frota<Glyph name="chevron" size={12} />Nova conta</a>
<header class="ttl">
	<h1 class="sr">Nova conta</h1>
	<p class="mu">
		O nome e a rota criam a conta. A sessão do jogo, que deixa o Chronicle abrir o cliente, vem em
		seguida ou depois.
	</p>
</header>

{#if failure}
	<div class="band"><Glyph name="alert" /><span>{failure}</span></div>
{/if}

<div class="cols">
	<div class="form">
		<section class="step">
			<span class="no" class:dn={!!lookup && !lookup.account}>
				{#if lookup && !lookup.account}<Glyph name="check" size={16} />{:else}1{/if}
			</span>
			<div class="c">
				<h2><label for="rsn">Personagem</label></h2>
				<form
					class="find"
					onsubmit={(e) => {
						e.preventDefault();
						void find();
					}}
				>
					<div class="field">
						<Glyph name="search" size={14} /><input
							id="rsn"
							bind:value={rsn}
							maxlength="12"
							autocomplete="off"
							placeholder="nome no jogo"
							disabled={!!created}
							oninput={() => (lookup = null)}
						/>
					</div>
					{#if !created}
						<button class="btn" type="submit" disabled={looking || !name}
							>{looking ? 'Procurando…' : 'Procurar'}</button
						>
					{/if}
				</form>
				{#if lookup?.account}
					<div class="found">
						<span class="ck"><Glyph name="check" size={11} /></span>
						<span><b>{lookup.account.rsn}</b> já está na frota</span>
						<button class="btn btn-s push" type="button" onclick={() => enter(lookup!.account!.id)}
							>Abrir</button
						>
					</div>
				{:else if lookup}
					<div class="found" class:soft={!lookup.found}>
						{#if lookup.found}<span class="ck"><Glyph name="check" size={11} /></span>{/if}
						<span>
							{#if lookup.hiscores_error}
								O hiscores não respondeu; dá para criar assim mesmo.
							{:else if !lookup.found}
								<b>{name}</b> não está no hiscores; dá para criar assim mesmo.
							{:else}
								<b>{name}</b> está no hiscores{lookup.total_level
									? ` · ${lookup.total_level} total`
									: ''}
							{/if}
						</span>
						{#if top.length}
							<span class="sk">
								{#each top as s (s.skill)}
									{@const icon = iconFor(`skill:${s.skill}`)}
									<span title={s.skill}
										>{#if icon}<img src={icon} alt={s.skill} />{/if}{s.level}</span
									>
								{/each}
							</span>
						{/if}
					</div>
					{#if !created}
						<div class="modes">
							<span class="lbl">{lookup.mode ? 'Modo, detectado pelo hiscores' : 'Modo'}</span>
							<div class="seg" role="group" aria-label="Modo da conta">
								{#each MODES as m (m.id)}
									<button type="button" class:on={mode === m.id} onclick={() => (mode = m.id)}
										>{m.label}</button
									>
								{/each}
							</div>
						</div>
					{/if}
				{/if}
			</div>
		</section>

		<section class="step" class:wait={!lookup || !!lookup.account}>
			<span class="no" class:dn={!!created}>
				{#if created}<Glyph name="check" size={16} />{:else}2{/if}
			</span>
			<div class="c">
				<h2>Rota</h2>
				<p>
					O progresso é da conta: o que ela fizer numa rota vale na outra, e dá para trocar depois
					sem perder o lugar.
				</p>
				<div class="opts">
					{#each sorted as g (g.id)}
						<label class="opt7" class:on={guide === g.id}>
							<input
								type="radio"
								name="rota"
								value={g.id}
								bind:group={guide}
								disabled={!!created}
							/>
							<span>
								<span class="t">{routeLabel(g.id, g.title)}</span>
								<span class="s">{AUTHORS[g.id] ?? g.source?.author ?? ''} · {g.steps} passos</span>
							</span>
						</label>
					{/each}
				</div>
				<label
					class="chk7"
					title="Enhanced são os passos que o ironman.guide acrescentou ao guia original do Oziris"
					><input type="checkbox" bind:checked={enhanced} disabled={!!created} />Mostrar os passos
					enhanced do ironman.guide</label
				>
				{#if !created}
					<div class="go">
						<button
							class="btn btn-p"
							type="button"
							disabled={busy || !lookup || !!lookup.account}
							onclick={create}
							><Glyph name="plus" size={14} />{busy ? 'Adicionando…' : 'Adicionar à frota'}</button
						>
						<a class="btn btn-g" href={resolve('/')}>Cancelar</a>
					</div>
				{/if}
			</div>
		</section>

		<section class="step" class:wait={!created}>
			<span class="no" class:dn={hasSession === true}>
				{#if hasSession === true}<Glyph name="check" size={16} />{:else}3{/if}
			</span>
			<div class="c">
				<h2>Sessão do jogo</h2>
				{#if hasSession === undefined && created}
					<p>
						Este servidor não guarda sessões (sem workers), então a conta fica só acompanhada pelo
						hiscores e pelo plugin.
					</p>
				{:else}
					<p>
						É o que deixa o Chronicle abrir o cliente desta conta. No computador com o launcher do
						Microbot aberto, rode:
					</p>
					<div class="code7">
						<code>{command}</code>
						<button type="button" onclick={copy} disabled={!name}
							>{copied ? 'Copiado' : 'Copiar'}</button
						>
					</div>
					{#if created && hasSession}
						<div class="found">
							<span class="ck"><Glyph name="check" size={11} /></span>
							<span>Sessão recebida. A conta já pode subir pela Frota.</span>
						</div>
					{:else if created}
						<div class="wait7">
							<span class="lamp"></span>Esperando a sessão chegar… esta tela avisa sozinha.
						</div>
					{/if}
				{/if}
				{#if created}
					{#if syncNote}<p>{syncNote}</p>{/if}
					<div class="go">
						<a class="btn btn-p" href={resolve('/')}>Ir para a Frota</a>
						<button class="btn btn-g" type="button" onclick={() => enter(created!.id)}
							>Abrir a conta</button
						>
					</div>
				{/if}
			</div>
		</section>
	</div>

	<aside class="pre">
		<p class="lbl">Como ela entra na frota</p>
		<div class="lane">
			<div class="stage">
				{#if face}
					{@const icon = iconFor(`skill:${face.skill}`)}
					{#if icon}<img src={icon} alt={face.skill} />{/if}
					<span class="lv">{face.level}</span>
				{:else}
					<span class="ini" aria-hidden="true">{(name || '?').charAt(0).toUpperCase()}</span>
				{/if}
			</div>
			<div class="now">
				<div class="who">
					<span class="lamp off"></span><b>{created?.rsn ?? (name || 'Nome da conta')}</b>
				</div>
				<p class="act">
					<b>{created && hasSession ? 'Parada' : 'Só acompanhada'}</b><span
						>{created && hasSession ? 'pronta para subir' : 'sem sessão do jogo'}</span
					>
				</p>
				<span class="m"
					>{MODES.find((m) => m.id === mode)?.label.toLowerCase()}{lookup?.total_level
						? ` · ${lookup.total_level} total`
						: ''}</span
				>
			</div>
		</div>
		<p class="mu note">
			O quadro mostra a skill mais alta. Onde a conta joga se decide na própria raia: Iniciar sobe
			um worker no saradomin.
		</p>
	</aside>
</div>

<p class="foot">{ICON_ATTRIBUTION}. Cliente de terceiros, fora do planner.</p>

<style>
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		width: max-content;
		font-size: 12px;
		text-decoration: none;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.ttl h1 {
		font-size: 30px;
	}
	.ttl p {
		margin: 0;
		font-size: 13px;
		max-width: 70ch;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 32px;
		align-items: flex-start;
	}
	.form {
		flex: 999 1 520px;
		min-width: 0;
		max-width: 760px;
		display: flex;
		flex-direction: column;
	}
	.step {
		display: grid;
		grid-template-columns: 34px minmax(0, 1fr);
		gap: 14px;
		padding: 18px 0;
		border-bottom: 1px solid var(--hairline);
	}
	.step:first-child {
		padding-top: 4px;
	}
	.step:last-child {
		border-bottom: 0;
	}
	.step.wait {
		opacity: 0.55;
	}
	.no {
		width: 34px;
		height: 34px;
		border-radius: 50%;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--gold);
		font:
			700 17px/34px Alegreya,
			Georgia,
			serif;
		text-align: center;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.no.dn {
		background: var(--done);
		box-shadow: none;
		color: var(--ground);
	}
	.c {
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 0;
	}
	.c h2 {
		font:
			700 18px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
		padding-top: 6px;
	}
	.c > p {
		margin: 0;
		font-size: 13px;
		color: var(--ink-muted);
	}
	.find {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.field {
		flex: 1 1 260px;
		display: flex;
		align-items: center;
		gap: 10px;
		height: 44px;
		padding: 0 12px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		box-sizing: border-box;
		color: var(--ink-subtle);
	}
	.field input {
		flex: 1;
		min-width: 0;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--ink);
		font:
			500 15px Inter,
			system-ui,
			sans-serif;
	}
	.field:focus-within {
		border-color: var(--gold-ink);
	}
	.found {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 12px;
		border-radius: 4px;
		border: 1px solid var(--done);
		background: var(--done-tint);
		font-size: 13px;
	}
	.found.soft {
		border-color: var(--border);
		background: transparent;
	}
	.found b {
		font-weight: 600;
	}
	.ck {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--done);
		color: var(--ground);
		display: flex;
		align-items: center;
		justify-content: center;
		flex: 0 0 auto;
	}
	.push {
		margin-left: auto;
	}
	.sk {
		margin-left: auto;
		display: flex;
		gap: 10px;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
	}
	.sk span {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.sk img {
		width: 14px;
		height: 14px;
		image-rendering: pixelated;
	}
	.modes {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 14px;
	}
	.seg button.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.opts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
		gap: 10px;
	}
	.opt7 {
		display: flex;
		gap: 10px;
		align-items: flex-start;
		padding: 12px;
		border-radius: 5px;
		border: 1px solid var(--border);
		cursor: pointer;
	}
	.opt7 input {
		margin: 2px 0 0;
		width: 16px;
		height: 16px;
		accent-color: var(--gold-ink);
		flex: 0 0 auto;
	}
	.opt7 .t {
		display: block;
		font-size: 13px;
		font-weight: 600;
	}
	.opt7 .s {
		display: block;
		font-size: 11px;
		color: var(--ink-muted);
		line-height: 1.35;
		padding-top: 2px;
	}
	.opt7.on {
		border-color: var(--gold-ink);
		background: var(--gold-tint);
	}
	.chk7 {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-height: 32px;
		font-size: 13px;
		cursor: pointer;
	}
	.chk7 input {
		width: 16px;
		height: 16px;
		margin: 0;
		accent-color: var(--gold-ink);
	}
	.go {
		display: flex;
		gap: 10px;
		align-items: center;
		flex-wrap: wrap;
		padding-top: 4px;
	}
	.code7 {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 6px 6px 12px;
		border-radius: 4px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--on-frame);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.code7 code {
		flex: 1;
		min-width: 0;
		overflow-x: auto;
		white-space: nowrap;
	}
	.code7 button {
		flex: 0 0 auto;
		height: 32px;
		padding: 0 10px;
		border-radius: 3px;
		border: 1px solid var(--frame-2);
		background: transparent;
		color: var(--on-frame);
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		cursor: pointer;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.35;
		}
	}
	.wait7 {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.lamp {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		box-sizing: border-box;
		background: var(--auto);
	}
	.wait7 .lamp {
		animation: pulse 1.4s ease-in-out infinite;
	}
	.lamp.off {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	@media (prefers-reduced-motion: reduce) {
		.wait7 .lamp {
			animation: none;
		}
	}
	.pre {
		flex: 1 1 300px;
		max-width: 400px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.lane {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr);
		gap: 16px;
		align-items: center;
		padding: 14px 16px;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
	}
	.stage {
		position: relative;
		width: 76px;
		height: 76px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: inset 0 2px 6px rgba(0, 0, 0, 0.14);
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
		text-align: center;
	}
	.ini {
		font:
			700 30px Alegreya,
			Georgia,
			serif;
		color: var(--ink-subtle);
	}
	.now {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.who {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.who b {
		font:
			700 18px/1.15 Alegreya,
			Georgia,
			serif;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.act {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0 8px;
		margin: 0;
		font-size: 14px;
	}
	.act b {
		font-weight: 600;
	}
	.act span,
	.m {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.note {
		margin: 0;
		font-size: 12px;
		line-height: 1.45;
	}
	.foot {
		margin: 0;
		font-size: 11px;
		color: var(--ink-subtle);
	}
</style>
