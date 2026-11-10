<script lang="ts">
	import { microbotApi, MicrobotError, type Run } from '#lib/api/microbot';
	import { pluginRows, type PluginRow } from '#lib/progress/plugins';
	import { RUN_LABEL, runFinal, runnable, stoppable } from '#lib/progress/fazer';

	/*
	 * Fazer agora (doc 08, fase 4, entrega 2; artboard V6MicrobotFazer1440): o agent para o que
	 * estiver marcado (a atividade principal), roda um plugin até ele se desligar sozinho (o Cat
	 * Swap), volta ao lugar e religa. Começar e cancelar pedem um segundo clique e vão para a
	 * auditoria; enquanto roda, as outras ações do cockpit são recusadas pelo agent.
	 */
	let { loggedIn, onchange }: { loggedIn: boolean; onchange?: () => void } = $props();

	let rows = $state<PluginRow[]>([]);
	let run = $state<Run | null>(null);
	let pick = $state('');
	let unchecked = $state<Record<string, boolean>>({});
	let stay = $state(false);
	let armed = $state<'start' | 'cancel' | null>(null);
	let busy = $state(false);
	let error = $state<string | null>(null);
	let open = $state(false);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	const options = $derived(runnable(rows));
	// O escolhido, ou o primeiro da lista (o Cat Swap quando está carregado).
	$effect(() => {
		if (!options.some((o) => o.className === pick)) pick = options[0]?.className ?? '';
	});
	const target = $derived(pick);
	const stops = $derived(
		stoppable(rows, target).map((s) => ({
			...s,
			checked: unchecked[s.row.className] === undefined ? s.checked : !unchecked[s.row.className]
		}))
	);
	const going = $derived(!!run && !runFinal(run));

	async function loadPlugins() {
		try {
			rows = pluginRows(await microbotApi.plugins());
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	async function loadRun() {
		try {
			const was = going;
			run = (await microbotApi.run()).run;
			if (was && runFinal(run)) {
				void loadPlugins();
				onchange?.();
			}
		} catch {
			/* o estado do cliente já mostra o erro */
		}
	}
	$effect(() => {
		void loadPlugins();
		void loadRun();
	});
	// Enquanto roda, relê a cada 3 s.
	$effect(() => {
		if (!going) return;
		const t = setInterval(() => {
			if (!document.hidden) void loadRun();
		}, 3000);
		return () => clearInterval(t);
	});

	function arm(what: 'start' | 'cancel') {
		armed = what;
		clearTimeout(disarm);
		disarm = setTimeout(() => (armed = null), 4000);
	}
	async function start() {
		if (armed !== 'start') return arm('start');
		armed = null;
		busy = true;
		error = null;
		try {
			const r = await microbotApi.runStart({
				className: target,
				stop: stops.filter((s) => s.checked).map((s) => s.row.className),
				stay
			});
			run = r.result.run;
			open = true;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = false;
			onchange?.();
		}
	}
	async function cancel() {
		if (armed !== 'cancel') return arm('cancel');
		armed = null;
		try {
			await microbotApi.runCancel();
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
		await loadRun();
		onchange?.();
	}

	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const nameOf = (className: string) =>
		rows.find((r) => r.className === className)?.name ?? className.split('.').pop();
	const outcome = (r: Run) => {
		const res = r.result ?? {};
		const parts: string[] = [];
		if (typeof res.deathRunesGained === 'number')
			parts.push(`+${res.deathRunesGained} death runes`);
		if (res.kittenFollows === true) parts.push('filhote seguindo');
		return parts.join(' · ');
	};
</script>

<section class="panel">
	<div class="plaque">Fazer agora<span class="r">para, roda e volta</span></div>
	<div class="pad stack">
		{#if going && run}
			<div class="now">
				<span class="dot go" aria-hidden="true"></span>
				<span class="nm">{nameOf(run.className)}</span>
				<span class="st">{RUN_LABEL[run.state]}</span>
			</div>
			<ol class="trail">
				{#each run.events.slice(-6) as ev, i (i)}
					<li><span class="t">{hhmm(ev.at)}</span>{ev.text}</li>
				{/each}
			</ol>
			<button
				class="btn btn-s self"
				class:btn-d={armed === 'cancel'}
				type="button"
				onclick={cancel}
				title="Para o plugin que está rodando; o principal continua parado"
				>{armed === 'cancel' ? 'Confirmar: cancelar' : 'Cancelar'}</button
			>
		{:else}
			<label class="fld">
				<span class="k">Rodar</span>
				<select bind:value={pick} disabled={!options.length}>
					{#each options as o (o.className)}
						<option value={o.className}>{o.name}</option>
					{:else}
						<option value="">nenhum plugin seu ou do Hub</option>
					{/each}
				</select>
			</label>
			<div class="fld">
				<span class="k">Parar durante e religar depois</span>
				{#each stops as s (s.row.className)}
					<label class="chk">
						<input
							type="checkbox"
							checked={s.checked}
							onchange={(e) => (unchecked[s.row.className] = !e.currentTarget.checked)}
						/>{s.row.name}
						{#if /kitten/i.test(s.row.className)}<span
								class="su hint"
								title="Fica ligado: precisa ver a mensagem do filhote novo">fica ligado</span
							>{/if}
					</label>
				{:else}
					<span class="su sm0">Nada ligado para parar.</span>
				{/each}
			</div>
			<label class="chk">
				<input
					type="checkbox"
					checked={!stay}
					onchange={(e) => (stay = !e.currentTarget.checked)}
				/>
				Voltar ao lugar depois
			</label>
			<button
				class="btn btn-s self"
				class:btn-d={armed === 'start'}
				type="button"
				disabled={!loggedIn || !target || busy}
				onclick={start}
				title={loggedIn ? 'Age na conta: anda, troca e compra' : 'Precisa da conta logada'}
				>{armed === 'start' ? 'Confirmar: agir na conta' : 'Fazer agora'}</button
			>
		{/if}
		{#if error}<p class="err">{error}</p>{/if}
		{#if run && runFinal(run)}
			<div class="last">
				<button class="lnk" type="button" onclick={() => (open = !open)} aria-expanded={open}>
					Último: {nameOf(run.className)} ·
					<span class={run.state}>{RUN_LABEL[run.state]}</span>
					{#if run.finishedAt}{` às ${hhmm(run.finishedAt)}`}{/if}
				</button>
				{#if outcome(run)}<span class="su sm">{outcome(run)}</span>{/if}
				{#if run.error}<p class="err">{run.error}</p>{/if}
				{#if open}
					<ol class="trail">
						{#each run.events as ev, i (i)}
							<li><span class="t">{hhmm(ev.at)}</span>{ev.text}</li>
						{/each}
					</ol>
				{/if}
			</div>
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
	.fld {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	select {
		height: 30px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font-size: 12px;
		padding: 0 6px;
	}
	.chk {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
	}
	.hint {
		margin-left: auto;
		font-size: 11px;
	}
	.self {
		align-self: flex-start;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.now {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
	}
	.now .nm {
		flex: 1;
		font-weight: 600;
	}
	.st {
		white-space: nowrap;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--progress);
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--progress);
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
	.last {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding-top: 8px;
		border-top: 1px solid var(--hairline);
	}
	.lnk {
		border: 0;
		background: none;
		padding: 0;
		text-align: left;
		font-size: 12px;
		color: var(--ink);
		cursor: pointer;
	}
	.done {
		color: var(--done);
		font-weight: 600;
	}
	.failed,
	.canceled {
		color: var(--not-started);
		font-weight: 600;
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.sm0 {
		margin: 0;
		font-size: 12px;
	}
	.sm {
		font-size: 11px;
	}
</style>
