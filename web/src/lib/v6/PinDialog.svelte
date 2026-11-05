<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { pendingBefore, statusView } from '#lib/progress/view';
	import type { BoardStep } from '#lib/progress/types';

	/**
	 * "Fixar outro" (V6AgoraEstados1440): painel lateral com a busca, os pendentes atrás do cursor
	 * e os de à frente. O pin vale só nesta rota e cai quando o passo concluir. "Marcar os
	 * anteriores como feitos" (lote com desfazer) continua como ação secundária do rodapé.
	 */
	let {
		guideLabel,
		steps,
		cursorId,
		onpin,
		onclose
	}: {
		guideLabel: string;
		steps: BoardStep[];
		cursorId: string | undefined;
		/** before: marcar como feitos os pendentes anteriores em vez de fixar. */
		onpin: (stepId: string, before: boolean) => void;
		onclose: () => void;
	} = $props();

	let query = $state('');
	let selected = $state(0);
	let input: HTMLInputElement | undefined = $state();

	const cursorIndex = $derived(steps.findIndex((s) => s.id === cursorId));
	const matches = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return steps
			.map((s, i) => ({ s, i }))
			.filter(({ s }) => !statusView(s.status).done && s.id !== cursorId)
			.filter(
				({ s }) => !q || s.ref.toLowerCase().includes(q) || s.title.toLowerCase().includes(q)
			);
	});
	const behind = $derived(matches.filter((r) => cursorIndex >= 0 && r.i < cursorIndex).slice(-4));
	const ahead = $derived(
		matches.filter((r) => cursorIndex < 0 || r.i > cursorIndex).slice(0, q() ? 8 : 6)
	);
	function q() {
		return query.trim() !== '';
	}
	const flat = $derived([...behind, ...ahead]);
	const pick = $derived(flat[selected] ?? null);
	const before = $derived(pick ? pendingBefore(steps, pick.s.id).length : 0);

	$effect(() => {
		input?.focus();
	});
	$effect(() => {
		if (selected >= flat.length) selected = Math.max(0, flat.length - 1);
	});

	function key(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
		else if (e.key === 'ArrowDown') {
			selected = Math.min(flat.length - 1, selected + 1);
			e.preventDefault();
		} else if (e.key === 'ArrowUp') {
			selected = Math.max(0, selected - 1);
			e.preventDefault();
		} else if (e.key === 'Enter' && pick) onpin(pick.s.id, false);
	}

	/** Realce do termo buscado no texto do passo. */
	function parts(text: string): { t: string; m: boolean }[] {
		const term = query.trim();
		if (!term) return [{ t: text, m: false }];
		const i = text.toLowerCase().indexOf(term.toLowerCase());
		if (i < 0) return [{ t: text, m: false }];
		return [
			{ t: text.slice(0, i), m: false },
			{ t: text.slice(i, i + term.length), m: true },
			{ t: text.slice(i + term.length), m: false }
		];
	}
</script>

{#snippet row(r: { s: BoardStep; i: number }, n: number)}
	<button
		class="it"
		class:it-on={n === selected}
		type="button"
		aria-current={n === selected ? 'true' : undefined}
		onclick={() => onpin(r.s.id, false)}
		onmouseenter={() => (selected = n)}
	>
		<span class="k">{r.s.ref}</span><span class="tx"
			>{#each parts(r.s.title) as p, j (j)}{#if p.m}<mark>{p.t}</mark>{:else}{p.t}{/if}{/each}</span
		>
		<span class="m">pos. {r.i + 1}</span>
	</button>
{/snippet}

<button class="scrim" type="button" onclick={onclose} aria-label="Fechar"></button>
<div
	class="drw drw-side pin"
	role="dialog"
	tabindex="-1"
	aria-label="Fixar outro passo"
	onkeydown={key}
>
	<div class="hd">
		<div class="ttl">
			<span class="sr">Fixar outro passo</span><span class="su sm"
				>{guideLabel} · o pin vale só nesta rota e cai quando o passo concluir</span
			>
		</div>
		<button class="x" type="button" onclick={onclose} aria-label="Fechar painel"
			><Glyph name="x" size={18} /></button
		>
	</div>
	<div class="find">
		<label class="inp"
			><Glyph name="search" size={14} /><input
				bind:this={input}
				bind:value={query}
				type="search"
				placeholder="id, texto, quest…"
				aria-label="Buscar passo para fixar"
			/></label
		>
	</div>
	<div class="list">
		{#if behind.length}
			<p class="lbl gl">Atrás do cursor · pendentes</p>
			{#each behind as r, n (r.s.id)}{@render row(r, n)}{/each}
		{/if}
		{#if ahead.length}
			<p class="lbl gl">À frente</p>
			{#each ahead as r, n (r.s.id)}{@render row(r, behind.length + n)}{/each}
		{/if}
		{#if !flat.length}
			<p class="su sm">Nenhum passo pendente com esse texto.</p>
		{/if}
		<div class="ft">
			<button
				class="btn btn-p"
				type="button"
				disabled={!pick}
				onclick={() => pick && onpin(pick.s.id, false)}>Fixar {pick?.s.ref ?? ''}</button
			>
			{#if pick && before > 0}
				<button
					class="btn btn-g"
					type="button"
					title="O cursor vai para o passo escolhido e os pendentes antes dele viram feitos; o lote pode ser desfeito."
					onclick={() => onpin(pick.s.id, true)}>Marcar os {before} anteriores</button
				>
			{/if}
			<button class="btn btn-g" type="button" onclick={onclose}>Cancelar</button>
			<span class="su keys"><span class="kbd">↑↓</span> <span class="kbd">↵</span></span>
		</div>
	</div>
</div>

<style>
	.hd {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 16px 10px;
		border-bottom: 1px solid var(--hairline);
	}
	.ttl {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.ttl .sr {
		font-size: 20px;
	}
	.sm {
		font-size: 12px;
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
	.find {
		padding: 12px 16px 0;
	}
	.list {
		display: flex;
		flex-direction: column;
		padding: 8px 16px 16px;
	}
	.gl {
		padding: 10px 0 4px;
	}
	button.it {
		width: 100%;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		font: inherit;
		font-size: 13px;
		text-align: left;
		cursor: pointer;
	}
	.tx {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ft {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding-top: 14px;
		border-top: 1px solid var(--hairline);
		margin-top: 8px;
	}
	.ft .btn {
		height: 40px;
	}
	.keys {
		margin-left: auto;
		font-size: 11px;
		align-self: center;
	}
</style>
