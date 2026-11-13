<script lang="ts">
	import { progressApi } from '#lib/api/client';
	import { toNext, shortQty, SKILL_ORDER } from '#lib/progress/conta';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { iconFor, itemIcon, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import { me } from '#lib/progress/me.svelte';
	import {
		bankSource,
		carriedSource,
		conquests,
		diaryRows,
		factSource,
		QUEST_MORE,
		questCounts,
		questRows,
		summaryCells,
		type QuestFilter
	} from '#lib/progress/personagem';
	import { todaySince, upToday } from '#lib/progress/ritmo';
	import { session } from '#lib/progress/session.svelte';
	import type { AccountRhythm, ContainerView, QuestView } from '#lib/progress/types';
	import { ago } from '#lib/progress/view';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Conta › Personagem (página 8 do canvas, V7Personagem1440; fatia V4 de decisoes-canvas.md, 7.5):
	 * o resumo, as skills com o caminho até o próximo nível, quests, diaries, o que ela leva agora,
	 * as conquistas e o banco, cada cartão com a fonte e a idade da leitura. O que o plugin nunca
	 * mandou fica "sem leitura" e nunca vira ausência (regra 8). Em verde, as skills que subiram hoje,
	 * pelo ritmo (fatia V5).
	 */
	const account = $derived(session.account);
	const st = $derived(me.state);
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});

	const cells = $derived(st ? summaryCells(st) : []);
	const skills = $derived(
		[...(st?.skills ?? [])].sort(
			(a, b) => SKILL_ORDER.indexOf(a.skill) - SKILL_ORDER.indexOf(b.skill)
		)
	);
	const skillSrc = $derived(factSource(skills, now));

	/* O que subiu desde a meia-noite deste navegador (GET /api/account/rhythm). */
	let rhythm = $state<AccountRhythm | null>(null);
	$effect(() => {
		const id = account;
		rhythm = null;
		if (!id) return;
		const read = () =>
			progressApi
				.rhythm(id, todaySince(new Date()))
				.then((r) => {
					if (r.account === id) rhythm = r;
				})
				.catch(() => {
					/* sem o ritmo, nada fica em verde */
				});
		void read();
		const t = setInterval(() => {
			if (!document.hidden) void read();
		}, 60_000);
		return () => clearInterval(t);
	});
	const ups = $derived(upToday(rhythm));

	/* Quests: seis por filtro, o resto atrás do link. */
	const SHOW = 6;
	let qFilter = $state<QuestFilter>('active');
	let qQuery = $state('');
	let qAll = $state(false);
	const quests = $derived(st?.quests ?? []);
	const qCount = $derived(questCounts(quests));
	const qRows = $derived(questRows(quests, qFilter, qQuery));
	const qShown = $derived(qAll || qQuery.trim() ? qRows : qRows.slice(0, SHOW));
	const FILTERS: [QuestFilter, string][] = [
		['active', 'Em andamento'],
		['done', 'Feitas'],
		['missing', 'Faltam']
	];
	const Q_VD: Record<QuestView['state'], string> = {
		FINISHED: 'met',
		IN_PROGRESS: 'prog',
		NOT_STARTED: 'unmet'
	};
	function pick(f: QuestFilter) {
		qFilter = f;
		qAll = false;
	}

	/* Diaries: seis áreas, as outras atrás do link. */
	let dAll = $state(false);
	const diaries = $derived(diaryRows(st?.diaries ?? []));
	const dShown = $derived(dAll ? diaries : diaries.slice(0, SHOW));
	const TIER: Record<string, string> = {
		easy: 'Easy',
		medium: 'Medium',
		hard: 'Hard',
		elite: 'Elite'
	};
	const DOT: Record<string, string> = { on: 'completo', off: 'falta', unk: 'sem leitura' };

	const conq = $derived(conquests(st?.combat_achievements ?? [], st?.collection_log));

	/* O que ela leva e o banco. */
	const cont = (name: string): ContainerView | undefined =>
		st?.containers.find((c) => c.container === name && c.state !== 'unavailable');
	const inv = $derived(cont('inventory'));
	const eq = $derived(cont('equipment'));
	const bank = $derived(cont('bank'));
	const carried = $derived(carriedSource([inv, eq], now));
	const bankSrc = $derived(bankSource(bank, now));
	const BANK_SHOW = 48;
	let bQuery = $state('');
	let bAll = $state(false);
	const bankItems = $derived.by(() => {
		const q = bQuery.trim().toLowerCase();
		return (bank?.items ?? []).filter((i) => !q || i.name.toLowerCase().includes(q));
	});
	const bankShown = $derived(bAll || bQuery.trim() ? bankItems : bankItems.slice(0, BANK_SHOW));
	const n = (v: number) => v.toLocaleString('pt-BR');
	const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
</script>

<svelte:head>
	<title>{st?.account.rsn ?? 'Personagem'} · Personagem · Chronicle</title>
</svelte:head>

