<script lang="ts">
	import { resolve } from '$app/paths';
	import Glyph from './Glyph.svelte';
	import { iconFor } from '#lib/progress/icons';
	import { chipTitle } from '#lib/progress/view';
	import type { Chip } from '#lib/progress/view';

	/**
	 * Chip de objetivo em duas camadas (v6): veredito pelo glifo (✓ cheio, ○ vermelho, ? pontilhado)
	 * e o nome com ícone. O papel vira rótulo do grupo em quem o usa; chave técnica e fonte ficam
	 * no `title`. O único sufixo é `current` (nível atual numa skill não atendida), quando houver.
	 * Com `link`, abre o painel da entidade no Mundo; desligue dentro de linhas já clicáveis.
	 */
	let {
		chip,
		small = false,
		link = true,
		current = ''
	}: { chip: Chip; small?: boolean; link?: boolean; current?: string } = $props();
	const icon = $derived(iconFor(chip.ref));
	const title = $derived(chipTitle(chip));
	const href = $derived(
		link && chip.ref ? `${resolve('/mundo')}?e=${encodeURIComponent(chip.ref)}` : null
	);
</script>

{#snippet body()}
	<span class="v {chip.view.cls}" aria-hidden="true">
		{#if chip.verdict === 'met'}<Glyph name="check" size={9} />{:else}{chip.view.glyph}{/if}
	</span>
	<span class="vh"
		>{chip.roleLabel}, {chip.verdict === 'met'
			? 'feito'
			: chip.verdict === 'unmet'
				? 'não atende'
				: 'não sabemos'}:</span
	>
	{#if icon}<img class="ski" src={icon} alt="" />{/if}{chip.label}
	{#if current && chip.kind === 'skill' && chip.verdict === 'unmet'}<span class="k">{current}</span
		>{/if}
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
	.vh {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
