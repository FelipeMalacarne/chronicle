<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { iconFor } from '#lib/progress/icons';
	import type { Chip } from '#lib/progress/view';

	/** Chip de objetivo: papel, veredito (unknown pontilhado, nunca vermelho), ícone, nome e dica. */
	let {
		chip,
		small = false,
		hint = true
	}: { chip: Chip; small?: boolean; hint?: boolean } = $props();
	const icon = $derived(iconFor(chip.ref));
	const title = $derived(
		`${chip.roleLabel} ${chip.label}: ${chip.verdict === 'met' ? 'feito' : chip.verdict === 'unmet' ? 'não atende' : 'não sabemos'} (${chip.key})`
	);
</script>

<span class="obj" class:obj-s={small} class:obj-unk={chip.view.unknown} {title}>
	<span class="role" class:role-use={chip.role === 'uses' || chip.role === 'requires'}
		>{chip.roleLabel}</span
	>
	<span class="v {chip.view.cls}">
		{#if chip.verdict === 'met'}<Glyph name="check" size={9} />{:else}{chip.view.glyph}{/if}
	</span>
	{#if icon}<img class="ski" src={icon} alt="" />{/if}{chip.label}
	{#if hint && chip.view.hint}<span class="k">{chip.view.hint}</span>{/if}
</span>
