<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import PageHead from '#lib/components/PageHead.svelte';
</script>

<PageHead
	eyebrow={`Erro · ${page.status}`}
	title={page.status === 404 ? 'Página não encontrada' : 'Algo deu errado'}
	description={page.error?.message}
/>
<a class="btn btn-ghost" href={resolve('/')}>Voltar para Agora</a>
