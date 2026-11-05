<script lang="ts">
	import { progressApi } from '#lib/api/client';
	import { quietDetection } from '#lib/api/events.svelte';
	import { createResource } from '#lib/api/resource.svelte';
	import { combatLevel, equipmentLayout, shortQty, SKILL_ORDER, toNext } from '#lib/progress/conta';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor, itemIcon, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { loadMe, me } from '#lib/progress/me.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { ago } from '#lib/progress/view';
	import type { ContainerView, DiaryView, QuestView } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';
	import Toast from '#lib/v6/Toast.svelte';

	/*
	 * Conta (V6Conta1440 e V6Conta640): skills, quests e diaries com fonte e idade primeiro, depois
	 * os containers com frescor, o banco e as fontes no fim. Facetas que o plugin nunca mandou ficam
	 * "sem leitura": nunca viram ausência (regra 8). O equipamento vai no boneco do jogo: cada item
	 * ocupa um slot só, o do catálogo da wiki.
	 */
	const account = $derived(session.account);
	const st = $derived(me.state);
	const runs = createResource(
		() => (account ? progressApi.syncRuns(account, 1).catch(() => null) : Promise.resolve(null)),
		{ tags: ['sync'] }
	);
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});
	const age = (at: string | undefined) => (at ? ago(Date.parse(at), now).replace('há ', '') : '');

	const MODE: Record<string, string> = {
		ironman: 'Ironman',
		hcim: 'Hardcore',
		uim: 'Ultimate',
		gim: 'Group'
	};
	const skills = $derived(
		[...(st?.skills ?? [])].sort(
			(a, b) => SKILL_ORDER.indexOf(a.skill) - SKILL_ORDER.indexOf(b.skill)
		)
	);
	const skillSrc = $derived.by(() => {
		const newest = skills.reduce<{ source: string; at: string } | null>(
			(m, s) => (!m || s.observed_at > m.at ? { source: s.source, at: s.observed_at } : m),
			null
		);
		return newest ? `${newest.source} · ${age(newest.at)}` : 'sem leitura';
	});
	const combat = $derived(combatLevel(skills));
	const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

	/* Quests */
	let qFilter = $state<'active' | 'done' | 'missing' | 'all'>('active');
	let qQuery = $state('');
	const quests = $derived(st?.quests ?? []);
	const doneQ = $derived(quests.filter((q) => q.state === 'FINISHED').length);
	const shownQ = $derived.by(() => {
		const q = qQuery.trim().toLowerCase();
		const pick = (x: QuestView) =>
			qFilter === 'all' ||
			(qFilter === 'active' && x.state === 'IN_PROGRESS') ||
			(qFilter === 'done' && x.state === 'FINISHED') ||
			(qFilter === 'missing' && x.state !== 'FINISHED');
		return quests
			.filter((x) => pick(x) && (!q || x.name.toLowerCase().includes(q)))
			.sort((a, b) => (b.observed_at > a.observed_at ? 1 : -1));
	});
	const qSrc = $derived(quests[0] ? `${quests[0].source} · ${age(quests[0].observed_at)}` : '');
	const Q_STATE: Record<string, string> = {
		IN_PROGRESS: 'iniciada',
		NOT_STARTED: 'não iniciada',
		FINISHED: ''
	};

	/* Diaries: uma linha por área, quatro bolinhas; a área aberta mostra os tiers. */
	const TIERS = ['easy', 'medium', 'hard', 'elite'] as const;
	const TIER_LABEL: Record<string, string> = {
		easy: 'Easy',
		medium: 'Medium',
		hard: 'Hard',
		elite: 'Elite'
	};
	const areas = $derived.by(() => {
		const by: Record<string, DiaryView[]> = {};
		for (const d of st?.diaries ?? []) (by[d.area] ??= []).push(d);
		return Object.entries(by).map(([area, ds]) => ({
			area,
			name: area.split('_').map(cap).join(' ').replace(/ And /g, ' & '),
			tiers: TIERS.map((t) => ds.find((d) => d.tier === t))
		}));
	});
	const doneD = $derived((st?.diaries ?? []).filter((d) => d.complete).length);
	const dSrc = $derived(
		st?.diaries[0] ? `${st.diaries[0].source} · ${age(st.diaries[0].observed_at)}` : ''
	);
	let focus = $state<string | null>(null);
	const focused = $derived(areas.find((a) => a.area === focus) ?? null);

	/* Containers e banco */
	const cont = (name: string): ContainerView | undefined =>
		st?.containers.find((c) => c.container === name);
	const inv = $derived(cont('inventory'));
	const eq = $derived(cont('equipment'));
	const bank = $derived(cont('bank'));
	function fresh(c: ContainerView | undefined): { text: string; live: boolean } {
		if (!c || c.state === 'unavailable') return { text: 'sem leitura', live: false };
		if (c.state === 'restored') return { text: `espelho · ${age(c.observed_at)}`, live: false };
		return { text: `ao vivo · ${age(c.observed_at)}`, live: true };
	}
	let bQuery = $state('');
	let bOrder = $state<'game' | 'name' | 'qty'>('game');
	const bankItems = $derived.by(() => {
		const q = bQuery.trim().toLowerCase();
		const list = (bank?.items ?? []).filter((i) => !q || i.name.toLowerCase().includes(q));
		if (bOrder === 'name') return [...list].sort((a, b) => a.name.localeCompare(b.name));
		if (bOrder === 'qty') return [...list].sort((a, b) => b.qty - a.qty);
		return list;
	});

	/* Sincronizar hiscores */
	let syncing = $state(false);
	let toast = $state<string | null>(null);
	async function syncNow() {
		if (!account || syncing) return;
		syncing = true;
		quietDetection('hiscores');
		try {
			const run = await progressApi.sync(account);
			toast =
				run.status === 'ok'
					? `${run.steps_changed} passos mudaram`
					: run.status === 'unchanged'
						? 'nada mudou desde o último sync'
						: (run.error ?? 'falhou');
			void loadMe(account);
			void loadSummary(account);
		} catch (e) {
			const tooSoon =
				e instanceof Error && 'status' in e && (e as { status: number }).status === 429;
			toast = tooSoon ? 'aguarde: um sync por minuto, e nunca durante o recuo' : String(e);
		} finally {
			syncing = false;
			runs.reload();
		}
	}
	const SOURCE: Record<string, string> = {
		plugin: 'Plugin (Microbot)',
		hiscores: 'Hiscores',
		wom: 'Wise Old Man',
		wikisync: 'WikiSync'
	};
	const lastRun = $derived(runs.data?.runs[0]);
