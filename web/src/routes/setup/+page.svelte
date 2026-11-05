<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { iconFor, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { rememberAccount } from '#lib/progress/account.svelte';
	import { routeLabel } from '#lib/progress/view';
	import type { AccountMode, AccountView, SetupLookup, SyncRun } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Primeiro acesso (V6Setup1440): RSN e modo, camadas do guia, rota inicial e primeiro sync. Um RSN
	 * que já tem conta entra direto no Agora (decisão do dono, 2026-11-04). Os passos 4 (progresso
	 * anterior do ironman.guide) e 5 (token do plugin) ficam para uma entrega seguinte. Escolher a
	 * conta pelo nome não é login: em AUTH_MODE=dev o app só escuta em loopback.
	 */
	const guides = createResource(() => progressApi.guides());

	const STEPS = [
		'RSN e modo',
		'Camadas do guia',
		'Rota inicial',
		'Progresso anterior',
		'Plugin do Microbot',
		'Primeiro sync'
	];
	let step = $state(0);

	let rsn = $state('');
	let lookup = $state<SetupLookup | null>(null);
	let looking = $state(false);
	let mode = $state<AccountMode>('ironman');
	let enhanced = $state(true);
	let guide = $state('oziris-main');
	let account = $state<AccountView | null>(null);
	let run = $state<SyncRun | null>(null);
	let busy = $state(false);
	let failure = $state<string | null>(null);

	const MODES: { id: AccountMode; label: string }[] = [
		{ id: 'ironman', label: 'Ironman' },
		{ id: 'hcim', label: 'Hardcore' },
		{ id: 'uim', label: 'Ultimate' },
		{ id: 'gim', label: 'Group ironman' }
	];
	const MODE_LABEL: Record<string, string> = {
		ironman: 'ironman',
		hcim: 'hardcore',
		uim: 'ultimate'
	};

	async function find() {
		const name = rsn.trim();
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
		void goto(`${resolve('/')}?account=${encodeURIComponent(id)}`);
	}

	async function create() {
		busy = true;
		failure = null;
		try {
			account = await progressApi.createAccount({
				rsn: rsn.trim(),
				mode,
				guide_id: guide,
				layer: enhanced ? 'enhanced' : 'classic'
			});
			rememberAccount(account.id);
			step = 5;
			try {
				run = await progressApi.sync(account.id);
			} catch (e) {
				failure = `Conta criada; o primeiro sync falhou: ${e instanceof Error ? e.message : String(e)}`;
			}
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	const top = $derived([...(lookup?.skills ?? [])].sort((a, b) => b.level - a.level).slice(0, 4));
	const ABBR: Record<string, string> = {
		attack: 'ATT',
		strength: 'STR',
		defence: 'DEF',
		ranged: 'RAN',
		prayer: 'PRA',
		magic: 'MAG',
		runecraft: 'RC',
		construction: 'CON',
		hitpoints: 'HP',
		agility: 'AGI',
		herblore: 'HER',
		thieving: 'THI',
		crafting: 'CRA',
		fletching: 'FLE',
		slayer: 'SLA',
		hunter: 'HUN',
		mining: 'MIN',
		smithing: 'SMI',
		fishing: 'FIS',
		cooking: 'COO',
		firemaking: 'FIR',
		woodcutting: 'WC',
		farming: 'FAR',
		sailing: 'SAI'
	};
	const AUTHORS: Record<string, string> = {
		'oziris-main': 'OzirisLoL · ironman.guide',
		'bruhsailer-main': 'So Iron BRUH e ParasailerOSRS'
	};
	const stepClass = (i: number) => (i < step ? 'stp-done' : i === step ? 'stp-on' : '');
</script>

<svelte:head>
	<title>Primeiro acesso · Chronicle</title>
</svelte:head>

<div class="page">
	<header class="head">
		<div class="ttl">
			<span
				class="sr wordmark"
				title="Uns dois minutos. Tudo que puder ser detectado será; o resto você marca à mão. Já tem conta? Digite o RSN e entre direto."
				><span class="logo"><Glyph name="book" size={26} /></span>Chronicle
				<span class="su desc">Ironman Planner · primeiro acesso</span></span
			>
			<h1 class="sr">Vamos ligar a conta</h1>
		</div>
	</header>

	{#if failure}
		<div class="band"><Glyph name="alert" /><span>{failure}</span></div>
	{/if}

	<div class="cols">
		<aside class="panel steps">
			{#each STEPS as s, i (s)}
				<div class="stp {stepClass(i)}">
					<span class="n"
						>{#if i < step}✓{:else}{i + 1}{/if}</span
					>{s}
				</div>
			{/each}
		</aside>

		<section class="main">
			<div class="panel box" class:dim={step > 0} class:on={step === 0}>
				<div class="plaque">
					1 · RSN e modo<span class="r"
						>{step > 0 ? `feito · ${MODE_LABEL[mode] ?? mode}` : 'o nome da conta no jogo'}</span
					>
				</div>
				<div class="pad row">
					<form
						class="find"
						onsubmit={(e) => {
							e.preventDefault();
							void find();
						}}
					>
						<label class="inp"
							><span class="su small">RSN</span><input
								bind:value={rsn}
								maxlength="12"
								autocomplete="off"
								disabled={step > 0}
								aria-label="Nome da conta"
							/></label
						>
						{#if step === 0}
							<button class="btn" type="submit" disabled={looking || !rsn.trim()}
								>{looking ? 'Procurando…' : 'Procurar'}</button
							>
						{/if}
					</form>
					{#if lookup?.account}
						<div class="known">
							<span class="badge badge-done">conta já cadastrada</span>
							<span class="mu small"
								>{lookup.account.rsn} · {lookup.account.mode} · rota {lookup.account.guide_id}</span
							>
							<button class="btn btn-p" type="button" onclick={() => enter(lookup!.account!.id)}
								>Entrar</button
							>
						</div>
					{:else if lookup}
						{#if lookup.hiscores_error}
							<span class="badge badge-stale">hiscores indisponível · escolha o modo</span>
						{:else if !lookup.found}
							<span class="badge badge-unk">não está no hiscores · escolha o modo</span>
						{:else if lookup.mode}
							<span class="badge badge-done">{MODE_LABEL[lookup.mode]} detectado pelo endpoint</span
							>
						{:else}
							<span class="badge badge-unk">só no hiscore normal · escolha o modo</span>
						{/if}
						{#if top.length}
							<div class="skls">
								{#each top as s (s.skill)}
									{@const icon = iconFor(`skill:${s.skill}`)}
									<div class="skl">
										<span class="ab"
											>{#if icon}<img src={icon} alt="" />{/if}{ABBR[s.skill] ?? s.skill}</span
										><span class="lv">{s.level}</span>
									</div>
								{/each}
								{#if lookup.total_level}<span class="su tiny"
										>+{lookup.skills.length - top.length} · total {lookup.total_level}</span
									>{/if}
							</div>
						{/if}
					{/if}
				</div>
				{#if lookup && !lookup.account && step === 0}
					<div class="pad opts">
						{#each MODES as m (m.id)}
							<label class="opt" class:opt-on={mode === m.id}
								><input type="radio" name="mode" value={m.id} bind:group={mode} /><span class="t"
									>{m.label}</span
								></label
							>
						{/each}
					</div>
					<div class="pad act">
						<button class="btn btn-p" type="button" onclick={() => (step = 1)}>Continuar</button>
					</div>
				{/if}
			</div>

			<div class="panel box" class:dim={step !== 1} class:on={step === 1}>
				<div class="plaque">
					2 · Camadas do guia<span class="r">{step > 1 ? 'feito' : 'o que aparece na rota'}</span>
				</div>
				<div class="pad chips">
					<span class="badge badge-gold">base · sempre</span>
					<button
						class="badge tog-badge"
						class:badge-gold={enhanced}
						type="button"
						disabled={step !== 1}
						aria-pressed={enhanced}
						title="Enhanced são os passos que o ironman.guide acrescentou ao guia original do Oziris"
						onclick={() => (enhanced = !enhanced)}>enhanced {enhanced ? '✓' : '· desligado'}</button
					>
				</div>
				{#if step === 1}
					<div class="pad act">
						<button class="btn btn-p" type="button" onclick={() => (step = 2)}>Continuar</button>
						<button class="btn btn-g" type="button" onclick={() => (step = 0)}>Voltar</button>
					</div>
				{/if}
			</div>

			<div class="panel box" class:dim={step !== 2} class:on={step === 2}>
				<div class="plaque">
					3 · Rota inicial<span
						class="r"
						title="O progresso é da conta, não da rota: o que você fizer seguindo uma vale na outra. Só o cursor e as marcas manuais são por rota."
						>uma entre N · troca depois sem perder o lugar</span
					>
				</div>
				<div class="pad grid">
					{#each [...(guides.data?.guides ?? [])].sort( (a, b) => (a.id.startsWith('oziris') ? -1 : b.id.startsWith('oziris') ? 1 : 0) ) as g (g.id)}
						<label class="opt" class:opt-on={guide === g.id}>
							<input
								type="radio"
								name="rota"
								value={g.id}
								bind:group={guide}
								disabled={step !== 2}
							/>
							<span class="col">
								<span class="t">{routeLabel(g.id, g.title)}</span>
								<span class="d"
									>{AUTHORS[g.id] ?? g.source?.author ?? ''} · {g.steps} passos, {g.sections.length}
									seções, {g.segments} trechos</span
								>
							</span>
						</label>
					{/each}
				</div>
				{#if step === 2}
					<div class="pad act">
						<button class="btn btn-p" type="button" disabled={busy} onclick={create}
							>{busy ? 'Criando…' : 'Criar a conta'}</button
						>
						<button class="btn btn-g" type="button" onclick={() => (step = 1)}>Voltar</button>
					</div>
				{/if}
			</div>

			<div class="panel box dim">
				<div class="plaque">
					4 · Progresso anterior<span
						class="r"
						title="Importar o JSON do ironman.guide chega numa próxima entrega. Até lá, marque na Rota."
						>import do ironman.guide · em breve</span
					>
				</div>
			</div>

			<div class="panel box dim">
				<div class="plaque">
					5 · Plugin do Microbot<span
						class="r"
						title="Em dev o plugin se identifica sem token pelo loopback. Sem plugin, quests, banco e itens ficam “não sabemos”."
						>opcional · somente leitura</span
					>
				</div>
			</div>

			<div class="panel box" class:dim={step !== 5} class:on={step === 5}>
				<div class="plaque">6 · Primeiro sync<span class="r">com progresso visível</span></div>
				<div class="pad chips">
					{#if run}
						<span class="badge" class:badge-done={run.status === 'ok' || run.status === 'unchanged'}
							>hiscores · {run.status === 'ok'
								? `${run.steps_changed} passos detectados`
								: run.status === 'unchanged'
									? 'sem mudança'
									: (run.error ?? run.status)}</span
						>
					{:else if step === 5 && busy}
						<span class="su small">Sincronizando com o hiscores…</span>
					{/if}
					<span class="badge badge-unk">quests · não sabemos sem leitura do plugin</span>
					<span class="badge badge-unk">banco · sem plugin</span>
				</div>
				{#if account}
					<div class="pad act">
						<button class="btn btn-p" type="button" onclick={() => enter(account!.id)}
							>Abrir o Agora</button
						>
					</div>
				{/if}
			</div>
		</section>
	</div>

	<p class="su tiny">
		Guia por OzirisLoL / ironman.guide · So Iron BRUH e ParasailerOSRS. {ICON_ATTRIBUTION}.
	</p>
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 24px;
		padding: 16px 16px 24px;
		max-width: 1440px;
		box-sizing: border-box;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.ttl h1 {
		font-size: 32px;
	}
	.wordmark {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		font-size: 22px;
		color: var(--gold-ink);
	}
	.logo {
		display: inline-flex;
	}
	.desc {
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
		margin-left: 4px;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		align-items: flex-start;
	}
	.steps {
		flex: 1 1 240px;
		max-width: 280px;
		padding: 8px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.stp {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 40px;
		padding: 0 12px;
		border-radius: 4px;
		color: var(--ink-muted);
		font-size: 13px;
	}
	.stp .n {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		border: 1.5px solid var(--border);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font:
			600 11px/1 Inter,
			sans-serif;
		flex: 0 0 auto;
		box-sizing: border-box;
	}
	.stp-done .n {
		background: var(--done);
		border-color: var(--done);
		color: var(--ground);
	}
	.stp-on {
		background: var(--elevated);
		box-shadow: var(--bevel);
		color: var(--ink);
	}
	.stp-on .n {
		background: var(--gold-ink);
		border-color: var(--gold-ink);
		color: var(--ground);
	}
	.main {
		flex: 999 1 560px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.box {
		display: flex;
		flex-direction: column;
	}
	.box.dim {
		opacity: 0.75;
	}
	.box.on {
		box-shadow:
			var(--bevel),
			0 0 0 1px var(--gold-ink);
		opacity: 1;
	}
	.pad {
		padding: 14px;
	}
	.pad + .pad {
		padding-top: 0;
	}
	.row,
	.chips,
	.find,
	.known,
	.skls,
	.opts,
	.act {
		display: flex;
		flex-wrap: wrap;
		gap: 10px 14px;
		align-items: center;
	}
	.inp {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 40px;
		padding: 0 12px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		color: var(--ink);
		font-size: 15px;
		min-width: 0;
		flex: 1 1 240px;
		max-width: 320px;
	}
	.inp input {
		flex: 1;
		min-width: 0;
		background: transparent;
		border: 0;
		outline: 0;
		color: var(--ink);
		font: inherit;
	}
	.opt {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		padding: 12px 14px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--elevated);
		box-shadow: var(--bevel);
		cursor: pointer;
	}
	.opt input {
		margin: 4px 0 0;
		accent-color: var(--gold-ink);
	}
	.opt .t {
		font-weight: 500;
		font-size: 13px;
	}
	.opt .d {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.opt-on {
		border-color: var(--gold-ink);
		box-shadow:
			var(--bevel),
			0 0 0 1px var(--gold-ink);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr));
		gap: 10px;
	}
	.col {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.full {
		grid-column: 1 / -1;
	}
	.skl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 10px;
		border-radius: 2px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		min-width: 0;
		color: var(--on-frame-muted);
	}
	.skl .ab {
		display: flex;
		align-items: center;
		gap: 5px;
		font:
			500 10px/1 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.skl .ab img {
		width: 16px;
		height: 16px;
		image-rendering: pixelated;
	}
	.skl .lv {
		font:
			600 20px/1.1 Inter,
			system-ui,
			sans-serif;
		font-variant-numeric: tabular-nums;
		color: var(--gold);
	}
	.tog-badge {
		border: 0;
		cursor: pointer;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	p.tiny {
		margin: 0;
	}
</style>
