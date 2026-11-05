<script lang="ts">
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { chipRole, verdictWords, type Chip } from '#lib/progress/view';

	/**
	 * Chip de objetivo: papel, veredito (unknown pontilhado, nunca vermelho), ícone, nome e dica.
	 * v5 limpeza: the role shows only when it is not "conclui"; the verdict is a glyph (✓, ✕, ?)
	 * with its words for screen readers; the hint text stays only for "não vale mais".
	 * Com `link`, abre o painel da entidade no Mundo (que leva ao wiki); desligue dentro de linhas
	 * que já são clicáveis, onde um link aninhado não é permitido.
	 */
	let {
		chip,
		small = false,
		hint = true,
		link = true
	}: { chip: Chip; small?: boolean; hint?: boolean; link?: boolean } = $props();
	const icon = $derived(iconFor(chip.ref));
	const role = $derived(chipRole(chip));
	const words = $derived(verdictWords(chip.verdict));
	const title = $derived(
		`${chip.roleLabel} ${chip.label}: ${words}${chip.view.unknown && chip.view.hint ? `, ${chip.view.hint}` : ''} (${chip.key})`
	);
	const href = $derived(
		link && chip.ref ? `${resolve('/mundo')}?e=${encodeURIComponent(chip.ref)}` : null
	);
</script>

{#snippet body()}
	{#if role}<span class="role" class:role-use={chip.role === 'uses' || chip.role === 'requires'}
			>{role}</span
		>{/if}
	<span class="v {chip.view.cls}" aria-hidden="true">
		{#if chip.verdict === 'met'}<Glyph
				name="check"
				size={9}
			/>{:else if chip.verdict === 'unmet'}<span class="x">✕</span>{:else}{chip.view.glyph}{/if}
	</span>
	<span class="vh">{role || chip.roleLabel}, {words}:</span>
	{#if icon}<img class="ski" src={icon} alt="" />{/if}{chip.label}
	{#if hint && chip.verdict === 'met' && chip.view.hint}<span class="k">{chip.view.hint}</span>{/if}
{/snippet}

{#if href}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + query da entidade -->
	<a class="obj obj-link" class:obj-s={small} class:obj-unk={chip.view.unknown} {title} {href}
		>{@render body()}</a
	>
{:else}
	<span class="obj" class:obj-s={small} class:obj-unk={chip.view.unknown} {title}
		>{@render body()}</span
	>
{/if}

<style>
	.obj-link:hover {
		border-color: var(--gold-ink);
	}
	.x {
		font-size: 8px;
		line-height: 1;
		color: var(--not-started);
	}
</style>
