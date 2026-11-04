<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { progressApi } from '#lib/api/client';
	import { session } from '#lib/progress/session.svelte';

	/* /rotas/<guia> abre a seção do cursor (ou a primeira, sem conta). */
	const guideId = $derived(page.params.guide ?? '');
	$effect(() => {
		const id = guideId;
		const route = session.summary?.routes.find((r) => r.guide === id);
		if (session.account && session.loading) return;
		void (async () => {
			let section = route?.cursor_section ?? route?.sections[0]?.id;
			if (!section)
				section = (await progressApi.guides()).guides.find((g) => g.id === id)?.sections[0]?.id;
			if (!section) return goto(resolve('/rotas'), { replaceState: true });
			const base = resolve('/rotas/[guide]/[section]', { guide: id, section });
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- âncora sobre resolve()
			return goto(route?.cursor ? `${base}#passo-${route.cursor.step_id}` : base, {
				replaceState: true
			});
		})();
	});
</script>

<p class="su" aria-busy="true">Abrindo a rota…</p>
