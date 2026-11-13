<script lang="ts">
	import '#lib/styles/pergaminho.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError } from '#lib/api/client';
	import { connectLive, dismissDetection, live } from '#lib/api/events.svelte';
	import Sidebar from '#lib/v6/Sidebar.svelte';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { loadSummary, session } from '#lib/progress/session.svelte';
	import { accountSub, loadMe, me } from '#lib/progress/me.svelte';
	import { theme } from '#lib/progress/theme.svelte';
	import Toast from '#lib/v6/Toast.svelte';
	import EntityPanel from '#lib/v6/EntityPanel.svelte';
	import AccountHead from '#lib/v6/AccountHead.svelte';
	import { chooseCockpitAccount } from '#lib/progress/microbot.svelte';

	let { children } = $props();
	const account = $derived(currentAccount());
	$effect(() => {
		void loadSummary(account);
		void loadMe(account);
	});
	// Sem conta escolhida, ou com uma que não existe mais, só a Frota e Nova conta fazem sentido:
	// as outras telas são da conta. Nesse caso o shell fica sem a barra lateral.
	const missing = $derived(session.error instanceof ApiError && session.error.status === 404);
	const fleetPage = $derived(
		page.url.pathname === '/' || page.url.pathname.startsWith('/contas/nova')
	);
	const setup = $derived(fleetPage && (!account || missing));
	// O Mundo e os Plugins são da frota (decisoes-canvas.md, 7.1): sem o cabeçalho da conta.
	const accountPage = $derived(
		!fleetPage &&
			!page.url.pathname.startsWith('/mundo') &&
			!page.url.pathname.startsWith('/plugins')
	);
	$effect(() => {
		if (!fleetPage && (!account || missing)) void goto(resolve('/'), { replaceState: true });
	});
	// O cliente que as telas de Cliente controlam é o da conta aberta (docs/11).
	$effect.pre(() => {
		if (account) chooseCockpitAccount(account);
	});
	// Tempo real: um stream por conta; os eventos refazem os recursos abertos (docs/04, 6.6).
	$effect(() => {
		if (!account || setup) return;
		return connectLive(account);
	});
	// ⌘K / Ctrl+K: a busca da sidebar abre o Mundo com o campo de busca em foco.
	function searchKey(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k' && !setup) {
			e.preventDefault();
			void goto(resolve('/mundo')).then(() =>
				document.querySelector<HTMLInputElement>('input[aria-label="Buscar entidade"]')?.focus()
			);
		}
	}
	// O tema também no body: o fundo fora do shell (overscroll) segue o tema.
	$effect(() => {
		document.body.dataset.theme = theme.value;
	});
</script>

<svelte:window onkeydown={searchKey} />

<svelte:head>
	<title>Chronicle</title>
</svelte:head>

<div class="pg shell" class:bare={setup} data-theme={theme.value}>
	<a class="skip" href="#conteudo">Pular para o conteúdo</a>
	{#if !setup}<Sidebar
			summary={session.summary}
			account={session.account}
			name={me.state?.account.rsn}
			sub={accountSub(me.state)}
			skills={me.state?.skills}
		/>{/if}
	<main id="conteudo">
		{#if accountPage && me.state && me.state.account.id === account}
			<AccountHead state={me.state} summary={session.summary} />
		{/if}
		{@render children()}
	</main>
	{#if !setup}<EntityPanel />{/if}
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
		align-content: flex-start;
		min-height: 100vh;
		/* A madeira da sidebar desce a página inteira; a sidebar em si fica presa no topo. */
		background: linear-gradient(to right, var(--frame) 240px, var(--ground) 240px);
	}
	@media (max-width: 900px) {
		.shell {
			background: var(--ground);
		}
	}
	.shell.bare {
		background: var(--ground);
	}
	main {
		flex: 999 1 560px;
		max-width: 1440px;
		min-width: 0;
		padding: 24px 32px 36px;
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	@media (max-width: 900px) {
		main {
			padding: 14px 14px 24px;
			gap: 14px;
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
