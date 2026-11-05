<script lang="ts">
	import Glyph from './Glyph.svelte';
	import ObjectiveChip from './ObjectiveChip.svelte';
	import {
		alsoIn,
		chipsOf,
		isParagraphStep,
		metaOf,
		paragraphOf,
		statusView,
		stepNotes,
		unknownHint
	} from '#lib/progress/view';
	import type { BoardStep, ContentStep } from '#lib/progress/types';

	/**
	 * O passo atual como card (decisão 4 do v4): anatomia curta para o Oziris (V4AgoraOziris) e
	 * passo-parágrafo com TL;DR e passo a passo recolhível para o BRUHsailer (V4AgoraBruh).
	 */
	let {
		step,
		board,
		prev,
		where,
		others,
		rotaHref,
		busy,
		canUndo,
		ondone,
		onskip,
		onundo,
		onpin
	}: {
		step: ContentStep;
		board: BoardStep | undefined;
		prev: BoardStep[];
		where: string;
		others: { guide: string; label: string }[];
		rotaHref: string;
		busy: boolean;
		canUndo: boolean;
		ondone: () => void;
		onskip: () => void;
		onundo: () => void;
		onpin: () => void;
	} = $props();

	const chips = $derived(chipsOf(step, board));
	const completing = $derived(chips.filter((c) => c.role === 'completes' || c.role === 'starts'));
	const metCount = $derived(completing.filter((c) => c.verdict === 'met').length);
	const hint = $derived(unknownHint(chips));
	const paragraph = $derived(isParagraphStep(step));
	const para = $derived(paragraphOf(step.text));
	const meta = $derived(metaOf(step));
	const notes = $derived(stepNotes(step));
	const wiki = $derived((step.links ?? []).filter((l) => l.type === 'wiki'));
	const cross = $derived(alsoIn(step, chips, others));
	let open = $state(false);
</script>

<article class="panel">
	<div class="plaque">
		<span class="st st-cur cur"></span>Passo atual · {step.ref}
		<span class="r">{where}</span>
	</div>
	<div class="body">
		{#if prev.length}
			<div class="prev">
				{#each prev as s (s.id)}
					{@const v = statusView(s.status)}
					<div class="row row-done mini">
						<span class="st {v.st} tiny"></span><span class="id">{s.ref}</span><span class="tx"
							>{s.title}</span
						>
						{#if v.label}<span class="badge">{v.label}</span>{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if paragraph}
			{#if meta.length}
				<div class="metas">
					{#each meta as m (m.label)}<span class="meta">{m.label}: <b>{m.value}</b></span>{/each}
				</div>
			{/if}
			<h1 class="sr para" class:long={para.head.length > 220}>
				{#if para.tldr}<span class="tl">TL;DR</span>{/if}{para.head}
			</h1>
		{:else}
			<h1 class="sr short">{step.text}</h1>
		{/if}
		{#each notes as n, i (i)}
			<p class="note" class:note-up={n.kind === 'update'} class:mu={n.kind !== 'update'}>
				{#if n.label}<span class="nl">{n.label}</span>{/if}{n.text}
			</p>
		{/each}

		<div class="objs">
			{#if paragraph && chips.length}
				<div class="objh">
					<p class="ey">Objetivos · {metCount} de {completing.length} concluídos</p>
					<div class="xp bar">
						<span style="width: {completing.length ? (metCount / completing.length) * 100 : 0}%"
						></span>
					</div>
				</div>
			{/if}
			<div class="chips">
				{#if completing.length === 0}
					<span class="obj obj-unk"
						><span class="v v-unk">?</span>sem objetivo detectável<span class="k"
							>só você conclui</span
						></span
					>
				{/if}
				{#each chips as c (c.role + c.key)}<ObjectiveChip chip={c} />{/each}
			</div>
			{#if hint}<span class="su tiny">{hint}</span>{/if}
		</div>

		{#if paragraph && para.rest.length}
			<div class="steps">
				<button class="tog" type="button" aria-expanded={open} onclick={() => (open = !open)}>
					<Glyph name="chevron" size={12} />Passo a passo do autor · {para.bullets}
					{para.bullets === 1 ? 'bullet' : 'bullets'} · {para.chars.toLocaleString('pt-BR')} caracteres
				</button>
				{#if open}
					<div class="prose box">
						{#each para.rest as p, i (i)}<p>{p}</p>{/each}
					</div>
				{/if}
			</div>
		{/if}

		{#if wiki.length}
			<p class="su line">
				<span class="ey mini-ey">wiki</span>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- link externo da wiki -->
				{#each wiki as l (l.url)}<a href={l.url} target="_blank" rel="noreferrer"
						>{l.text ?? l.url}</a
					>{/each}
			</p>
		{/if}
		{#each cross as c (c.guide)}
			<p class="su line">
				<span class="ey mini-ey">também na rota {c.label}</span>
				{#each c.entries as e, i (i)}<span>{e.label} · {e.where}</span>{/each}
			</p>
		{/each}

		<div class="actions">
			<button class="btn btn-p" type="button" disabled={busy} onclick={ondone}
				><Glyph name="check" size={14} />Feito</button
			>
			<button class="btn" type="button" disabled={busy} onclick={onskip}>Pular</button>
			<button class="btn btn-g" type="button" disabled={busy || !canUndo} onclick={onundo}
				>Desfazer</button
			>
			<button class="btn btn-g" type="button" disabled={busy} onclick={onpin}>Fixar outro</button>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href montado com resolve() pela página -->
			<a class="btn btn-g" href={rotaHref}>Abrir na rota</a>
			<span class="su keys"
				><span class="kbd">J</span>feito <span class="kbd">K</span>pular
				<span class="kbd">U</span>desfazer</span
			>
		</div>
	</div>
</article>

<style>
	article {
		display: flex;
		flex-direction: column;
	}
	.cur {
		width: 14px;
		height: 14px;
		box-shadow: inset 0 0 0 2px var(--frame);
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: 16px 22px 18px;
	}
	.prev {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.mini {
		min-height: 26px;
		padding: 0;
	}
	.tiny.st {
		width: 14px;
		height: 14px;
	}
	.short {
		font-size: 32px;
		text-wrap: pretty;
		max-width: 28ch;
	}
	.para {
		font-size: 26px;
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
			500 12px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		color: var(--ink-subtle);
	}
	.note {
		margin: 0;
		max-width: 64ch;
	}
	.note-up {
		padding: 6px 10px;
		border-left: 3px solid var(--stale);
		background: var(--stale-tint);
		color: var(--ink);
	}
	.nl {
		margin-right: 8px;
		font:
			600 11px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--stale);
	}
	.metas,
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 8px;
		align-items: center;
	}
	.objs,
	.steps {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.objh {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.bar {
		flex: 1;
		max-width: 160px;
		margin-left: auto;
	}
	.box {
		padding: 10px 14px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--hairline);
	}
	.line {
		margin: 0;
		font-size: 12px;
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		align-items: center;
	}
	.mini-ey {
		font-size: 10px;
	}
	.tiny {
		font-size: 11px;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding-top: 2px;
	}
	.keys {
		margin-left: auto;
		display: inline-flex;
		gap: 6px;
		align-items: center;
		font-size: 11px;
	}
</style>
