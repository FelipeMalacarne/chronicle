<script lang="ts">
	import { resolve } from '$app/paths';
	import { api } from '#lib/api/client';
	import { createResource } from '#lib/api/resource.svelte';
	import { skillMilestones } from '#lib/api/rules';
	import type { Now, Section, SectionSummary, Step } from '#lib/api/types';
	import Icon from '#lib/components/Icon.svelte';
	import StepRow from '#lib/components/StepRow.svelte';
	import ErrorState from '#lib/components/ErrorState.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';

	const nowRes = createResource<Now>(() => api.now());
	const guideRes = createResource<SectionSummary[]>(() => api.guide());

	// Pular é só local até o banco existir: avança a fila na tela.
	let skipped = $state<string[]>([]);
	const queue = $derived.by<Step[]>(() => {
		const n = nowRes.data;
		if (!n) return [];
		return [n.current, ...n.next].filter((s) => !skipped.includes(s.id));
	});
	const current = $derived(queue[0] ?? null);
	const next = $derived(queue.slice(1, 6));

	const sectionId = $derived(current ? current.id.split('.').slice(0, -1).join('.') : '');
	const section = $derived(guideRes.data?.find((s) => s.id === sectionId) ?? null);

	// Índice do passo dentro da seção (precisa da lista da seção; refaz só quando o slug muda).
	const sectionRes = createResource<Section | null>(() => {
		const slug = section?.slug;
		return slug ? api.section(slug) : Promise.resolve(null);
	});
	const indexInSection = $derived.by(() => {
		const steps = sectionRes.data?.steps;
		const id = current?.id;
		if (!steps || !id) return null;
		const i = steps.findIndex((x) => x.id === id);
		return i >= 0 ? i + 1 : null;
	});

	const eyebrow = $derived.by(() => {
		const parts = ['Agora'];
		if (sectionId) parts.push(`seção ${sectionId}`);
		if (indexInSection && section) parts.push(`passo ${indexInSection} de ${section.steps}`);
		return parts.join(' · ');
	});

	// Marcos de skill: varre o resto da seção a partir do passo atual (ou só a fila, se a seção não carregou).
	const milestones = $derived(
		skillMilestones(
			sectionRes.data ? sectionRes.data.steps.slice((indexInSection ?? 1) - 1) : queue
		)
	);

	// Anotar: rascunho por passo, só neste navegador (localStorage).
	let noting = $state(false);
	let noteText = $state('');
	function noteKey(id: string) {
		return `ironman-planner:note:${id}`;
	}
	$effect(() => {
		const id = current?.id;
		noting = false;
		if (!id) return;
		try {
			noteText = localStorage.getItem(noteKey(id)) ?? '';
		} catch {
			noteText = '';
		}
	});
	function saveNote() {
		if (!current) return;
		try {
			if (noteText.trim()) localStorage.setItem(noteKey(current.id), noteText);
			else localStorage.removeItem(noteKey(current.id));
		} catch {
			/* armazenamento indisponível: rascunho fica só em memória */
		}
		noting = false;
	}
	function skip() {
		if (current) skipped = [...skipped, current.id];
	}
	function onKey(e: KeyboardEvent) {
		const t = e.target as HTMLElement | null;
		if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
		if (e.key === 'k' || e.key === 'K') {
			e.preventDefault();
			skip();
		}
		// J (feito) fica reservado: precisa do banco.
	}

	const sources = [
		{ name: 'Plugin RuneLite', detail: 'quests, diaries, banco', state: 'off' },
		{ name: 'Hiscores', detail: 'skills, kc', state: 'off' },
		{ name: 'OSRS Wiki', detail: 'requisitos, dados estruturados', state: 'off' }
	] as const;
</script>

<svelte:window onkeydown={onKey} />

<svelte:head>
	<title>Agora · Ironman Planner</title>
</svelte:head>

