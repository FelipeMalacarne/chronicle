<script lang="ts">
	import { microbotApi, MicrobotError, type Run, type RunHistory } from '#lib/api/microbot';
	import { RUN_LABEL, runDuration, runFinal, runOutcome, runWhen } from '#lib/progress/fazer';

	/*
	 * Últimas execuções (V7Conta1440; antes o painel Runs do cockpit, V6MicrobotRuns1440): os
	 * últimos runs da conta, o mais novo primeiro: quando, o quê, quem pediu e o resultado, com a
	 * trilha ao abrir a linha. Só lê; relê a cada 30 s e quando Fazer agora avisa que um run
	 * terminou (version).
	 */
	let { version = 0 }: { version?: number } = $props();

	const FIRST = 6;
	let data = $state<RunHistory | null>(null);
	let error = $state<string | null>(null);
	let open = $state<string | null>(null);
	let all = $state(false);

	async function load() {
		try {
			data = await microbotApi.runs();
			error = null;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	$effect(() => {
		void version;
		void load();
		const read = setInterval(() => {
			if (!document.hidden) void load();
		}, 30000);
		return () => clearInterval(read);
	});

	const runs = $derived(data ? (all ? data.runs : data.runs.slice(0, FIRST)) : []);
	const tone = (r: Run) =>
		!runFinal(r) ? 'going' : r.state === 'done' ? 'ok' : r.state === 'canceled' ? 'off' : 'bad';
	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const short = (className: string) =>
		className
			.split('.')
			.pop()
			?.replace(/Plugin$/, '');
	const why = (r: Run) =>
		[r.trigger ? 'sozinho' : 'você pediu', runOutcome(r)].filter(Boolean).join(' · ');
</script>

<section class="card">
	<div class="h hb">
		<h2>Últimas execuções</h2>
		{#if data?.runs.length}<span>{data.runs.length} guardadas</span>{/if}
	</div>
	{#if error && !data}
		<p class="msg err">{error}</p>
	{:else if !data}
		<p class="msg mu" aria-busy="true">Lendo…</p>
	{:else}
		{#each runs as r (r.id)}
			<button
				class="hist"
				type="button"
				aria-expanded={open === r.id}
				onclick={() => (open = open === r.id ? null : r.id)}
			>
				<span class="t" title={runWhen(r.startedAt)}>{hhmm(r.startedAt)}</span>
				<span class="dot {tone(r)}" aria-hidden="true"></span>
				<span class="nm"><b>{short(r.className)}</b></span>
				<span class="r">{why(r)}</span>
				<span class="d">{runDuration(r) ?? ''}</span>
				<span class="res {tone(r)}">{RUN_LABEL[r.state]}</span>
			</button>
			{#if open === r.id}
				<div class="open">
					{#if r.error}<p class="err">{r.error}</p>{/if}
					<ol class="trail">
						{#each r.events as ev, i (i)}
							<li><span class="t">{hhmm(ev.at)}</span>{ev.text}</li>
						{/each}
					</ol>
				</div>
			{/if}
		{:else}
			<p class="msg mu">Nenhuma execução ainda.</p>
		{/each}
		{#if data.runs.length > FIRST}
			<button class="more" type="button" onclick={() => (all = !all)}
				>{all ? 'ver menos' : `ver as ${data.runs.length}`}</button
			>
		{/if}
		{#if data.error}
			<p class="msg err" title={data.error}>O histórico não foi gravado.</p>
		{/if}
	{/if}
</section>

<style>
	.hb {
		padding-bottom: 10px;
	}
	button.hist {
		width: 100%;
		border-left: 0;
		border-right: 0;
		border-bottom: 0;
		background: none;
		color: var(--ink);
		font: inherit;
		font-size: 13px;
		text-align: left;
		cursor: pointer;
	}
	button.hist:hover {
		background: var(--elevated);
	}
	.nm {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nm b {
		font-weight: 600;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		justify-self: center;
		background: var(--ink-subtle);
	}
	.dot.ok {
		background: var(--done);
	}
	.dot.going {
		background: var(--progress);
	}
	.dot.bad {
		background: var(--not-started);
	}
	.res.going {
		color: var(--progress);
	}
	.res.off {
		color: var(--ink-subtle);
	}
	.open {
		padding: 0 16px 10px 100px;
	}
	.trail {
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 11px;
		color: var(--ink-muted);
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.trail li {
		overflow-wrap: anywhere;
	}
	.trail .t {
		font-family: 'JetBrains Mono', monospace;
		color: var(--ink-subtle);
		margin-right: 6px;
	}
	.msg {
		margin: 0;
		padding: 4px 16px 14px;
		font-size: 13px;
	}
	.more {
		align-self: flex-start;
		margin: 8px 16px 12px;
		border: 0;
		background: none;
		padding: 0;
		font-size: 12px;
		color: var(--gold-ink);
		text-decoration: underline;
		cursor: pointer;
	}
	.err {
		color: var(--not-started);
		font-size: 12px;
	}
	@media (max-width: 900px) {
		.open {
			padding-left: 16px;
		}
	}
</style>
