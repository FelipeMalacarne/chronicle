<script lang="ts">
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { session, setSummary } from '#lib/progress/session.svelte';
	import { checkboxLine, coverageLine, pct, sectionLabel } from '#lib/progress/view';
	import type { RouteSummary } from '#lib/progress/types';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Rotas (V4Rotas): cada rota é uma ordem e o progresso é um só; a cobertura diz quanto de
	 * cada rota os fatos da conta já respondem. A aba Comparar (V5Comparar) põe duas lado a lado.
	 */
	const guides = createResource(() => progressApi.guides());
	const routes = $derived(
		[...(session.summary?.routes ?? [])].sort((a, b) => Number(b.active) - Number(a.active))
	);
	const info = $derived(new Map((guides.data?.guides ?? []).map((g) => [g.id, g])));
	let busy = $state<string | null>(null);
	let failure = $state<string | null>(null);

	async function activate(guide: string) {
		if (!session.account) return;
		busy = guide;
		failure = null;
		try {
			setSummary(await progressApi.setActiveGuide(session.account, guide));
		} catch (e) {
			failure = e instanceof Error ? e.message : String(e);
		} finally {
			busy = null;
		}
	}

	const AUTHORS: Record<string, string> = {
		'oziris-main': 'OzirisLoL · ironman.guide',
		'bruhsailer-main': 'So Iron BRUH e ParasailerOSRS'
	};
	function author(r: RouteSummary): string {
		return AUTHORS[r.guide] ?? r.source?.author ?? '';
	}
	function cursorHref(r: RouteSummary): string {
		const section = r.cursor_section ?? r.sections[0]?.id;
		if (!section) return resolve('/rotas');
		const base = resolve('/rotas/[guide]/[section]', { guide: r.guide, section });
		return r.cursor ? `${base}#passo-${r.cursor.step_id}` : base;
	}
	const imported = (r: RouteSummary) =>
		r.source?.imported_at?.split('-').reverse().slice(0, 2).join('/');
</script>

<svelte:head>
	<title>Rotas · Chronicle</title>
</svelte:head>

<header class="head">
	<div class="txt">
		<p class="ey">Rotas · {routes.length} importadas</p>
		<h1 class="sr">Cada rota é uma ordem; o progresso é um só</h1>
		<p class="mu">
			O que você já fez na conta vale em todas. Cada rota guarda o próprio cursor e as próprias
			marcas manuais. A cobertura diz quanto de cada rota o seu progresso já responde.
		</p>
	</div>
	<div class="seg" role="tablist">
		<a class="on" href={resolve('/rotas')} role="tab" aria-selected="true" aria-current="page"
			>Lista</a
		>
		<a href={resolve('/compare')} role="tab" aria-selected="false">Comparar</a>
	</div>
</header>

