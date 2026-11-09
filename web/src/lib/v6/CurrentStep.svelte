<script lang="ts">
	import { resolve } from '$app/paths';
	import ChipGroups from './ChipGroups.svelte';
	import Glyph from './Glyph.svelte';
	import {
		chipsOf,
		isParagraphStep,
		metaOf,
		paragraphOf,
		statusView,
		stepNotes
	} from '#lib/progress/view';
	import { actionsOf, missingOf, type PassoObjective } from '#lib/progress/passo';
	import type { BoardStep, ContentStep } from '#lib/progress/types';

	/**
	 * O passo atual (V6Agora1440/640 e V6AgoraBruh1440/640): os dois anteriores com quem os fez, o
	 * título (o TL;DR do passo-parágrafo do BRUHsailer, ou a primeira frase), o que falta no passo, os
	 * chips agrupados por papel, a nota do autor, o passo a passo do autor em ações com os objetivos
	 * citados em cada uma, a wiki, "também na <rota>" e Feito/Pular/Desfazer.
	 */
	let {
		step,
		board,
		prev,
		prevWho,
		where,
		others,
		rotaHref,
		busy,
		canUndo,
		narrow,
		ondone,
		onskip,
		onundo,
		onpin
	}: {
		step: ContentStep;
		board: BoardStep | undefined;
		prev: BoardStep[];
		/** "plugin · 12 min" / "você · 9 min" para um passo anterior (dos eventos da rota). */
		prevWho: (id: string) => string;
		where: string;
		others: { guide: string; label: string }[];
		rotaHref: string;
		busy: boolean;
		canUndo: boolean;
		narrow: boolean;
		ondone: () => void;
		onskip: () => void;
		onundo: () => void;
		onpin: () => void;
	} = $props();

	const chips = $derived(chipsOf(step, board));
	const paragraph = $derived(isParagraphStep(step));
	const para = $derived(paragraphOf(step.text));
	/* Passo-parágrafo por dentro: as ações (frases do autor) com os objetivos citados em cada uma,
	 * e o que ainda falta no passo. A linha do TL;DR já é o título, então fica fora das ações. */
	const passoObjs = $derived<PassoObjective[]>(
		chips.map((c) => {
			const ev = step.objectives?.find((o) => o.key === c.key && o.role === c.role)?.evidence;
			const s = ev?.span;
			const onText = s?.length === 2 && (ev?.field ?? '/text') === '/text';
			return {
				key: c.key,
				role: c.role,
				label: c.label,
				verdict: c.verdict,
				span: onText ? ([s[0], s[1]] as [number, number]) : undefined
			};
		})
	);
	const tldrLine = $derived(/^.*TL;?DR.*$/im.exec(step.text));
	const allActions = $derived.by(() => {
		if (!paragraph) return [];
		const all = actionsOf(step.text, passoObjs);
		const t = tldrLine;
		return t ? all.filter((a) => a.start < t.index || a.start >= t.index + t[0].length) : all;
	});
	const missing = $derived(paragraph ? missingOf(passoObjs) : []);
	/* Sem TL;DR, a primeira ação é o título; sai da lista quando não cita objetivo nenhum. */
	const headline = $derived(para.tldr ? para.head : (allActions[0]?.text ?? para.head));
	const actions = $derived(
		!para.tldr && allActions[0]?.state === 'plain' ? allActions.slice(1) : allActions
	);
	const openActions = $derived(actions.filter((a) => a.state === 'open').length);
	const ACT_GLYPH = { done: 'v-met', open: 'v-unmet', unknown: 'v-unk', plain: 'plain' } as const;
	const ROLE_WORD: Record<string, string> = {
		completes: 'conclui',
		starts: 'inicia',
		requires: 'requer',
		uses: 'usa'
	};
	const VERDICT_WORD: Record<string, string> = {
		met: 'feito',
		unmet: 'falta',
		unknown: 'não sabemos'
	};
	const meta = $derived(metaOf(step).filter((m) => m.label !== 'itens'));
	const notes = $derived(stepNotes(step));
	const wiki = $derived((step.links ?? []).filter((l) => l.type === 'wiki'));
	const plaqueRight = $derived(
		[where, ...meta.map((m) => `${m.label} ${m.value}`)].filter(Boolean).join(' · ')
	);
	// "também na <rota>": onde a outra rota conclui as mesmas entidades; as que ela não tem ficam
	// de fora, e uma rota sem nenhuma some. Inicia e conclui da mesma quest no mesmo passo da
	// outra rota viram um link só.
	const cross = $derived.by(() => {
		const completing = chips.filter((c) => c.role === 'completes' || c.role === 'starts');
		return others
			.map((g) => ({
				...g,
				entries: completing
					.flatMap((c) => {
						const occ = step.also?.[c.key]?.find((o) => o.guide === g.guide);
						return occ ? [{ id: `${c.label}|${occ.step}`, label: c.label, occ }] : [];
					})
					.filter((e, i, all) => all.findIndex((x) => x.id === e.id) === i)
			}))
			.filter((g) => g.entries.length > 0);
	});
	let open = $state<boolean | null>(null);
	const expanded = $derived(open ?? !narrow);
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs internos vêm de resolve() + âncora; os externos são da wiki -->

