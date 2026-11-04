<script lang="ts">
	import '#lib/styles/tokens.css';
	import TopBar from '#lib/components/TopBar.svelte';
	let { children } = $props();
</script>

<svelte:head>
	<title>Ironman Planner</title>
</svelte:head>

<a class="skip" href="#conteudo">Pular para o conteúdo</a>
<TopBar />
<main id="conteudo" class="shell">
	{@render children()}
</main>
<footer class="foot caption">
	Conteúdo do guia por OzirisLoL, versão interativa de ironman.guide. Uso pessoal; atribuição
	mantida.
</footer>

<style>
	.skip {
		position: absolute;
		left: -999px;
		top: 8px;
		z-index: 20;
		padding: 8px 12px;
		border-radius: var(--r-chip);
		background: var(--accent);
		color: var(--accent-text);
	}
	.skip:focus {
		left: 8px;
	}
	.shell {
		max-width: 1180px;
		margin: 0 auto;
		padding: var(--sp-8) var(--sp-6) var(--sp-8);
	}
	.foot {
		max-width: 1180px;
		margin: 0 auto;
		padding: 0 var(--sp-6) var(--sp-8);
	}
	@media (max-width: 719px) {
		.shell {
			padding: var(--sp-5) var(--sp-4);
		}
		.foot {
			padding: 0 var(--sp-4) calc(var(--bottombar-h) + var(--sp-6));
		}
	}
</style>
