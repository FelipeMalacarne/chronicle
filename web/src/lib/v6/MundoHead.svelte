<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Snippet } from 'svelte';

	/** Cabeçalho do Mundo (V6Mundo1440, V6Metas1440, V6Grafo1440): título e as abas Entidades · Metas · Grafo. */
	let {
		title,
		current,
		children
	}: { title: string; current: 'entidades' | 'metas' | 'grafo'; children?: Snippet } = $props();
</script>

<header class="mh">
	<div class="ttl">
		<p class="su">Mundo · compartilhado entre as rotas</p>
		<h1 class="sr">{title}</h1>
	</div>
	<nav class="tabs" aria-label="Mundo">
		<a
			class:on={current === 'entidades'}
			href={resolve('/mundo')}
			aria-current={current === 'entidades' ? 'page' : undefined}>Entidades</a
		>
		<a
			class:on={current === 'metas'}
			href={resolve('/mundo/metas')}
			aria-current={current === 'metas' ? 'page' : undefined}>Metas</a
		>
		<a
			class:on={current === 'grafo'}
			href={resolve('/mundo/grafo')}
			aria-current={current === 'grafo' ? 'page' : undefined}>Grafo</a
		>
	</nav>
	{@render children?.()}
</header>

<style>
	.mh {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: 8px;
	}
	.ttl p {
		margin: 0;
		font-size: 12px;
	}
	.ttl h1 {
		font-size: 26px;
	}
</style>