{#snippet source(f: { lamp: string; text: string })}
	<span class="src7"><span class="lamp {f.lamp}"></span>{f.text}</span>
{/snippet}

{#snippet slot(i: { id: number; name: string; qty: number })}
	{@const icon = itemIcon(i.id)}
	<button
		class="bs7"
		type="button"
		title={`${i.name}${i.qty > 1 ? ` ×${n(i.qty)}` : ''}`}
		onclick={() => openEntity(`item:${i.id}`)}
		>{#if icon}<img src={icon} alt={i.name} />{:else}<span class="ab">{i.name.slice(0, 3)}</span
			>{/if}{#if i.qty > 1}<span>{shortQty(i.qty)}</span>{/if}</button
	>
{/snippet}

{#if !account}
	<p class="mu">Sem conta aberta.</p>
{:else if !st}
	<p class="mu" aria-busy="true">Lendo a conta…</p>
{:else}
	{#if cells.length}
		<section class="sum7" aria-label="Resumo do personagem">
			{#each cells as c (c.k)}
				<div class="kv">
					<span class="k">{c.k}</span><span class="val"
						>{c.v}{#if c.of}
							<small>{c.of}</small>{/if}</span
					>
				</div>
			{/each}
		</section>
	{/if}

	<div class="row1">
		<section class="card skills">
			<div class="h">
				<h2>Skills</h2>
				{@render source(skillSrc)}
			</div>
			<div class="b">
				{#if skills.length}
					<div class="skg7">
						{#each skills as s (s.skill)}
							{@const icon = iconFor(`skill:${s.skill}`)}
							{@const p = toNext(s.level, s.xp)}
							<button
								class="sk7"
								class:max={s.level >= 99}
								class:up={!!ups.levels[s.skill]}
								type="button"
								title={`${cap(s.skill)} ${s.level}${ups.levels[s.skill] ? ` (+${ups.levels[s.skill]} hoje)` : ''} · ${n(s.xp)} xp${s.level < 99 ? ` · a barra é o caminho até o ${s.level + 1}: ${Math.round(p * 100)}%` : ''} · ${s.source} ${ago(Date.parse(s.observed_at), now)}`}
								onclick={() => openEntity(`skill:${s.skill}`)}
								>{#if icon}<img src={icon} alt="" />{/if}<span class="nm">{cap(s.skill)}</span><span
									class="lv7">{s.level}</span
								><span class="bar7"><span style="width: {p * 100}%"></span></span></button
							>
						{/each}
					</div>
					{#if ups.text}
						<p class="mu subiu">Em verde, o que subiu hoje: {ups.text}.</p>
					{/if}
				{:else}
					<p class="mu">Sem leitura ainda.</p>
				{/if}
			</div>
		</section>

		<section class="card quests">
			<div class="h">
				<h2>Quests</h2>
				{#if quests.length}<span>{n(qCount.done)} de {n(quests.length)}</span>{/if}
			</div>
			<div class="b">
				{#if quests.length}
					<label class="field fs">
						<Glyph name="search" size={14} /><input
							type="search"
							placeholder="buscar quest"
							aria-label="Buscar quest"
							bind:value={qQuery}
						/>
					</label>
					<div class="flt7" role="group" aria-label="Filtro das quests">
						{#each FILTERS as [k, l] (k)}
							<button
								type="button"
								class:on={qFilter === k}
								aria-pressed={qFilter === k}
								onclick={() => pick(k)}>{l} <span>{n(qCount[k])}</span></button
							>
						{/each}
					</div>
					<div>
						{#each qShown as q (q.name)}
							<button
								class="qr7"
								type="button"
								disabled={!q.id}
								onclick={() => q.id && openEntity(`quest:${q.id}`)}
								><span class="vd7 {Q_VD[q.state]}"
									>{#if q.state === 'FINISHED'}<Glyph name="check" size={9} />{/if}</span
								><span>{q.name}</span><span class="w"
									>{q.state === 'IN_PROGRESS' ? 'iniciada' : ''}</span
								></button
							>
						{:else}
							<p class="mu">Nenhuma quest neste filtro.</p>
						{/each}
					</div>
					{#if qShown.length < qRows.length}
						<button class="lnk more" type="button" onclick={() => (qAll = true)}
							>mais {n(qRows.length - qShown.length)} {QUEST_MORE[qFilter]}</button
						>
					{/if}
				{:else}
					<p class="mu">O plugin ainda não leu as quests.</p>
				{/if}
			</div>
		</section>
	</div>

	<div class="row2">
		<section class="card">
			<div class="h">
				<h2>Diaries</h2>
				<span>fácil · médio · difícil · elite</span>
			</div>
			<div class="b tight">
				{#each dShown as d (d.area)}
					<button
						class="qr7"
						type="button"
						title={d.tiers.map((t) => `${TIER[t.tier]}: ${DOT[t.dot]}`).join(' · ')}
						onclick={() => openEntity(`diary:${d.area}:${d.next}`)}
						><span></span><span>{d.name}</span><span class="dots7"
							>{#each d.tiers as t (t.tier)}<i class:on={t.dot === 'on'} class:unk={t.dot === 'unk'}
								></i>{/each}</span
						></button
					>
				{:else}
					<p class="mu">O plugin ainda não leu os diaries.</p>
				{/each}
				{#if !dAll && diaries.length > SHOW}
					<button class="lnk more" type="button" onclick={() => (dAll = true)}
						>os {diaries.length} diários</button
					>
				{/if}
			</div>
		</section>

		<section class="card">
			<div class="h">
				<h2>Com ela agora</h2>
				{@render source(carried)}
			</div>
			<div class="b">
				{#if eq || inv}
					{#if eq}
						<p class="lbl">Equipado</p>
						{#if eq.items.length}
							<div class="bankg7 eq">
								{#each eq.items as i (i.id)}{@render slot(i)}{/each}
							</div>
						{:else}
							<p class="mu">Nada equipado.</p>
						{/if}
					{/if}
					{#if inv}
						<p class="lbl">Inventário · {inv.items.length} de 28</p>
						{#if inv.items.length}
							<div class="bankg7 inv">
								{#each inv.items as i, k (k)}{@render slot(i)}{/each}
							</div>
						{:else}
							<p class="mu">Inventário vazio.</p>
						{/if}
					{/if}
				{:else}
					<p class="mu">Sem leitura ainda.</p>
				{/if}
			</div>
		</section>

		<section class="card">
			<div class="h">
				<h2>Conquistas</h2>
				<span>pelo jogo</span>
			</div>
			<div class="b">
				{#each conq.rows as r (r.tier)}
					<div class="prog ca" title={r.warn ?? `${r.done} de ${r.total} ${r.label}`}>
						<span class="l">{r.label}</span>
						<div class="xp"><span style="width: {r.pct}%"></span></div>
						<span class="mono" class:warn={!!r.warn}>{r.done}/{r.total}</span>
					</div>
				{/each}
				{#if conq.note}<p class="mu note">{conq.note}</p>{/if}
				{#if !conq.rows.length && !conq.note}
					<p class="mu">O plugin ainda não leu os combat achievements nem o collection log.</p>
				{/if}
			</div>
		</section>
	</div>

	<section class="card">
		<div class="h">
			<h2>Banco</h2>
			{@render source(bankSrc)}
		</div>
		<div class="b">
			{#if bank}
				{#if bankSrc.stale}
					<p>
						O que aparece aqui é a última leitura. “Tem” e “falta” podem estar errados até a conta
						abrir o banco de novo.
					</p>
				{/if}
				<div class="bctl">
					<label class="field fs">
						<Glyph name="search" size={14} /><input
							type="search"
							placeholder="buscar item"
							aria-label="Buscar item no banco"
							bind:value={bQuery}
						/>
					</label>
					<span class="mu cnt"
						>{bQuery.trim()
							? `${n(bankItems.length)} de ${n(bank.items.length)} itens`
							: `${n(bank.items.length)} itens`}</span
					>
				</div>
				{#if bankShown.length}
					<div class="bankg7">
						{#each bankShown as i (i.id)}{@render slot(i)}{/each}
					</div>
				{:else}
					<p class="mu">{bQuery.trim() ? 'Nenhum item com esse nome.' : 'Banco vazio.'}</p>
				{/if}
				{#if bankShown.length < bankItems.length}
					<button class="lnk more" type="button" onclick={() => (bAll = true)}
						>ver o banco inteiro</button
					>
				{/if}
			{:else}
				<p class="mu">Sem leitura ainda: o plugin lê o banco quando ela o abre no jogo.</p>
			{/if}
		</div>
	</section>

	<p class="foot">{ICON_ATTRIBUTION}.</p>
{/if}

<style>
	.row1 {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.skills {
		flex: 999 1 560px;
	}
	.quests {
		flex: 1 1 320px;
		max-width: 420px;
	}
	@media (max-width: 960px) {
		.quests {
			max-width: none;
		}
	}
	.row2 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
		gap: 20px;
		align-items: start;
	}
	.sk7 .nm {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	/* What went up today (V7Personagem1440). */
	.sk7.up {
		border-color: var(--done);
		background: var(--done-tint);
	}
	.sk7.up .lv7 {
		color: var(--done);
	}
	.subiu {
		margin: 0;
		font-size: 12px;
	}
	.quests .b {
		gap: 10px;
	}
	.fs {
		height: 36px;
	}
	.qr7:disabled {
		cursor: default;
		color: var(--ink);
	}
	.b.tight {
		gap: 0;
	}
	button.more {
		align-self: flex-start;
		min-height: 32px;
		padding: 0;
		border: 0;
		background: none;
		font: inherit;
		font-size: 13px;
		cursor: pointer;
	}
	.eq {
		grid-template-columns: repeat(6, minmax(0, 48px));
	}
	.inv {
		grid-template-columns: repeat(7, minmax(0, 48px));
	}
	.ca {
		grid-template-columns: 64px minmax(0, 1fr) 52px;
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
	}
	.ca .mono {
		font-size: 11px;
		text-align: right;
	}
	.ca .warn {
		color: var(--stale);
	}
	.note {
		font-size: 12px;
	}
	.bctl {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		align-items: center;
	}
	.bctl .fs {
		flex: 0 1 280px;
	}
	.cnt {
		font-size: 12px;
	}
</style>
