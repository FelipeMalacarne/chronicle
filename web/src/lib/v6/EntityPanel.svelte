<script lang="ts">
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { progressApi } from '#lib/api/client';
	import { invalidate } from '#lib/api/resource.svelte';
	import { createResource } from '#lib/api/resource.svelte';
	import { closeEntity, entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { narrow } from '#lib/progress/narrow.svelte';
	import { session } from '#lib/progress/session.svelte';
	import { entityVerdict, KIND_LABEL, mainKey, routeLabel, stateLabel } from '#lib/progress/view';
	import type { Goal, Verdict, WorldLink } from '#lib/progress/types';

	/**
	 * Painel de entidade (V6Entidade; comportamento em V6Agora1440/640 e V6Mundo1440): estado com
	 * a fonte, onde cada rota faz isso, requer → destrava, "Virar meta" e "Focar aqui" (o grafo). Lateral no desktop,
	 * folha de baixo na janela estreita. `inline` desenha o mesmo painel dentro da página (Mundo).
	 */
	let { inline = false, refOverride }: { inline?: boolean; refOverride?: string | null } = $props();

	const ref = $derived(inline ? (refOverride ?? null) : entityPanel.ref);
	const account = $derived(session.account);
	const entity = createResource(
		() => (account && ref ? progressApi.entity(account, ref) : Promise.resolve(null)),
		{ tags: ['progress'] }
	);
	const goals = createResource(
		() => (account && ref ? progressApi.goals(account) : Promise.resolve({ goals: [] as Goal[] })),
		{ tags: ['progress'] }
	);
	const e = $derived(entity.data && entity.data.ref === ref ? entity.data : null);
	const keys = $derived(e?.objectives.map((o) => o.key) ?? []);
	const goalKey = $derived(mainKey(keys));
	const goal = $derived(
		goals.data?.goals.find((g) => g.ref === ref && g.status === 'active') ?? null
	);
	const agg = $derived(e ? entityVerdict(e.objectives) : 'unknown');
	const icon = $derived(ref ? iconFor(ref) : undefined);
	const labelOf = (guide: string) =>
		routeLabel(guide, session.summary?.routes.find((r) => r.guide === guide)?.title ?? guide);
	const cursorOf = (guide: string) =>
		session.summary?.routes.find((r) => r.guide === guide)?.cursor?.step_id;
	const routeCount = $derived(new Set(e?.routes.map((r) => r.guide) ?? []).size);

	const V_CLS = { met: 'v-met', prog: 'v-prog', unmet: 'v-unmet', unknown: 'v-unk' } as const;
	const ROLE: Record<string, string> = {
		completes: 'conclui',
		starts: 'inicia',
		requires: 'requer',
		uses: 'usa'
	};

	/** A frase do estado: o que sabemos e o que falta para saber. */
	const stateText = $derived.by(() => {
		if (!e) return '';
		const parts = e.objectives.map((o) => `${stateLabel(o.key)}: ${VERDICT[o.verdict]}`);
		switch (agg) {
			case 'met':
				return 'Feito.';
			case 'prog':
				return `Em progresso. ${parts.join(' · ')}`;
			case 'unmet':
				return `Não atende. ${parts.join(' · ')}`;
			default:
				return `Não sabemos. ${SOURCE_HINT[e.kind] ?? 'Nenhuma fonte lê isto ainda.'}`;
		}
	});
	const VERDICT: Record<Verdict, string> = {
		met: 'feito',
		unmet: 'não atende',
		unknown: 'não sabemos'
	};
	const SOURCE_HINT: Record<string, string> = {
		quest: 'O plugin ainda não leu esta quest.',
		diary: 'O plugin ainda não leu este diary.',
		item: 'Nenhum container lido ainda.',
		skill: 'Sem leitura do hiscores nem do plugin.',
		unlock: 'Sem fonte para este unlock.',
		location: 'Local: nunca é verificado.'
	};
	const sourcesLine = $derived.by(() => {
		const s = session.summary?.sources ?? [];
		const want = e?.kind === 'skill' || e?.kind === 'boss' ? ['hiscores', 'plugin'] : ['plugin'];
		const seen = s.filter((x) => want.includes(x.source));
		return seen.length
			? `Última leitura: ${seen.map((x) => `${x.source} ${new Date(x.last_observed_at).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' })}`).join(' · ')}`
			: 'Nenhuma leitura ainda: abra o jogo com o plugin para o estado chegar sozinho.';
	});

	let busy = $state(false);
	let failure = $state<string | null>(null);
	async function toggleGoal() {
		if (!account || busy) return;
		busy = true;
		failure = null;
		try {
			if (goal) await progressApi.removeGoal(account, goal.id);
			else if (goalKey) await progressApi.addGoal(account, goalKey);
			goals.reload();
			invalidate('progress');
		} catch (err) {
			failure = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}

	const sheet = $derived(narrow.value);
	function key(ev: KeyboardEvent) {
		if (!inline && ref && ev.key === 'Escape') closeEntity();
	}
	const stepHref = (r: { guide: string; section: string; step: string }) =>
		`${resolve('/rotas/[guide]/[section]', { guide: r.guide, section: r.section })}#passo-${r.step}`;
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs internos vêm de resolve() + âncora; os externos são da wiki -->

<svelte:window onkeydown={key} />

{#snippet link(l: WorldLink)}
	{@const c = V_CLS[l.verdict]}
	{#if l.ref}
		<button class="n" type="button" onclick={() => openEntity(l.ref ?? '')} title={l.key}
			><span class="v {c}"
				>{#if l.verdict === 'met'}<Glyph
						name="check"
						size={9}
					/>{:else if l.verdict === 'unknown'}?{/if}</span
			>{l.label}{#if l.key.includes('>=')}&nbsp;{l.key.split('>=')[1]}{/if}{#if l.boostable}<span
					class="su tiny">&nbsp;boost</span
				>{/if}</button
		>
	{/if}
{/snippet}

{#snippet body()}
	<div class="hd">
		<span class="ico"
			>{#if icon}<img src={icon} alt="" />{/if}</span
		>
		<div class="ttl">
			<span class="sr nm">{e?.name ?? ref}</span>
			<span class="su sm"
				>{e ? (KIND_LABEL[e.kind] ?? e.kind) : ''}{#if e?.wiki}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + âncora, ou link externo da wiki -->
					· <a href={e.wiki} target="_blank" rel="noreferrer">wiki</a>{/if}</span
			>
		</div>
		{#if !inline}
			<button class="x" type="button" onclick={closeEntity} aria-label="Fechar painel"
				><Glyph name="x" size={18} /></button
			>
		{/if}
	</div>
	{#if entity.error && !e}
		<div class="bd"><p class="su sm">Não deu para carregar: {entity.error.message}</p></div>
	{:else if !e}
		<div class="bd"><p class="su sm" aria-busy="true">Carregando…</p></div>
	{:else}
		<div class="bd">
			<div class="kv">
				<p class="lbl">Estado</p>
				<div class="st-line">
					<span class="v {V_CLS[agg]} big" aria-hidden="true"
						>{#if agg === 'met'}<Glyph
								name="check"
								size={12}
							/>{:else if agg === 'unknown'}?{/if}</span
					><span>{stateText}</span>
				</div>
				<span class="su sm">{sourcesLine}</span>
			</div>
			<div class="kv">
				<p class="lbl">Onde cada rota faz isso</p>
				{#each e.routes as r (r.guide + r.step + r.role)}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + âncora, ou link externo da wiki -->
					<a class="rtl" href={stepHref(r)} onclick={() => !inline && closeEntity()}
						><span class="g">{labelOf(r.guide)}</span><span class="i">{r.ref}</span><span class="t"
							>{r.title ?? ''}</span
						><span class="r"
							>{ROLE[r.role] ?? r.role}{cursorOf(r.guide) === r.step
								? ' · você está aqui'
								: ''}</span
						></a
					>
				{:else}
					<span class="su sm">Nenhuma rota faz isso.</span>
				{/each}
				{#if routeCount > 1}
					<span class="su sm"
						>O progresso é um só: concluir aqui marca o objetivo para as duas rotas.</span
					>
				{/if}
			</div>
			{#if e.requires.length || e.unlocks.length || e.combat_level}
				<div class="kv">
					<p class="lbl">Requer → destrava</p>
					<div class="req">
						<div class="c">
							{#each e.requires as l (l.key)}{@render link(l)}{/each}
							{#if e.combat_level}<span class="n"
									><span class="v v-unk">?</span>Combat {e.combat_level}</span
								>{/if}
							{#if !e.requires.length && !e.combat_level}<span class="su sm">nada</span>{/if}
						</div>
						<span class="su arrow">→</span>
						<div class="c">
							{#each e.unlocks as l (l.key)}{@render link(l)}{/each}
							{#if !e.unlocks.length}<span class="su sm">nada no catálogo</span>{/if}
						</div>
					</div>
				</div>
			{/if}
			{#if failure}<p class="red sm">A meta não mudou: {failure}</p>{/if}
			<div class="acts">
				{#if goalKey}
					<button
						class="btn"
						class:btn-p={!goal}
						type="button"
						disabled={busy}
						aria-pressed={!!goal}
						onclick={toggleGoal}>{goal ? 'Meta fixada ✓ · tirar' : 'Virar meta'}</button
					>
				{/if}
				{#if goalKey}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query do foco -->
					<a
						class="btn btn-g"
						href={`${resolve('/mundo/grafo')}?focus=${encodeURIComponent(goalKey)}`}
						onclick={() => !inline && closeEntity()}>Focar aqui</a
					>
				{/if}
				{#if !inline}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da entidade -->
					<a
						class="btn btn-g"
						href={`${resolve('/mundo')}?e=${encodeURIComponent(e.ref)}`}
						onclick={closeEntity}>Abrir no Mundo</a
					>
				{/if}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + âncora, ou link externo da wiki -->
				{#if e.wiki}<a class="btn btn-g" href={e.wiki} target="_blank" rel="noreferrer">wiki</a
					>{/if}
			</div>
		</div>
	{/if}
{/snippet}

{#if inline}
	<div class="dr inl">{@render body()}</div>
{:else if ref}
	<button class="scrim" type="button" onclick={closeEntity} aria-label="Fechar painel"></button>
	<div
		class="drw dr"
		class:drw-side={!sheet}
		class:drw-sheet={sheet}
		role="dialog"
		aria-label={e?.name ?? ref}
	>
		{#if sheet}<div class="grab" aria-hidden="true"></div>{/if}
		{@render body()}
	</div>
{/if}

<style>
	.kv {
		gap: 6px;
	}
	.drw-sheet {
		border-radius: 10px 10px 0 0;
	}
	.drw {
		overflow: auto;
	}
	.inl {
		width: auto;
		min-height: 0;
		border-radius: 6px;
		box-shadow: var(--bevel);
		background: var(--surface);
		border: 1px solid var(--border);
	}
	.ttl {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.nm {
		font-size: 20px;
	}
	.sm {
		font-size: 12px;
		margin: 0;
	}
	.tiny {
		font-size: 11px;
	}
	.big {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		border: 1.5px solid var(--border);
		flex: 0 0 auto;
		font:
			600 12px Inter,
			sans-serif;
	}
	.n .v {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		border: 1.5px solid var(--border);
		font-size: 9px;
		flex: 0 0 auto;
	}
	button.n {
		border: 0;
		background: transparent;
		padding: 0;
		cursor: pointer;
		color: var(--ink);
		font: inherit;
		font-size: 13px;
		text-align: left;
	}
	button.n:hover {
		text-decoration: underline;
	}
	.arrow {
		padding-top: 6px;
	}
	.rtl {
		text-decoration: none;
		color: var(--ink);
	}
	.rtl:hover {
		background: var(--elevated);
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding-top: 8px;
		border-top: 1px solid var(--hairline);
	}
	.acts .btn {
		height: 40px;
	}
	.x {
		margin-left: auto;
		width: 44px;
		height: 44px;
		border: 0;
		background: transparent;
		color: var(--ink-subtle);
		cursor: pointer;
		border-radius: 4px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
</style>
