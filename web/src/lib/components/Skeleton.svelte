<script lang="ts">
	let { lines = 3, height = 14 }: { lines?: number; height?: number } = $props();
</script>

<div class="sk" aria-busy="true" aria-live="polite">
	<span class="visually-hidden">Carregando…</span>
	{#each Array.from({ length: lines }, (_, i) => i) as i (i)}
		<span class="bar" style:height="{height}px" style:width="{100 - (i % 3) * 18}%"></span>
	{/each}
</div>

<style>
	.sk {
		display: grid;
		gap: 10px;
	}
	.bar {
		display: block;
		border-radius: 6px;
		background: linear-gradient(90deg, var(--elevated), var(--overlay), var(--elevated));
		background-size: 200% 100%;
		animation: shimmer 1.4s infinite linear;
	}
	@keyframes shimmer {
		from {
			background-position: 200% 0;
		}
		to {
			background-position: -200% 0;
		}
	}
</style>
