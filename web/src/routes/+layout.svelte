<script lang="ts">
	import '#lib/styles/pergaminho.css';
	import Sidebar from '#lib/v4/Sidebar.svelte';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { theme } from '#lib/progress/theme.svelte';

	let { children } = $props();
	const account = $derived(currentAccount());
	$effect(() => {
		void loadSummary(account);
	});
	// O tema também no body: o fundo fora do shell (overscroll) segue o tema.
	$effect(() => {
		document.body.dataset.theme = theme.value;
	});
</script>

<svelte:head>
	<title>Ironman Planner</title>
</svelte:head>

<div class="pg shell" data-theme={theme.value}>
	<a class="skip" href="#conteudo">Pular para o conteúdo</a>
	<Sidebar summary={session.summary} account={session.account} />
	<main id="conteudo">
		{@render children()}
	</main>
</div>

<style>
	:global(body) {
		background: var(--ground);
	}
	.shell {
		display: flex;
		flex-wrap: wrap;
		min-height: 100vh;
	}
	main {
		flex: 999 1 560px;
		min-width: 0;
		padding: 24px 32px 36px;
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	@media (max-width: 640px) {
		main {
			padding: 16px;
		}
	}
	.skip {
		position: absolute;
		left: -999px;
		top: 8px;
		z-index: 50;
		padding: 8px 12px;
		border-radius: 4px;
		background: var(--frame);
		color: var(--gold);
	}
	.skip:focus {
		left: 8px;
	}
</style>