{#if !session.account}
	<div class="band">
		<Glyph name="alert" /><span
			>Sem conta: abra com <code>?account=acc_&lt;rsn&gt;</code> para ver cursores e cobertura.</span
		>
	</div>
{/if}
{#if failure}
	<div class="band"><Glyph name="alert" /><span>A rota ativa não mudou: {failure}</span></div>
{/if}

<div class="cards">
	{#each routes as r (r.guide)}
		{@const g = info.get(r.guide)}
		<article class="panel card-r" class:on={r.active}>
			<div class="plaque">
				<span class="st" class:st-cur={r.active} class:mini-off={!r.active}></span>{r.title}
				<span class="r">{r.active ? 'rota ativa' : 'cursor preservado'}</span>
			</div>
			<div class="body">
				<div class="kvs">
					<div class="kv"><span class="k">Autor</span><span class="v">{author(r)}</span></div>
					<div class="kv">
						<span class="k">Tamanho</span><span class="v"
							>{r.steps} passos · {g?.sections.length ?? r.sections.length} seções · {r.segments} trechos</span
						>
					</div>
					<div class="kv">
						<span class="k">Versão</span><span class="v mono small"
							>{r.version ?? '—'}{imported(r) ? ` · importada ${imported(r)}` : ''}</span
						>
					</div>
				</div>
				<div class="cov">
					<div class="covh">
						<span class="small">Cobertura pelo seu progresso</span>
						{#if r.coverage.objectives > 0}
							<span class="mono su tiny">{coverageLine(r.coverage)}</span>
						{:else}
							<span class="badge badge-stale">sem objetivos até a extração</span>
						{/if}
					</div>
					<div class="xp">
						<span style="width: {pct(r.coverage.met, r.coverage.objectives)}%"></span>
					</div>
					<span class="su tiny">{checkboxLine(r.coverage)}</span>
				</div>
				<div class="secs">
					{#each r.sections as s (s.id)}
						<a
							class="sec"
							href={resolve('/rotas/[guide]/[section]', { guide: r.guide, section: s.id })}
						>
							<span class="i">{sectionLabel(s.id)}</span>
							<span class="t"
								>{s.title}{#if s.track}&nbsp;<span class="badge">track</span>{/if}</span
							>
							{#if r.cursor_section === s.id && r.cursor_ref}<span class="badge badge-gold"
									>cursor {r.cursor_ref}</span
								>{/if}
							<div class="xp">
								<span style="width: {pct(s.counts.done, s.counts.total)}%"></span>
							</div>
							<span class="p">{s.counts.done} / {s.counts.total}</span>
						</a>
					{/each}
				</div>
				<div class="btns">
					{#if r.active}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- cursorHref usa resolve() -->
						<a class="btn btn-p btn-s" href={cursorHref(r)}
							>Abrir no cursor{r.cursor_ref ? ` · ${r.cursor_ref}` : ''}</a
						>
						<a class="btn btn-g btn-s" href={resolve('/')}>Ver no Agora</a>
					{:else}
						<button
							class="btn btn-s"
							type="button"
							disabled={!!busy || !session.account}
							onclick={() => activate(r.guide)}
							>{busy === r.guide ? 'Ativando…' : 'Tornar ativa'}</button
						>
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- cursorHref usa resolve() -->
						<a class="btn btn-g btn-s" href={cursorHref(r)}
							>Abrir no cursor{r.cursor_ref ? ` · ${r.cursor_ref}` : ''}</a
						>
					{/if}
				</div>
			</div>
		</article>
	{:else}
		{#if session.account && session.loading}<p class="su">Carregando as rotas…</p>{/if}
	{/each}
</div>

<p class="su tiny">
	Nunca mesclamos, sobrepomos ou concatenamos rotas. Ids são por rota: o Oziris mostra o id do autor
	(1.1.75); o BRUHsailer mostra a referência do autor (CH1.6) e a posição, nunca o hash interno.
	Atribuição: OzirisLoL / ironman.guide · So Iron BRUH e ParasailerOSRS.
</p>

<style>
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px 24px;
	}
	.seg {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
	}
	.seg > * {
		display: inline-flex;
		align-items: center;
		height: 26px;
		padding: 0 12px;
		border-radius: 3px;
		color: var(--ink-muted);
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		text-decoration: none;
	}
	.seg .on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.txt {
		display: flex;
		flex-direction: column;
		gap: 6px;
		flex: 1 1 420px;
	}
	.txt h1 {
		font-size: 30px;
	}
	.txt p.mu {
		margin: 0;
		max-width: 72ch;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(460px, 100%), 1fr));
		gap: 20px;
		align-items: start;
	}
	.card-r {
		display: flex;
		flex-direction: column;
	}
	.card-r.on {
		box-shadow:
			var(--bevel),
			0 0 0 1px var(--gold-ink);
	}
	.plaque .st {
		width: 14px;
		height: 14px;
		box-shadow: inset 0 0 0 2px var(--frame);
	}
	.mini-off {
		border-color: var(--on-frame-muted);
		box-shadow: none !important;
	}
	.body {
		padding: 16px 18px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.kvs {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
	}
	.kv {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.kv .k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.kv .v {
		font-size: 13px;
	}
	.cov {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.covh {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 8px;
		flex-wrap: wrap;
	}
	.secs {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.sec {
		display: flex;
		align-items: center;
		gap: 10px;
		height: 30px;
		font-size: 12px;
		padding: 0 4px;
		color: var(--ink);
		text-decoration: none;
		border-radius: 4px;
	}
	.sec:hover {
		background: var(--elevated);
	}
	.sec .i {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 70px;
		flex: 0 0 auto;
	}
	.sec .t {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sec .xp {
		width: 90px;
		flex: 0 0 auto;
	}
	.sec .p {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 60px;
		text-align: right;
		flex: 0 0 auto;
	}
	.btns {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.small {
		font-size: 13px;
	}
	.mono.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	p.tiny {
		margin: 0;
	}
	code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}
</style>