<div class="layout">
	<section class="col-main" aria-labelledby="agora-title">
		{#if nowRes.error}
			<ErrorState error={nowRes.error} retry={nowRes.reload} />
		{:else if nowRes.loading || !current}
			<div class="panel">
				<Skeleton lines={4} height={18} />
			</div>
		{:else}
			<article class="panel now">
				<p class="eyebrow">{eyebrow}</p>
				<h1 id="agora-title" class="display title">{current.text}</h1>

				<div class="meta">
					{#if current.location}
						<span class="chip"><Icon name="pin" size={14} /> {current.location}</span>
					{/if}
					{#if current.layer === 'enhanced'}
						<span class="badge badge-accent">enhanced</span>
					{/if}
					{#if current.quest}
						<span class="chip">{current.quest.name} · {current.quest.status}</span>
					{/if}
				</div>

				{#if current.items?.length}
					<div class="block">
						<p class="eyebrow">Levar</p>
						<ul class="chips" aria-label="Itens para levar">
							{#each current.items as it (it)}
								<li class="chip chip-mono">{it}</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if current.note}
					<p class="note muted">{current.note}</p>
				{/if}
				{#if current.update_note}
					<p class="note muted">
						<span class="badge badge-warning">atualização</span>
						{current.update_note}
					</p>
				{/if}

				{#if noting}
					<form class="noteform" onsubmit={(e) => (e.preventDefault(), saveNote())}>
						<label class="label" for="nota">Anotação (fica só neste navegador)</label>
						<textarea id="nota" class="input area" bind:value={noteText} rows="3"></textarea>
						<div class="actions">
							<button class="btn btn-accent btn-compact" type="submit">Salvar</button>
							<button
								class="btn btn-ghost btn-compact"
								type="button"
								onclick={() => (noting = false)}>Cancelar</button
							>
						</div>
					</form>
				{:else if noteText}
					<p class="note own"><span class="eyebrow">Sua nota</span> {noteText}</p>
				{/if}

				<div class="actions">
					<span class="tip" data-tip="em breve: precisa do banco">
						<button class="btn btn-accent" type="button" disabled aria-describedby="feito-tip">
							<Icon name="check" size={16} /> Feito
						</button>
						<span id="feito-tip" role="tooltip" class="visually-hidden"
							>em breve: precisa do banco</span
						>
					</span>
					<button class="btn" type="button" onclick={skip} disabled={queue.length < 2}>
						<Icon name="skip" size={16} /> Pular
					</button>
					<button
						class="btn btn-ghost"
						type="button"
						aria-pressed={noting}
						onclick={() => (noting = !noting)}
					>
						<Icon name="note" size={16} /> Anotar
					</button>
					<span class="hints caption">
						<span class="kbd">J</span> feito · <span class="kbd">K</span> pular
					</span>
				</div>
			</article>

			<section class="list" aria-labelledby="seguida">
				<p id="seguida" class="eyebrow">Em seguida</p>
				{#if next.length === 0}
					<p class="muted">Fim da fila carregada.</p>
				{:else}
					<div class="rows">
						{#each next as s (s.id)}
							<StepRow
								step={s}
								href={section
									? `${resolve('/guide/[section]', { section: section.slug })}#step-${s.id}`
									: undefined}
							/>
						{/each}
					</div>
				{/if}
				{#if nowRes.data}
					<p class="caption progress">
						{nowRes.data.progress.done} de {nowRes.data.progress.total} passos concluídos
					</p>
				{/if}
			</section>
		{/if}
	</section>

	<aside class="col-aside">
		<section class="card" aria-labelledby="fontes">
			<p id="fontes" class="eyebrow">Fontes</p>
			<ul class="sources">
				{#each sources as src (src.name)}
					<li>
						<span class="sdot {src.state}" aria-hidden="true"></span>
						<span class="sname">{src.name}</span>
						<span class="caption sdetail">não conectado</span>
					</li>
				{/each}
			</ul>
		</section>

		<section class="card" aria-labelledby="marcos">
			<p id="marcos" class="eyebrow">Próximos marcos de skill</p>
			{#if milestones.length === 0}
				<p class="caption">Nenhuma regra de skill nos próximos passos.</p>
			{:else}
				<ul class="milestones">
					{#each milestones as m (m.skill)}
						<li>
							<span class="mskill">{m.skill}</span>
							<span class="data mlevel">≥ {m.level}</span>
							<span class="caption mstep" title={m.stepText}>{m.stepId}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	</aside>
</div>

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 300px;
		gap: var(--sp-6);
		align-items: start;
	}
	.col-aside {
		display: grid;
		gap: var(--sp-4);
		position: sticky;
		top: calc(var(--topbar-h) + var(--sp-6));
	}
	.now {
		display: grid;
		gap: var(--sp-4);
	}
	.title {
		line-height: 1.25;
	}
	.meta,
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.block {
		display: grid;
		gap: var(--sp-2);
	}
	.note {
		max-width: 70ch;
	}
	.note .badge {
		vertical-align: 1px;
		margin-right: 4px;
	}
	.own {
		padding: 10px 12px;
		border-radius: var(--r-chip);
		background: var(--bg);
	}
	.own .eyebrow {
		display: block;
		margin-bottom: 4px;
	}
	.noteform {
		display: grid;
		gap: var(--sp-2);
	}
	.area {
		height: auto;
		padding: 8px 12px;
		resize: vertical;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-2);
	}
	.hints {
		margin-left: auto;
	}
	.tip {
		position: relative;
		display: inline-flex;
	}
	.tip::after {
		content: attr(data-tip);
		position: absolute;
		left: 50%;
		bottom: calc(100% + 8px);
		transform: translateX(-50%);
		padding: 5px 9px;
		border-radius: var(--r-chip);
		background: var(--elevated);
		border: 1px solid var(--border);
		color: var(--text);
		font-size: var(--fs-caption);
		white-space: nowrap;
		opacity: 0;
		pointer-events: none;
		transition: opacity 120ms ease;
	}
	.tip:hover::after,
	.tip:focus-within::after {
		opacity: 1;
	}
	.list {
		margin-top: var(--sp-6);
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--sp-2);
	}
	.rows {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 2px;
	}
	.progress {
		padding-left: var(--sp-4);
	}
	.sources,
	.milestones {
		list-style: none;
		margin: var(--sp-3) 0 0;
		padding: 0;
		display: grid;
		gap: var(--sp-3);
	}
	.sources li,
	.milestones li {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}
	.sdot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--subtle);
	}
	.sname,
	.mskill {
		flex: 1;
		text-transform: capitalize;
	}
	.sdetail {
		white-space: nowrap;
	}
	.mlevel {
		color: var(--accent);
	}
	.mstep {
		min-width: 46px;
		text-align: right;
	}
	@media (max-width: 719px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
		.col-aside {
			position: static;
		}
		.title {
			font-size: var(--fs-heading);
		}
		.hints {
			display: none;
		}
	}
</style>
