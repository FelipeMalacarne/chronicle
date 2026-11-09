<script lang="ts">
	import { microbotApi, MicrobotError } from '#lib/api/microbot';

	/*
	 * O fim do log do cliente de dev (doc 08, "logs e conta"): o agent lê o client.log do home de
	 * dev, como `mb-client logs`, e mascara o que parece token ou sessão. Fechado por padrão; aberto,
	 * relê a cada 5 s se "acompanhar" estiver ligado.
	 */
	let open = $state(false);
	let follow = $state(false);
	let lines = $state<string[]>([]);
	let file = $state('');
	let filter = $state('');
	let error = $state<string | null>(null);
	let box = $state<HTMLPreElement>();

	const shown = $derived(
		filter.trim()
			? lines.filter((l) => l.toLowerCase().includes(filter.trim().toLowerCase()))
			: lines
	);

	async function load() {
		try {
			const r = await microbotApi.logs(300);
			lines = r.lines;
			file = r.file;
			error = null;
			queueMicrotask(() => box?.scrollTo({ top: box.scrollHeight }));
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	$effect(() => {
		if (!open) return;
		void load();
		if (!follow) return;
		const t = setInterval(() => {
			if (!document.hidden) void load();
		}, 5000);
		return () => clearInterval(t);
	});
</script>

<section class="panel">
	<div class="plaque">
		Log do cliente<span class="r"
			>{open ? `${file || 'client.log'} · últimas ${lines.length} linhas` : 'mb-client logs'}</span
		>
	</div>
	<div class="pad stack">
		<div class="acts">
			<button class="btn btn-s" type="button" onclick={() => (open = !open)}
				>{open ? 'Fechar' : 'Abrir o log'}</button
			>
			{#if open}
				<button class="btn btn-g btn-s" type="button" onclick={load}>Reler</button>
				<label class="chk"><input type="checkbox" bind:checked={follow} /> acompanhar</label>
				<input class="txt" placeholder="filtrar (ERROR, nome do plugin…)" bind:value={filter} />
			{/if}
		</div>
		{#if error}<p class="err">{error}</p>{/if}
		{#if open}
			<pre class="raw" bind:this={box}>{shown.join('\n') || 'Log vazio.'}</pre>
		{/if}
	</div>
</section>

<style>
	.pad {
		padding: 12px;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
	}
	.chk {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.txt {
		flex: 1 1 180px;
		min-width: 0;
		height: 30px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.raw {
		margin: 0;
		max-height: 360px;
		overflow: auto;
		padding: 10px;
		border-radius: 4px;
		background: var(--frame);
		color: var(--on-frame);
		font:
			400 11px/1.5 'JetBrains Mono',
			monospace;
		white-space: pre-wrap;
		word-break: break-all;
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
</style>
