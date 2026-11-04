<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
</script>

<header class="head">
	<p class="ey">Erro · {page.status}</p>
	<h1 class="sr">{page.status === 404 ? 'Página não encontrada' : 'Algo deu errado'}</h1>
	{#if page.error?.message}<p class="mu">{page.error.message}</p>{/if}
</header>
<a class="btn btn-s" href={resolve('/')}>Voltar para Agora</a>

<style>
	.head {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	h1 {
		font-size: 30px;
	}
	p {
		margin: 0;
	}
	.btn {
		align-self: flex-start;
	}
</style>
