<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Snippet } from 'svelte';
	import { me } from '#lib/progress/me.svelte';

	/**
	 * Cabeçalho do Mundo: título e as abas Metas · Entidades · Grafo (a ordem de V7Mundo1440). Com
	 * `lead`, o da v7 (título e a frase abaixo, as abas à direita): as Metas são da frota. Sem ele, o
	 * da v6 (V6Mundo1440, V6Grafo1440), que diz de qual conta é a tela.
	 */
	let {
		title,
		current,
		lead,
		children
	}: {
		title: string;
		current: 'entidades' | 'metas' | 'grafo';
		lead?: string;
		children?: Snippet;
	} = $props();
</script>

<header class="mh" class:v7={!!lead}>
	{#if lead}
		<div class="ttl">
			<h1 class="sr">{title}</h1>
			<p class="mu">{lead}</p>
		</div>
	{:else}
		<div class="ttl">
			<p class="su">
				Mundo · {me.state?.account.rsn ?? 'conta aberta'} · compartilhado entre as rotas
			</p>
			<h1 class="sr">{title}</h1>
		</div>
	{/if}
	<nav class="tabs" aria-label="Mundo">
		<a
			class:on={current === 'metas'}
			href={resolve('/mundo/metas')}
			aria-current={current === 'metas' ? 'page' : undefined}>Metas</a
		>
		<a
			class:on={current === 'entidades'}
			href={resolve('/mundo')}
			aria-current={current === 'entidades' ? 'page' : undefined}>Entidades</a
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
	.v7 {
		align-items: flex-end;
	}
	.v7 .ttl {
		margin-right: auto;
	}
	.v7 .ttl h1 {
		font-size: 30px;
	}
	.v7 .ttl p {
		font-size: 13px;
	}
</style>
