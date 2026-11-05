<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { entityPanel, openEntity } from '#lib/progress/entity.svelte';
	import { iconFor } from '#lib/progress/icons';
	import type { Chip } from '#lib/progress/view';

	/**
	 * Chip de objetivo da v6 (V6Fundamentos, átomo 1): só veredito, ícone e nome; o papel vira o
	 * rótulo do grupo ("Conclui", "Usa") e a chave técnica vai para o título. Abre o painel da
	 * entidade por cima da tela. `small` é o chip da linha (dentro de um link: não clicável).
	 */
	let { chip, small = false, hint }: { chip: Chip; small?: boolean; hint?: string } = $props();
	const icon = $derived(iconFor(chip.ref));
	const VERDICT = { met: 'feito', unmet: 'não atende', unknown: 'não sabemos' } as const;
	const title = $derived(
		`${chip.roleLabel} ${chip.label} · ${VERDICT[chip.verdict]}${chip.view.hint && chip.verdict === 'unknown' ? `: ${chip.view.hint}` : ''} (${chip.key})`
	);
	const sel = $derived(!!chip.ref && entityPanel.ref === chip.ref);
</script>

{#snippet body()}
	<span class="v {chip.view.cls}" aria-hidden="true"
		>{#if chip.verdict === 'met'}<Glyph name="check" size={9} />{:else}{chip.view.glyph}{/if}</span
	><span class="vh">{VERDICT[chip.verdict]}:</span>{#if icon}<img
			class="ski"
			src={icon}
			alt=""
		/>{/if}{chip.label}{#if hint}<span class="k">{hint}</span>{/if}
{/snippet}

{#if small || !chip.ref}
	<span class="obj" class:obj-s={small} {title}>{@render body()}</span>
{:else}
	<button
		type="button"
		class="obj"
		class:sel
		aria-haspopup="dialog"
		{title}
		onclick={() => openEntity(chip.ref)}>{@render body()}</button
	>
{/if}
