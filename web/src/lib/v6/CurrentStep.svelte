<script lang="ts">
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import GoalGrid from './GoalGrid.svelte';
	import { actionsSummary, nextAction } from '#lib/progress/objetivo';
	import { actionsOf, type Action, type PassoObjective } from '#lib/progress/passo';
	import {
		chipsOf,
		isParagraphStep,
		looseLinks,
		markRanges,
		paragraphOf,
		piecesOf,
		stepNotes
	} from '#lib/progress/view';
	import type { BoardStep, ContentStep } from '#lib/progress/types';

	/**
	 * O passo atual da v7 (V7Rota1440 e V7Rota640; sem objetivo detectável, V7Estados1440): a
	 * referência e onde ele está, o título (o TL;DR do passo-parágrafo do BRUHsailer, ou a primeira
	 * frase), os objetivos com quanto falta ("240 / 1.250") agrupados por papel, o passo a passo do
	 * autor com uma caixa em cada ação que cita objetivo, as notas e links do autor, e Feito, Pular,
	 * Desfazer, "também na <rota>" e Fixar outro passo. A 640 o passo a passo abre num botão.
	 */
	let {
		step,
		board,
		where,
		facts,
		others,
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
		/** "2 de 31 no capítulo". */
		where: string;
		/** "GP depois 777.349 · tempo do guia 5 d 0 h", do BRUHsailer; vazio no Oziris. */
		facts: string;
		others: { guide: string; label: string }[];
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
	/* As ações do autor, sem a linha do TL;DR (ela já é o título). Sem TL;DR, a primeira ação é
	 * também o título e fica na lista, como no artboard: é nela que a caixa do que vem agora está. */
	const actions = $derived.by(() => {
		if (!paragraph) return [];
		const all = actionsOf(step.text, passoObjs);
		const t = tldrLine;
		return t ? all.filter((a) => a.start < t.index || a.start >= t.index + t[0].length) : all;
	});
	const headline = $derived(para.tldr ? para.head : (actions[0]?.text ?? para.head));
	const headlineAt = $derived(para.tldr ? para.headAt : (actions[0]?.start ?? para.headAt));
	const nx = $derived(nextAction(actions));
	const notes = $derived(stepNotes(step));
	const wiki = $derived((step.links ?? []).filter((l) => l.type === 'wiki'));
	const ranges = $derived(markRanges(step.text, step.marks));
	const cited = $derived((step.links ?? []).filter((l) => l.type !== 'wiki'));
	const loose = $derived(looseLinks(step.text, cited));
	const piecesAt = (text: string, at: number) => piecesOf(text, at, ranges, '', cited);
	const actionParts = (a: Action) => {
		let at = a.start;
		return a.pieces.map((p) => {
			const sub = piecesAt(p.text, at);
			at += p.text.length;
			return { ...p, sub };
		});
	};
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
	// "também na <rota>": onde a outra rota conclui as mesmas entidades; inicia e conclui da mesma
	// quest no mesmo passo da outra rota viram um link só.
	const cross = $derived.by(() => {
		const completing = chips.filter((c) => c.role === 'completes' || c.role === 'starts');
		return others
			.map((g) => ({
				...g,
				entries: completing
					.flatMap((c) => {
						const occ = step.also?.[c.key]?.find((o) => o.guide === g.guide);
						return occ ? [{ id: occ.step, ref: occ.ref, occ, label: c.label }] : [];
					})
					.filter((e, i, all) => all.findIndex((x) => x.id === e.id) === i)
			}))
			.filter((g) => g.entries.length > 0);
	});
	let open = $state<boolean | null>(null);
	const expanded = $derived(open ?? !narrow);
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs internos vêm de resolve() + âncora; os externos são da wiki -->

{#snippet text(ps: ReturnType<typeof piecesAt>)}{#each ps as x, i (i)}{#if x.href}<a
				class={x.c ? `mk mk-${x.c}` : undefined}
				href={x.href}
				target="_blank"
				rel="noreferrer">{x.t}</a
			>{:else if x.c}<span class="mk mk-{x.c}">{x.t}</span>{:else}{x.t}{/if}{/each}{/snippet}

<section class="now7" class:plain={!chips.length} aria-label="Passo atual">
	<div class="hd">
		<span class="ref">{step.ref}</span><span>passo atual{where ? ` · ${where}` : ''}</span>
		{#if facts}<span class="facts">{facts}</span>{/if}
	</div>
	<h2 class:long={headline.length > 220}>{@render text(piecesAt(headline, headlineAt))}</h2>

	{#if chips.length}
		<GoalGrid {chips} objectives={board?.objectives} />
	{:else}
		<p class="mu none">Nada aqui dá para detectar sozinho. Marque quando terminar.</p>
	{/if}

	{#if actions.length > 1}
		{#if narrow}
			<button class="tog" type="button" aria-expanded={expanded} onclick={() => (open = !expanded)}
				><span class="chev" class:down={expanded}><Glyph name="chevron" size={12} /></span
				>{actionsSummary(actions)}</button
			>
		{/if}
		{#if expanded}
			<ol class="acts7" aria-label="Passo a passo">
				{#each actions as a, i (a.start)}
					<li
						class="a7"
						class:dn={a.state === 'done'}
						class:nx={i === nx}
						class:uk={a.state === 'unknown'}
					>
						{#if a.state === 'plain'}<span class="pl" aria-hidden="true"></span>{:else}<span
								class="bx"
								aria-hidden="true"
								>{#if a.state === 'done'}<Glyph name="check" size={11} />{/if}</span
							>{/if}<span
							>{#each actionParts(a) as p, j (j)}{#if p.objective && p.objective.verdict !== 'met'}<u
										class:uk={p.objective.verdict === 'unknown'}
										title={`${ROLE_WORD[p.objective.role] ?? p.objective.role} ${p.objective.label} · ${VERDICT_WORD[p.objective.verdict]}`}
										>{@render text(p.sub)}</u
									>{:else}{@render text(p.sub)}{/if}{/each}</span
						>
					</li>
				{/each}
			</ol>
		{/if}
	{/if}

	{#if notes.length || wiki.length || loose.length}
		<div class="extra">
			{#each notes as n, i (i)}
				<p class="note" class:note-up={n.kind === 'update'}>
					<span class="lbl">{n.label || 'Nota do autor'}</span>{n.text}
				</p>
			{/each}
			{#if wiki.length || loose.length}
				<p class="links">
					<span class="lbl">Links</span>
					{#each [...wiki, ...loose] as l, i (l.url + i)}<a
							href={l.url}
							target="_blank"
							rel="noreferrer">{l.text?.trim() || l.url}</a
						>{/each}
				</p>
			{/if}
		</div>
	{/if}

	<div class="do7">
		<button class="btn btn-p" class:wide={narrow} type="button" disabled={busy} onclick={ondone}
			><Glyph name="check" size={14} />Feito</button
		>{#if !narrow}<span class="kbd">J</span>{/if}
		<button class="btn" type="button" disabled={busy} onclick={onskip}>Pular</button
		>{#if !narrow}<span class="kbd">K</span>{/if}
		<button class="btn btn-g" type="button" disabled={busy || !canUndo} onclick={onundo}
			>Desfazer</button
		>{#if !narrow}<span class="kbd">U</span>
			<span class="sp"></span>
			{#each cross as c (c.guide)}
				<span class="mu also7"
					>também na {c.label}:
					{#each c.entries as e, i (e.id)}{#if i},
						{/if}<a
							href={`${resolve('/rotas/[guide]/[section]', { guide: e.occ.guide, section: e.occ.section })}#passo-${e.occ.step}`}
							title={e.label}>{e.ref}</a
						>{/each}</span
				>
			{/each}
			<button class="btn btn-g btn-s" type="button" disabled={busy} onclick={onpin}
				>Fixar outro passo</button
			>{/if}
	</div>
</section>

<style>
	/* Cores do autor no texto (BRUHsailer), como na Rota. */
	.mk {
		font-weight: 600;
	}
	.mk-red {
		color: var(--not-started);
	}
	.mk-orange {
		color: var(--stale);
	}
	.mk-green {
		color: var(--done);
	}
	.facts {
		margin-left: auto;
	}
	h2.long {
		font-size: 19px;
		line-height: 1.35;
		max-width: 60ch;
	}
	.none {
		margin: 0;
		padding: 8px 18px 0;
		font-size: 13px;
	}
	.tog {
		margin: 0 12px;
		justify-content: flex-start;
	}
	.chev {
		display: inline-flex;
		transition: transform 0.15s;
	}
	.chev.down {
		transform: rotate(90deg);
	}
	.extra {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 12px 18px 0;
	}
	.note,
	.links {
		margin: 0;
		font-size: 13px;
		color: var(--ink-muted);
		max-width: 80ch;
	}
	.note .lbl,
	.links .lbl {
		margin-right: 6px;
	}
	.note-up {
		padding: 6px 10px;
		border-left: 3px solid var(--stale);
		color: var(--ink);
	}
	.note-up .lbl {
		color: var(--stale);
	}
	.links {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 12px;
		align-items: baseline;
	}
	.also7 {
		font-size: 12px;
	}
	.wide {
		flex: 1 1 120px;
	}
</style>
