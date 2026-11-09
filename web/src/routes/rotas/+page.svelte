<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { currentAccount } from '#lib/progress/account.svelte';
	import { session } from '#lib/progress/session.svelte';

	/*
	 * A lista de Rotas saiu na v6 ("Rota · decisões", item 8): "Rotas" abre a rota ativa na seção
	 * do cursor, com as abas Oziris · BRUHsailer · Comparar no topo.
	 */
	$effect(() => {
		// O efeito desta página roda antes do layout carregar a conta: com uma conta conhecida,
		// espera o resumo dela, senão cairia na primeira guia e não na rota ativa.
		const account = currentAccount();
		if (account && (session.account !== account || session.loading || !session.summary)) return;
		const guide =
			session.summary?.routes.find((r) => r.active)?.guide ?? session.summary?.routes[0]?.guide;
		void (async () => {
			const id = guide ?? (await progressApi.guides()).guides[0]?.id;
			if (id) await goto(resolve('/rotas/[guide]', { guide: id }), { replaceState: true });
		})();
	});
</script>

<svelte:head>
	<title>Rotas · Chronicle</title>
</svelte:head>

<p class="su" aria-busy="true">Abrindo a rota ativa…</p>
