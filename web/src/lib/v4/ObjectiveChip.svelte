<script lang="ts">
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { iconFor } from '#lib/progress/icons';
	import type { Chip } from '#lib/progress/view';

	/**
	 * Chip de objetivo: papel, veredito (unknown pontilhado, nunca vermelho), ícone, nome e dica.
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
	const title = $derived(
		`${chip.roleLabel} ${chip.label}: ${chip.verdict === 'met' ? 'feito' : chip.verdict === 'unmet' ? 'não atende' : 'não sabemos'} (${chip.key})`
	);
	const href = $derived(
		link && chip.ref ? `${resolve('/mundo')}?e=${encodeURIComponent(chip.ref)}` : null
	);
</script>

{#snippet body()}
	<span class="role" class:role-use={chip.role === 'uses' || chip.role === 'requires'}
		>{chip.roleLabel}</span
	>
	<span class="v {chip.view.cls}">
		{#if chip.verdict === 'met'}<Glyph name="check" size={9} />{:else}{chip.view.glyph}{/if}
	</span>
	{#if icon}<img class="ski" src={icon} alt="" />{/if}{chip.label}
	{#if hint && chip.view.hint}<span class="k">{chip.view.hint}</span>{/if}
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
</style>
