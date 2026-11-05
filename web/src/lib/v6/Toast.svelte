<script lang="ts">
	// auto: detecção automática (plugin ou hiscores), com o marcador "feito automático" à
	// esquerda, como o toast de V4AgoraOziris.
	let {
		title,
		text,
		auto = false,
		action,
		onaction,
		onclose
	}: {
		title: string;
		text: string;
		auto?: boolean;
		action?: string;
		onaction?: () => void;
		onclose: () => void;
	} = $props();

	$effect(() => {
		const t = setTimeout(onclose, 6000);
		return () => clearTimeout(t);
	});
</script>

<div class="toast" role="status" aria-live="polite">
	{#if auto}<span class="st st-auto mk" aria-hidden="true"></span>{/if}
	<span><span class="t">{title}</span> · {text}</span>
	{#if action && onaction}<button type="button" onclick={onaction}>{action}</button>{/if}
	<button type="button" onclick={onclose}>fechar</button>
</div>

<style>
	.mk {
		width: 14px;
		height: 14px;
	}
</style>
