<script lang="ts">
	import '#lib/styles/pergaminho.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError } from '#lib/api/client';
	import { connectLive, dismissDetection, live } from '#lib/api/events.svelte';
	import Sidebar from '#lib/v4/Sidebar.svelte';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { theme } from '#lib/progress/theme.svelte';
	import Toast from '#lib/v4/Toast.svelte';

	let { children } = $props();
	const account = $derived(currentAccount());
	$effect(() => {
		void loadSummary(account);
	});
	// Sem conta, ou com uma conta que não existe mais, o primeiro acesso (V4Setup) escolhe ou cria.
	const setup = $derived(page.url.pathname === '/setup');
	$effect(() => {
		const missing = session.error instanceof ApiError && session.error.status === 404;
		if (!setup && (!account || missing)) void goto(resolve('/setup'), { replaceState: true });
	});
	// Tempo real: um stream por conta; os eventos refazem os recursos abertos (docs/04, 6.6).
	$effect(() => {
		if (!account || setup) return;
		return connectLive(account);
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
	{#if !setup}<Sidebar summary={session.summary} account={session.account} />{/if}
	<main id="conteudo">
		{@render children()}
	</main>
	{#if live.toast}
		{#key live.toast.id}
			<Toast auto title={live.toast.title} text={live.toast.text} onclose={dismissDetection} />
		{/key}
	{/if}
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