</script>

<svelte:head>
	<title>Conta · Chronicle</title>
</svelte:head>

{#snippet item(i: { id: number; name: string; qty: number }, extra = '')}
	{@const icon = itemIcon(i.id)}
	<button
		class="it {extra}"
		type="button"
		title={`${i.name}${i.qty > 1 ? ` ×${i.qty.toLocaleString('pt-BR')}` : ''}`}
		onclick={() => openEntity(`item:${i.id}`)}
		>{#if icon}<img src={icon} alt="" />{:else}<span class="ab">{i.name.slice(0, 3)}</span
			>{/if}{#if i.qty > 1}<span class="q">{shortQty(i.qty)}</span>{/if}</button
	>
{/snippet}

{#snippet container(title: string, c: ContainerView | undefined, slots: number, equip = false)}
	{@const f = fresh(c)}
	<section class="panel pb">
		<div class="plaque">
			{title}<span class="r"
				>{#if c}<span class="dot dotin" class:dot-w={!f.live}></span>{/if}{f.text}{#if c && slots}
					· {c.items.length} de {slots}{/if}</span
			>
		</div>
		{#if c && c.state !== 'unavailable'}
			<div class="cbody ctr">
				{#if equip}
					{@const lay = equipmentLayout(c.items)}
					<div class="eq">
						{#each lay.cells as cell (cell.slot)}
							<div class="slotcell" style="grid-row: {cell.row}; grid-column: {cell.col};">
								{#if cell.item}{@render item(cell.item)}{:else}<span
										class="it e"
										title={cell.blockedBy2h
											? `${cell.label}: vazio (arma de duas mãos)`
											: `${cell.label}: vazio`}
									></span>{/if}
							</div>
						{/each}
					</div>
					{#if lay.rest.length}
						<div class="g4">
							{#each lay.rest as i (i.id)}{@render item(i)}{/each}
						</div>
					{/if}
				{:else}
					<div class="g4">
						{#each c.items as i (i.id)}{@render item(i)}{/each}
						{#each Array.from({ length: Math.max(0, slots - c.items.length) }, (_v, n) => n) as n (n)}<span
								class="it e"
								aria-hidden="true"
							></span>{/each}
					</div>
				{/if}
			</div>
		{:else}
			<div
				class="emp"
				title="Sem leitura não é ausência: o plugin lê o container quando ele muda no jogo"
			>
				<span>sem leitura ainda</span>
			</div>
		{/if}
	</section>
{/snippet}

{#if !account}
	<div class="band"><Glyph name="alert" /><span>Sem conta: abra o primeiro acesso.</span></div>
{:else if !st}
	<p class="su" aria-busy="true">Carregando a conta…</p>
{:else}
	<header class="top">
		<div class="ttl">
			<p class="su small">Conta</p>
			<h1 class="sr">
				{st.account.rsn}
				<span class="su sub"
					>{MODE[st.account.mode] ?? st.account.mode}{st.total_level
						? ` · ${st.total_level} total`
						: ''}{st.quest_points !== undefined ? ` · ${st.quest_points} QP` : ''}</span
				>
			</h1>
		</div>
		<button class="btn btn-s push" type="button" disabled={syncing} onclick={syncNow}
			><Glyph name="sync" size={14} />{syncing ? 'Sincronizando…' : 'Sincronizar hiscores'}</button
		>
	</header>

	<div class="row1">
		<section class="panel pb" id="skills">
			<div class="plaque">Skills<span class="r">{skillSrc}</span></div>
			{#if skills.length}
				<div class="sk">
					{#each skills as s (s.skill)}
						{@const icon = iconFor(`skill:${s.skill}`)}
						{@const p = toNext(s.level, s.xp)}
						<button
							class="skc"
							type="button"
							title={`${cap(s.skill)} ${s.level} · ${s.source} há ${age(s.observed_at)}${s.level < 99 ? ` · ${Math.round(p * 100)}% até ${s.level + 1}` : ''}`}
							onclick={() => openEntity(`skill:${s.skill}`)}
							>{#if icon}<img src={icon} alt="" />{/if}<span class="nm">{cap(s.skill)}</span><span
								class="lv">{s.level}</span
							><i class="xp" style="width: {p * 100}%"></i></button
						>
					{/each}
				</div>
				<div class="skf">
					<span class="skc" title="Soma dos níveis"
						><span class="nm">Total</span><span class="lv">{st.total_level ?? '—'}</span></span
					>
					<span class="skc" title="Nível de combate"
						><span class="nm">Combate</span><span class="lv">{combat ?? '—'}</span></span
					>
					<span class="skc" title="Quest points (varp 101 do plugin)"
						><span class="nm">Quest points</span><span class="lv">{st.quest_points ?? '?'}</span
						></span
					>
				</div>
			{:else}
				<div class="emp"><span>sem leitura ainda</span></div>
			{/if}
		</section>

		<section class="panel pb fixh" id="quests">
			<div class="plaque">
				Quests<span class="r"
					>{qSrc ? `${qSrc} · ${doneQ} de ${quests.length}` : 'sem leitura'}</span
				>
			</div>
			<div class="qs">
				<label class="inp qin"
					><Glyph name="search" size={14} /><input
						type="search"
						bind:value={qQuery}
						placeholder="buscar quest"
						aria-label="Buscar quest"
					/></label
				>
			</div>
			<div class="chips">
				{#each [['active', 'Ativas'], ['done', 'Feitas'], ['missing', 'Faltam'], ['all', 'Tudo']] as [k, l] (k)}
					<button
						type="button"
						class:on={qFilter === k}
						aria-pressed={qFilter === k}
						onclick={() => (qFilter = k as typeof qFilter)}>{l}</button
					>
				{/each}
			</div>
			<div class="scroll">
				{#each shownQ as q (q.name)}
					<button
						class="qrow"
						class:done={q.state === 'FINISHED'}
						type="button"
						onclick={() => q.id && openEntity(`quest:${q.id}`)}
						><span
							class="vd"
							class:vd-met={q.state === 'FINISHED'}
							class:vd-prog={q.state === 'IN_PROGRESS'}
							class:vd-unmet={q.state === 'NOT_STARTED'}
							>{#if q.state === 'FINISHED'}<Glyph name="check" size={9} />{/if}</span
						><span>{q.name}</span><span class="w"
							>{[Q_STATE[q.state], age(q.observed_at)].filter(Boolean).join(' · ')}</span
						></button
					>
				{:else}
					<p class="su small pad">
						{quests.length ? 'Nenhuma quest neste filtro.' : 'O plugin ainda não leu as quests.'}
					</p>
				{/each}
			</div>
		</section>

		<section class="panel pb fixh" id="diaries">
			<div class="plaque">
				Diaries<span class="r"
					>{dSrc ? `${dSrc} · ${doneD} de ${st.diaries.length}` : 'sem leitura'}</span
				>
			</div>
			{#if focused}
				<div class="pb grow">
					<button class="drow back" type="button" onclick={() => (focus = null)}
						><span class="flip"><Glyph name="chevron" size={12} /></span><span>todas as áreas</span
						></button
					>
					<div class="fh">
						<span class="sr">{focused.name}</span>
					</div>
					<div class="scroll">
						{#each focused.tiers as d, i (i)}
							<button
								class="task tbtn"
								type="button"
								onclick={() => openEntity(`diary:${focused.area}:${TIERS[i]}`)}
								><span class="vd" class:vd-met={d?.complete} class:vd-unmet={d && !d.complete}
									>{#if d?.complete}<Glyph name="check" size={9} />{:else if !d}?{/if}</span
								><span
									>{TIER_LABEL[TIERS[i]]}{d
										? d.complete
											? ' · completo'
											: ' · falta'
										: ' · sem leitura'}</span
								></button
							>
						{/each}
					</div>
				</div>
			{:else if areas.length}
				<div class="pb grow">
					<div class="dhead">
						<span></span><span class="c"
							><span>E</span><span>M</span><span>H</span><span>EL</span></span
						><span></span><span></span>
					</div>
					<div class="scroll">
						{#each areas as a (a.area)}
							<button class="drow" type="button" onclick={() => (focus = a.area)}
								><span>{a.name}</span><span class="dots4"
									>{#each a.tiers as d, i (i)}<span
											class="vd"
											class:vd-met={d?.complete}
											class:vd-unmet={d && !d.complete}
											>{#if d?.complete}<Glyph name="check" size={9} />{/if}</span
										>{/each}</span
								><span class="n">{a.tiers.filter((d) => d?.complete).length}/4</span><Glyph
									name="chevron"
									size={12}
								/></button
							>
						{/each}
					</div>
				</div>
			{:else}
				<div class="emp"><span>o plugin ainda não leu os diaries</span></div>
			{/if}
		</section>
	</div>

	<div class="cgrid" id="itens">
		{@render container('Equipamento', eq, 0, true)}
		{@render container('Inventário', inv, 28)}
	</div>

	<section class="panel pb" id="banco">
		<div class="plaque">
			Banco<span
				class="r"
				title="O banco é lido quando você o abre no jogo; até lá o planner usa a última leitura"
				>{fresh(bank).text}{bank ? ` · ${bank.items.length} itens` : ''}</span
			>
		</div>
		{#if bank && bank.state !== 'unavailable'}
			<div class="bbody">
				<div class="bctl">
					<label class="inp bin"
						><Glyph name="search" size={14} /><input
							type="search"
							bind:value={bQuery}
							placeholder="buscar item"
							aria-label="Buscar item no banco"
						/></label
					>
					<div class="seg push" role="group" aria-label="Ordem">
						{#each [['game', 'Como no jogo'], ['name', 'Nome'], ['qty', 'Quantidade']] as [k, l] (k)}
							<button
								type="button"
								class:on={bOrder === k}
								aria-pressed={bOrder === k}
								onclick={() => (bOrder = k as typeof bOrder)}>{l}</button
							>
						{/each}
					</div>
				</div>
				<div class="bg ctr">
					{#each bankItems as i (i.id)}{@render item(i)}{/each}
				</div>
				<div class="bfoot su">
					<span>{bankItems.length} de {bank.items.length}</span>
				</div>
			</div>
		{:else}
			<div class="emp"><span>sem leitura ainda · abra o banco no jogo</span></div>
		{/if}
	</section>

	<section class="panel" id="fontes">
		<div class="plaque">Fontes<span class="r">de onde vem cada dado</span></div>
		{#each session.summary?.sources ?? [] as s (s.source)}
			{@const stale = now - Date.parse(s.last_observed_at) > 24 * 3600_000}
			<div class="src2">
				<span class="dot" class:dot-w={stale}></span><span>{SOURCE[s.source] ?? s.source}</span
				><span class="t"
					>há {age(s.last_observed_at)} · {s.facts} fatos{s.source === 'hiscores' && lastRun
						? ` · último sync ${lastRun.status}`
						: ''}</span
				><span></span>
			</div>
		{:else}
			<div class="src2">
				<span class="dot dot-w"></span><span>Nenhuma fonte ainda</span><span class="t"></span><span
				></span>
			</div>
		{/each}
	</section>
	<p class="su tiny">{ICON_ATTRIBUTION}.</p>
{/if}

{#if toast}
	<Toast title="Hiscores" text={toast} onclose={() => (toast = null)} />
{/if}

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.ttl p {
		margin: 0;
	}
	.ttl h1 {
		font-size: 26px;
	}
	.sub {
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
	}
	.push {
		margin-left: auto;
	}
	.row1 {
		display: grid;
		grid-template-columns: minmax(0, 480px) minmax(0, 1fr) minmax(0, 330px);
		gap: 16px;
		align-items: stretch;
	}
	@media (max-width: 1200px) {
		.row1 {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
		#skills {
			grid-column: 1 / -1;
		}
	}
	@media (max-width: 900px) {
		.row1 {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.fixh {
		height: 468px;
	}
	.grow {
		flex: 1;
		min-height: 0;
	}
	.scroll {
		flex: 1;
		min-height: 0;
		overflow: auto;
	}
	.qs {
		padding: 10px 10px 8px;
	}
	.qin {
		height: 36px;
	}
	button.skc,
	button.qrow,
	button.it {
		font: inherit;
		background: transparent;
		border-top: 0;
		border-left: 0;
		cursor: pointer;
		text-align: left;
		width: 100%;
		color: var(--ink);
	}
	button.qrow {
		border-right: 0;
	}
	button.it {
		width: 40px;
		padding: 0;
		border: 1px solid var(--hairline);
		background: var(--ground);
	}
	.ab {
		font:
			500 9px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		text-transform: uppercase;
	}
	.slotcell {
		width: 40px;
		height: 40px;
	}
	.cbody {
		gap: 10px;
	}
	.dotin {
		display: inline-block;
		margin-right: 6px;
		vertical-align: middle;
	}
	.back {
		grid-template-columns: 14px 1fr;
		min-height: 34px;
		color: var(--ink-subtle);
		font-size: 12px;
	}
	.flip {
		display: inline-flex;
		transform: rotate(180deg);
	}
	.fh {
		padding: 10px 12px 6px;
	}
	.fh .sr {
		font-size: 17px;
	}
	.tbtn {
		width: 100%;
		border: 0;
		background: transparent;
		font: inherit;
		color: var(--ink);
		cursor: pointer;
		text-align: left;
	}
	.cgrid {
		grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
	}
	.bbody {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 12px 14px;
	}
	.bctl {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.bin {
		flex: 1 1 200px;
		max-width: 320px;
		height: 36px;
	}
	.bfoot {
		display: flex;
		gap: 12px;
		font-size: 12px;
	}
	.pad {
		padding: 10px 12px;
		margin: 0;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
</style>
