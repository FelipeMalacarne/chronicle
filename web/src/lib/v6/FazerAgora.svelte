<script lang="ts">
	import { microbotApi, MicrobotError, type Run, type Triggers } from '#lib/api/microbot';
	import { pluginRows, type PluginRow } from '#lib/progress/plugins';
	import {
		RUN_LABEL,
		firingText,
		isCatSwap,
		runFinal,
		runOutcome,
		runnable,
		stoppable
	} from '#lib/progress/fazer';

	/*
	 * Fazer agora (doc 08, fase 4, entrega 2; artboard V6MicrobotFazer1440): o agent para o que
	 * estiver marcado (a atividade principal), roda um plugin até ele se desligar sozinho (o Cat
	 * Swap), volta ao lugar e religa. Começar e cancelar pedem um segundo clique e vão para a
	 * auditoria; enquanto roda, as outras ações do cockpit são recusadas pelo agent. "Sozinho" (entrega
	 * 3; artboard V6MicrobotSozinho1440) salva as escolhas como gatilho: o agent faz o mesmo run quando
	 * o filhote cresce, mesmo com o cockpit fechado.
	 */
	let {
		loggedIn,
		onchange,
		bare = false
	}: {
		loggedIn: boolean;
		onchange?: () => void;
		/** Dentro de outro cartão (o provisório da Visão geral): sem o cartão próprio. */
		bare?: boolean;
	} = $props();

	let rows = $state<PluginRow[]>([]);
	let run = $state<Run | null>(null);
	let pick = $state('');
	let unchecked = $state<Record<string, boolean>>({});
	let stay = $state(false);
	let armed = $state<'start' | 'cancel' | 'auto' | null>(null);
	let triggers = $state<Triggers | null>(null);
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
	const grown = $derived(triggers?.triggers.find((t) => t.event === 'kitten_grown') ?? null);
	const auto = $derived(!!grown?.enabled);
	const fired = $derived(firingText(triggers?.last));

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
	async function loadTriggers() {
		try {
			triggers = await microbotApi.triggers();
		} catch {
			triggers = null;
		}
	}
	$effect(() => {
		void loadPlugins();
		void loadRun();
		void loadTriggers();
	});
	// Com o gatilho ligado, o run pode começar sozinho: relê a cada 15 s.
	$effect(() => {
		if (!auto || going) return;
		const t = setInterval(() => {
			if (document.hidden) return;
			void loadRun();
			void loadTriggers();
		}, 15000);
		return () => clearInterval(t);
	});
	// Enquanto roda, relê a cada 3 s.
	$effect(() => {
		if (!going) return;
		const t = setInterval(() => {
			if (!document.hidden) void loadRun();
		}, 3000);
		return () => clearInterval(t);
	});

	function arm(what: 'start' | 'cancel' | 'auto') {
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

	// Ligar salva as escolhas do formulário como o run do gatilho; desligar guarda o run salvo.
	async function setAuto(on: boolean) {
		if (on && armed !== 'auto') return arm('auto');
		armed = null;
		busy = true;
		error = null;
		try {
			const saved = on
				? {
						className: target,
						stop: stops.filter((s) => s.checked).map((s) => s.row.className),
						stay
					}
				: (grown?.run ?? { className: target });
			const r = await microbotApi.triggerSet({ event: 'kitten_grown', enabled: on, run: saved });
			triggers = r.result;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = false;
			onchange?.();
		}
	}

	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const nameOf = (className: string) =>
		rows.find((r) => r.className === className)?.name ?? className.split('.').pop();
	const outcome = runOutcome;
</script>

{#snippet body()}
	{#if going && run}
		<div class="now">
			<span class="dot go" aria-hidden="true"></span>
			<span class="nm"
				>{nameOf(run.className)}{#if run.trigger}<span class="su tag">sozinho</span>{/if}</span
			>
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
			<input type="checkbox" checked={!stay} onchange={(e) => (stay = !e.currentTarget.checked)} />
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
	<div class="auto">
		<div class="arow">
			<span class="k">Sozinho</span>
			{#if triggers && !triggers.watching}<span
					class="su hint"
					title={triggers.watchError ?? 'O agent não está lendo o chat do jogo'}
					>sem ler o chat</span
				>{:else if auto}<span class="on">ligado</span>{/if}
		</div>
		{#if auto && grown}
			<p class="sm0">
				Quando o filhote crescer: {nameOf(grown.run.className)}{grown.run.stop?.length
					? `, parando ${grown.run.stop.map(nameOf).join(', ')}`
					: ''}{grown.run.stay ? '' : ', e volta ao lugar'}.
			</p>
			<button
				class="btn btn-s self"
				type="button"
				disabled={busy}
				onclick={() => setAuto(false)}
				title="O agent para de trocar o gato sozinho">Desligar</button
			>
		{:else}
			<button
				class="btn btn-s self"
				class:btn-d={armed === 'auto'}
				type="button"
				disabled={!triggers || !isCatSwap(target) || busy || going}
				onclick={() => setAuto(true)}
				title={!isCatSwap(target)
					? 'Só o Cat Swap troca o gato'
					: 'Salva as escolhas acima: o agent faz isto sozinho quando aparecer "Your kitten has grown into a healthy cat"'}
				>{armed === 'auto'
					? 'Confirmar: agir sozinho'
					: 'Quando o filhote crescer, fazer isto'}</button
			>
		{/if}
		{#if fired && triggers?.last}<span class="su sm">{hhmm(triggers.last.at)} · {fired}</span>{/if}
	</div>
	{#if run && runFinal(run)}
		<div class="last">
			<button class="lnk" type="button" onclick={() => (open = !open)} aria-expanded={open}>
				<!-- One span: the button is a flex box, which drops the spaces between its items. -->
				<span
					>Último: {nameOf(run.className)}{run.trigger ? ' (sozinho)' : ''} ·
					<span class={run.state}>{RUN_LABEL[run.state]}</span
					>{#if run.finishedAt}{` às ${hhmm(run.finishedAt)}`}{/if}</span
				>
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
{/snippet}

{#if bare}
	<div class="bare stack">
		<p class="lbl">Fazer agora</p>
		{@render body()}
	</div>
{:else}
	<section class="card">
		<div class="h">
			<h2>Fazer agora</h2>
			<span>para, roda e volta</span>
		</div>
		<div class="pad stack">
			{@render body()}
		</div>
	</section>
{/if}

<style>
	.pad {
		padding: 12px;
	}
	.bare {
		padding-top: 10px;
		border-top: 1px solid var(--hairline);
	}
	.bare .lbl {
		margin: 0;
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
	.auto {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding-top: 8px;
		border-top: 1px solid var(--hairline);
	}
	.arow {
		display: flex;
		align-items: center;
	}
	.on {
		margin-left: auto;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--done);
	}
	.tag {
		margin-left: 6px;
		font-size: 11px;
		font-weight: 400;
	}
</style>
