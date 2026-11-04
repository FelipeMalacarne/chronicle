<script lang="ts">
	let {
		title,
		text,
		action,
		onaction,
		onclose
	}: { title: string; text: string; action?: string; onaction?: () => void; onclose: () => void } =
		$props();

	$effect(() => {
		const t = setTimeout(onclose, 6000);
		return () => clearTimeout(t);
	});
</script>

<div class="toast" role="status" aria-live="polite">
	<span><span class="t">{title}</span> · {text}</span>
	{#if action && onaction}<button type="button" onclick={onaction}>{action}</button>{/if}
	<button type="button" onclick={onclose}>fechar</button>
</div>