<article class="panel">
	<div class="plaque">
		<span class="st st-cur stc" aria-hidden="true"></span>Passo atual · {step.ref}
		<span class="r" title={plaqueRight}>{plaqueRight}</span>
	</div>
	{#if prev.length}
		<div class="prev">
			{#each prev as s, i (s.id)}
				{@const v = statusView(s.status)}
				<a
					class="row row-done mini"
					class:last={i === prev.length - 1}
					href={`${rotaHref.split('#')[0]}#passo-${s.id}`}
					><span class="st {v.st} tiny"
						>{#if v.done}<Glyph name="check" size={9} />{/if}</span
					><span class="id">{s.ref}</span><span class="tx">{s.title}</span><span class="fr"
						>{prevWho(s.id)}</span
					></a
				>
			{/each}
		</div>
	{/if}
	<div class="body">
		{#if paragraph}
			<h1 class="sr para" class:long={headline.length > 220}>
				{#if para.tldr}<span class="tl">TL;DR</span>{/if}{headline}
			</h1>
			{#if missing.length}
				<p class="falta" title="O que este passo conclui ou inicia e a conta ainda não tem">
					<span class="lbl">Falta</span>{missing.join(' · ')}
				</p>
			{/if}
		{:else}
			<h1 class="sr short">{step.text}</h1>
		{/if}

		{#if chips.length}
			<ChipGroups {chips} />
		{:else}
			<div class="cg">
				<span class="lbl">Conclui</span><span
					class="obj"
					title="Nenhum objetivo detectável neste passo: só você marca como feito."
					><span class="v v-unk" aria-hidden="true">?</span>só você conclui</span
				>
			</div>
		{/if}

		{#each notes as n, i (i)}
			<p class="note" class:note-up={n.kind === 'update'} class:mu={n.kind !== 'update'}>
				<span class="lbl nl">{n.label || 'Nota do autor'}</span>{n.text}
			</p>
		{/each}

		{#if paragraph && actions.length > 0}
			<div class="steps">
				<button
					class="tog"
					type="button"
					aria-expanded={expanded}
					onclick={() => (open = !expanded)}
					><Glyph name="chevron" size={12} /><span class="lbl"
						>Passo a passo · {actions.length} ações{openActions
							? ` · ${openActions} com o que falta`
							: ''}{step.bank_actions?.length ? ' · banco dentro do passo' : ''}</span
					></button
				>
				{#if expanded}
					<ol class="alist box">
						{#each actions as a (a.start)}
							<li class="act act-{a.state}">
								<span class="v {ACT_GLYPH[a.state]}" aria-hidden="true"
									>{#if a.state === 'done'}<Glyph
											name="check"
											size={9}
										/>{:else if a.state === 'unknown'}?{/if}</span
								><span class="tx"
									>{#each a.pieces as p, i (i)}{#if p.objective}<mark
												class="q q-{p.objective.verdict}"
												title={`${ROLE_WORD[p.objective.role] ?? p.objective.role} ${p.objective.label} · ${VERDICT_WORD[p.objective.verdict]}`}
												>{p.text}</mark
											>{:else}{p.text}{/if}{/each}</span
								>
							</li>
						{/each}
					</ol>
				{/if}
			</div>
		{/if}

		{#if wiki.length}
			<div class="line">
				<span class="lbl">wiki</span>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- link externo da wiki -->
				{#each wiki as l (l.url)}<a href={l.url} target="_blank" rel="noreferrer"
						>{l.text ?? l.url}</a
					>{/each}
			</div>
		{/if}
		{#each cross as c (c.guide)}
			<div class="line">
				<span class="lbl">também na {c.label}</span>
				{#each c.entries as e (e.id)}
					<a
						href={`${resolve('/rotas/[guide]/[section]', { guide: e.occ.guide, section: e.occ.section })}#passo-${e.occ.step}`}
						>{e.label} · {e.occ.ref}</a
					>
				{/each}
			</div>
		{/each}

		<div class="actions">
			<button class="btn btn-p wide" type="button" disabled={busy} onclick={ondone}
				><Glyph name="check" size={14} />Feito</button
			>
			<button class="btn mid" type="button" disabled={busy} onclick={onskip}>Pular</button>
			<button class="btn btn-g" type="button" disabled={busy || !canUndo} onclick={onundo}
				>Desfazer</button
			>
			<span class="grow"></span>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href montado com resolve() pela página -->
			<a class="btn btn-g" href={rotaHref}>Abrir na rota</a>
			<button class="btn btn-g" type="button" disabled={busy} onclick={onpin}>Fixar outro</button>
		</div>
		<p class="su keys">
			<span class="kbd">J</span> feito · <span class="kbd">K</span> pular ·
			<span class="kbd">U</span> desfazer
		</p>
	</div>
</article>

<style>
	article {
		display: flex;
		flex-direction: column;
	}
	.stc {
		width: 14px;
		height: 14px;
		box-shadow: inset 0 0 0 2px var(--frame);
	}
	.plaque .r {
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}
	.prev {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 16px 0;
		border-bottom: 1px solid var(--hairline);
	}
	.mini {
		min-height: 28px;
		padding: 0;
		border: 0;
	}
	.mini.last {
		padding-bottom: 6px;
	}
	.mini .tx {
		font-size: 12px;
	}
	.tiny.st {
		width: 14px;
		height: 14px;
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: 16px;
	}
	.short {
		font-size: 32px;
		text-wrap: pretty;
		max-width: 28ch;
	}
	.para {
		font-size: 28px;
		line-height: 1.2;
		text-wrap: pretty;
		max-width: 40ch;
	}
	.para.long {
		font-size: 19px;
		line-height: 1.35;
		max-width: 60ch;
	}
	.tl {
		display: block;
		margin-bottom: 6px;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		color: var(--ink-subtle);
	}
	.cg {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}
	.cg .lbl {
		margin-right: 4px;
	}
	.note {
		margin: 0;
		font-size: 13px;
		max-width: 70ch;
	}
	.nl {
		margin-right: 6px;
	}
	.note-up {
		padding: 6px 10px;
		border-left: 3px solid var(--stale);
		color: var(--ink);
	}
	.note-up .nl {
		color: var(--stale);
	}
	.steps {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.box {
		padding: 10px 14px;
		border-radius: 4px;
		border: 1px solid var(--hairline);
	}
	.falta {
		margin: 0;
		font-size: 14px;
		color: var(--not-started);
	}
	.falta .lbl {
		margin-right: 8px;
		color: var(--not-started);
	}
	.alist {
		list-style: none;
		margin: 0;
		padding: 2px 14px;
		display: flex;
		flex-direction: column;
	}
	.act {
		display: grid;
		grid-template-columns: 14px 1fr;
		gap: 10px;
		align-items: start;
		padding: 7px 0;
		border-bottom: 1px solid var(--hairline);
		font-size: 14px;
		line-height: 1.45;
		max-width: 80ch;
	}
	.act:last-child {
		border-bottom: 0;
	}
	.act .v {
		margin-top: 3px;
	}
	.act-done .tx,
	.act-plain .tx {
		color: var(--ink-muted);
	}
	.act-open .tx {
		color: var(--ink);
		font-weight: 500;
	}
	.v.plain {
		border: 0;
		width: 14px;
		height: 14px;
		background: radial-gradient(circle, var(--ink-subtle) 2px, transparent 2.5px);
	}
	mark.q {
		background: transparent;
		color: inherit;
		box-shadow: none;
		text-decoration-line: underline;
		text-decoration-thickness: 2px;
		text-underline-offset: 3px;
	}
	mark.q-met {
		text-decoration-color: var(--done);
	}
	mark.q-unmet {
		text-decoration-color: var(--not-started);
		color: var(--ink);
	}
	mark.q-unknown {
		text-decoration-style: dotted;
		text-decoration-color: var(--ink-subtle);
	}
	.line {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		font-size: 12px;
		align-items: center;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding-top: 2px;
	}
	.wide {
		min-width: 104px;
	}
	.mid {
		min-width: 88px;
	}
	.grow {
		flex: 1;
	}
	.keys {
		margin: -6px 0 0;
		font-size: 11px;
	}
	@media (max-width: 900px) {
		.short {
			font-size: 28px;
		}
		.para {
			font-size: 22px;
		}
	}
</style>
