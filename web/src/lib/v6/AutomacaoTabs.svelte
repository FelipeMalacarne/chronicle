<script lang="ts">
	import { resolve } from '$app/paths';

	/*
	 * As duas telas do Cliente de uma conta (página 8 do canvas, V7Cliente1440). A conta é a que
	 * está aberta: o seletor e a aba Workers saíram, a Frota faz o papel deles.
	 */
	let { current }: { current: 'workers' | 'cockpit' | 'plugins' } = $props();
</script>

<div class="seg" role="tablist" aria-label="Cliente">
	<a class:on={current === 'cockpit'} href={resolve('/automacao')}>Jogo</a>
	<a class:on={current === 'plugins'} href={resolve('/automacao/plugins')}>Plugins</a>
</div>

<style>
	.seg a {
		display: inline-flex;
		align-items: center;
		height: 32px;
		padding: 0 10px;
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		color: var(--ink-muted);
		text-decoration: none;
		background: var(--elevated);
	}
	.seg a + a {
		border-left: 1px solid var(--border);
	}
	.seg a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
</style>
