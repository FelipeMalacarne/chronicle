<script lang="ts">
	import { ApiError } from '#lib/api/client';
	import Icon from './Icon.svelte';
	let { error, retry }: { error: Error; retry?: () => void } = $props();
	const offline = $derived(error instanceof ApiError && error.offline);
</script>

<div class="err card" role="alert">
	<span class="ic"><Icon name="alert" size={18} /></span>
	<div class="txt">
		<p class="title">{offline ? 'Servidor indisponível' : 'Não foi possível carregar'}</p>
		<p class="caption">
			{offline
				? 'O núcleo Go não respondeu em /api. Em dev, use `pnpm dev:mock` ou suba o servidor na porta 8080.'
				: error.message}
		</p>
	</div>
	{#if retry}
		<button class="btn btn-ghost btn-compact" type="button" onclick={retry}>Tentar de novo</button>
	{/if}
</div>

<style>
	.err {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		border-color: var(--error);
		background: var(--error-tint);
	}
	.ic {
		color: var(--error);
		display: inline-flex;
	}
	.txt {
		flex: 1;
	}
	.title {
		font-weight: 500;
	}
</style>
