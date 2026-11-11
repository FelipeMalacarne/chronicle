<script lang="ts">
	import { microbotApi, MicrobotError, type Run, type RunHistory } from '#lib/api/microbot';
	import { RUN_LABEL, runDuration, runFinal, runOutcome, runWhen } from '#lib/progress/fazer';

	/*
	 * Runs (doc 08, fase 4; artboard V6MicrobotRuns1440): os últimos runs do agent, o mais novo
	 * primeiro, com a trilha de cada um. O agent guarda o histórico na máquina do jogo, então ele
	 * vale com o cockpit fechado e depois de o agent reiniciar. Só lê; relê a cada 30 s e quando o
	 * painel Fazer agora avisa que um run terminou (version).
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
		!runFinal(r) ? 'going' : r.state === 'done' ? 'done' : r.state === 'canceled' ? 'off' : 'bad';
	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const short = (className: string) =>
		className
			.split('.')
			.pop()
			?.replace(/Plugin$/, '');
	const detail = (r: Run) => [runDuration(r), runOutcome(r)].filter(Boolean).join(' · ') || null;
</script>

<section class="panel">
	<div class="plaque">Runs<span class="r">últimos {data?.runs.length ?? ''}</span></div>
	<div class="pad">
		{#if error && !data}
			<p class="err">{error}</p>
		{:else if !data}
			<span class="su sm0" aria-busy="true">Lendo…</span>
		{:else}
			{#each runs as r (r.id)}
				<div class="run">
					<button
						class="row"
						type="button"
						aria-expanded={open === r.id}
						onclick={() => (open = open === r.id ? null : r.id)}
					>
						<span class="dot {tone(r)}" aria-hidden="true"></span>
						<span class="nm"
							>{short(r.className)}{#if r.trigger}<span class="tag">sozinho</span>{/if}</span
						>
						<span class="rs {tone(r)}">{RUN_LABEL[r.state]}</span>
						<span class="when">{runWhen(r.startedAt)}</span>
					</button>
					{#if detail(r)}<span class="su sm">{detail(r)}</span>{/if}
					{#if open === r.id}
						{#if r.error}<p class="err">{r.error}</p>{/if}
						<ol class="trail">
							{#each r.events as ev, i (i)}
								<li><span class="t">{hhmm(ev.at)}</span>{ev.text}</li>
							{/each}
						</ol>
					{/if}
				</div>
			{:else}
				<span class="su sm0">Nenhum run ainda.</span>
			{/each}
			{#if data.runs.length > FIRST}
				<button class="more" type="button" onclick={() => (all = !all)}
					>{all ? 'Ver menos' : `Ver todos (${data.runs.length})`}</button
				>
			{/if}
			{#if data.error}
				<p class="err" title={data.error}>O agent não conseguiu gravar o histórico.</p>
			{/if}
		{/if}
	</div>
</section>

<style>
	.pad {
		padding: 8px 12px 12px;
	}
	.run {
		display: flex;
		flex-direction: column;
		gap: 3px;
		padding: 6px 0;
		border-bottom: 1px solid var(--hairline);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
		border: 0;
		background: none;
		padding: 0;
		font: inherit;
		font-size: 12px;
		color: var(--ink);
		text-align: left;
		cursor: pointer;
	}
	.nm {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tag {
		margin-left: 6px;
		padding: 0 5px;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font-size: 10px;
		color: var(--ink-muted);
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
		background: var(--ink-subtle);
	}
	.dot.done {
		background: var(--done);
	}
	.dot.going {
		background: var(--progress);
	}
	.dot.bad {
		background: var(--not-started);
	}
	.rs {
		font-size: 11px;
		color: var(--ink-muted);
	}
	.rs.done {
		color: var(--done);
		font-weight: 600;
	}
	.rs.bad {
		color: var(--not-started);
		font-weight: 600;
	}
	.rs.going {
		color: var(--progress);
		font-weight: 600;
	}
	.when {
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.trail {
		margin: 2px 0 0 16px;
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
	.su {
		color: var(--ink-muted);
	}
	.sm {
		margin-left: 16px;
		font-size: 11px;
	}
	.more {
		margin-top: 8px;
		border: 0;
		background: none;
		padding: 0;
		font-size: 12px;
		color: var(--gold-ink);
		text-decoration: underline;
		cursor: pointer;
	}
	.err {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
</style>
