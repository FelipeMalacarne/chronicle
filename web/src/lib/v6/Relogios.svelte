<script lang="ts">
	import { microbotApi, MicrobotError, type Timers } from '#lib/api/microbot';
	import { clockRows } from '#lib/progress/relogios';

	/*
	 * Relógios (doc 08, fase 4, entrega 1): o Time Tracking do RuneLite pela ponte, os mesmos números
	 * do painel do cliente. Só lê. Relê a cada minuto e conta o tempo no navegador entre leituras,
	 * pelo relógio da máquina do jogo (a diferença para o daqui é guardada na leitura).
	 */
	let { running }: { running: boolean } = $props();

	let data = $state<Timers | null>(null);
	let error = $state<string | null>(null);
	/** Segundos a somar ao relógio do navegador para ter o da máquina do jogo. */
	let skew = 0;
	let now = $state(Date.now() / 1000);

	async function load() {
		try {
			const t = await microbotApi.timers();
			skew = t.now - Date.now() / 1000;
			now = t.now;
			data = t;
			error = null;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	$effect(() => {
		if (!running) {
			data = null;
			return;
		}
		void load();
		const read = setInterval(() => {
			if (!document.hidden) void load();
		}, 60000);
		const tick = setInterval(() => (now = Date.now() / 1000 + skew), 15000);
		return () => {
			clearInterval(read);
			clearInterval(tick);
		};
	});

	const view = $derived(data ? clockRows(data.timers, now) : null);
	const at = (s?: number) =>
		s
			? new Date(s * 1000).toLocaleString('pt-BR', {
					weekday: 'short',
					hour: '2-digit',
					minute: '2-digit'
				})
			: undefined;
</script>

<section class="panel">
	<div class="plaque">Relógios<span class="r">Time Tracking</span></div>
	<div class="pad">
		{#if !running}
			<span class="su sm0">O cliente não está rodando.</span>
		{:else if error && !data}
			<p class="err">{error}</p>
		{:else if !data}
			<span class="su sm0" aria-busy="true">Lendo…</span>
		{:else if !data.enabled}
			<span class="su sm0">O plugin Time Tracking está desligado no cliente.</span>
		{:else if view}
			{#each view.rows as r (r.key)}
				<div class="clk" title={r.at ? `pronto ${at(r.at)}` : undefined}>
					<span class="dot {r.tone}" aria-hidden="true"></span>
					<span class="nm">{r.label}</span>
					<span class="when {r.tone}">{r.when}</span>
				</div>
			{:else}
				<span class="su sm0">Nada plantado que o Time Tracking conheça.</span>
			{/each}
			{#if view.unseen.length}
				<p
					class="unseen"
					title="O Time Tracking só conhece um canteiro depois de passar perto dele"
				>
					Não vistos: {view.unseen.join(', ')}
				</p>
			{/if}
		{/if}
	</div>
</section>

<style>
	.pad {
		padding: 8px 12px 12px;
	}
	.clk {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 5px 0;
		border-bottom: 1px solid var(--hairline);
		font-size: 12px;
	}
	.nm {
		flex: 1;
		min-width: 0;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
		background: var(--ink-subtle);
	}
	.dot.ready {
		background: var(--done);
	}
	.dot.growing {
		background: var(--progress);
	}
	.when {
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
	}
	.when.ready {
		color: var(--done);
		font-weight: 600;
	}
	.when.idle {
		color: var(--ink-subtle);
	}
	.unseen {
		margin: 8px 0 0;
		font-size: 11px;
		color: var(--ink-subtle);
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
</style>
