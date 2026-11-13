<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		fleetPluginsApi,
		microbotApi,
		MicrobotError,
		type MicrobotStatus,
		type RecurringStatus,
		type Run,
		type Triggers
	} from '#lib/api/microbot';
	import { fleet, loadFleet } from '#lib/progress/fleet.svelte';
	import { stoppable, runnable } from '#lib/progress/fazer';
	import { ICON_ATTRIBUTION, itemIcon } from '#lib/progress/icons';
	import { microbotModule, probeMicrobot } from '#lib/progress/microbot.svelte';
	import { pluginRows, type PluginRow } from '#lib/progress/plugins';
	import {
		accountRows,
		CLOCKS,
		doSteps,
		draftOf,
		draftProblem,
		EVENTS,
		fleetRoutines,
		fleetRuns,
		headline,
		intervalText,
		LIMITS,
		newDraft,
		pluginLabel,
		queueHead,
		runLine,
		savePlan,
		watcherLine,
		withEnabled,
		type Draft,
		type FleetRoutine,
		type RoutineRow
	} from '#lib/progress/rotinas';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Rotinas da frota (v7, fatia V6; artboard V7Rotinas1440). O servidor guarda as rotinas por
	 * conta (doc 11, seção 16.4): as recorrentes, pelo relógio do Time Tracking ou por intervalo, e
	 * os gatilhos, por evento do jogo. A tela junta as cópias de cada conta numa rotina da frota
	 * (pelo nome; os gatilhos pelo evento), com as contas em que ela vale; a Fila mostra o que cada
	 * conta tem pela frente. Ligar faz o agent agir sozinho no jogo, então nada nasce ligado, salvar
	 * nunca liga e ligar pede um segundo clique. Um run que falhou segura a rotina até ela ser salva
	 * de novo. Divergências do artboard: decisoes-canvas.md, seção 7.8.
	 */
	interface AccountData {
		recurring: RecurringStatus | null;
		triggers: Triggers | null;
		runs: Run[];
		status: MicrobotStatus | null;
		error: string | null;
	}

	let data = $state<Record<string, AccountData>>({});
	let loaded = $state(false);
	let now = $state(new Date());
	/** As linhas do cliente ligado de cada conta (nome e plugins que dá para rodar e parar). */
	let clientPlugins = $state<Record<string, PluginRow[]>>({});
	/** Os plugins dos perfis da frota (Plugins, V8): nome e classe sem cliente ligado. */
	let fleetPlugins = $state<{ className: string; name: string }[]>([]);
	let sel = $state<string | null>(null);
	let draft = $state<Draft | null>(null);
	let runAccount = $state('');
	let armed = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let note = $state<string | null>(null);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	const accounts = $derived(fleet.accounts ?? []);
	const rsn = (id: string) => accounts.find((a) => a.id === id)?.rsn ?? id;
	const message = (e: unknown) => (e instanceof MicrobotError ? e.message : String(e));
	const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

	async function loadAccount(id: string) {
		let error: string | null = null;
		const [recurring, triggers, runs, status] = await Promise.all([
			microbotApi.recurringOf(id).catch((e) => {
				error = message(e);
				return null;
			}),
			quiet(microbotApi.triggersOf(id)),
			quiet(microbotApi.runsOf(id)),
			quiet(microbotApi.statusOf(id))
		]);
		data[id] = { recurring, triggers, runs: runs?.runs ?? [], status, error };
		if (status?.agent.connected && !clientPlugins[id]) {
			const plugins = await quiet(microbotApi.pluginsOf(id));
			if (plugins) clientPlugins[id] = pluginRows(plugins);
		}
	}

	async function loadAll() {
		await Promise.all(accounts.map((a) => loadAccount(a.id)));
		now = new Date();
		loaded = true;
		if (!sel && !draft) {
			// A Fila da Visão geral abre uma rotina por ?rotina=<chave>.
			const want = routines.find((f) => f.key === page.url.searchParams.get('rotina'));
			if (want ?? routines[0]) open(want ?? routines[0]);
			else if (accounts.length) startNew();
		}
	}

	$effect(() => {
		void probeMicrobot();
		void loadFleet();
	});
	$effect(() => {
		if (!fleet.accounts || microbotModule.enabled !== true) return;
		void loadAll();
		void quiet(fleetPluginsApi.list()).then((v) => {
			fleetPlugins = (v?.plugins ?? [])
				.filter((p) => p.class_name && p.source !== 'core')
				.sort((a, b) => Number(b.group === 'routines') - Number(a.group === 'routines'))
				.map((p) => ({ className: p.class_name as string, name: p.title || p.name }));
		});
		// O vigia do agent olha a cada 30 s: a tela acompanha.
		const t = setInterval(() => {
			if (!document.hidden) void loadAll();
		}, 30_000);
		return () => clearInterval(t);
	});

	/** Nomes dos plugins pelos clientes ligados e pelos perfis ("Birdhouse Runner" no lugar da classe). */
	const names = $derived(
		new Map([
			...fleetPlugins.map((p) => [p.className, p.name] as [string, string]),
			...Object.values(clientPlugins)
				.flat()
				.map((r) => [r.className, r.name] as [string, string])
		])
	);
	function rowsOf(id: string): RoutineRow[] {
		const d = data[id];
		if (!d) return [];
		return accountRows(d.recurring, d.triggers, d.runs, now, names);
	}
	const groups = $derived(
		accounts.map((a) => ({ id: a.id, rsn: a.rsn, rows: rowsOf(a.id), d: data[a.id] }))
	);
	const routines = $derived(fleetRoutines(groups.map((g) => ({ account: g.id, rows: g.rows }))));
	const current = $derived(sel ? (routines.find((f) => f.key === sel) ?? null) : null);
	/** As contas em que o último run segura todas as rotinas: qualquer salvamento ali solta. */
	const heldAgent = $derived(
		new Set(groups.filter((g) => g.rows.some((r) => r.held === 'held_agent')).map((g) => g.id))
	);

	/** As rotinas seguras: um aviso por rotina, ou um por conta quando é o último run dela. */
	const holds = $derived(
		groups.flatMap((g) => {
			const own = g.rows.filter((r) => r.held === 'held');
			const agent = g.rows.find((r) => r.held === 'held_agent');
			const runs = g.d?.runs ?? [];
			const out = own.map((r) => ({
				account: g.id,
				row: r,
				run: runs.find((x) => x.routine === r.id)
			}));
			// Quando o último run da conta é o de uma rotina segura, liberar ela libera todas: um aviso só.
			const latest = runs[0];
			if (agent && !own.some((r) => r.id === latest?.routine))
				out.push({ account: g.id, row: agent, run: latest });
			return out;
		})
	);

	const live = (id: string) => !!data[id]?.status?.agent.connected;
	const loggedIn = (id: string) => !!data[id]?.status?.client?.state?.loggedIn;
	/** A conta cujo cliente dá a lista de plugins: a primeira marcada que tem um ligado. */
	const formAccount = $derived(
		draft?.accounts.find((a) => clientPlugins[a]) ?? draft?.accounts[0] ?? ''
	);
	const formRows = $derived(formAccount ? (clientPlugins[formAccount] ?? null) : null);

	/** O que a rotina pode rodar: os plugins do cliente ligado ou, sem ele, os dos perfis e os que as rotinas já rodam. */
	const runOptions = $derived.by(() => {
		const out: { className: string; name: string }[] = formRows
			? runnable(formRows).map((r) => ({ className: r.className, name: r.name }))
			: [...fleetPlugins];
		const extra = formRows ? [] : groups.flatMap((g) => g.rows.map((r) => r.run.className));
		for (const c of [draft?.className ?? '', ...extra]) {
			if (c && !out.some((o) => o.className === c))
				out.push({ className: c, name: pluginLabel(c, names) });
		}
		return out;
	});
	/** O que parar durante o run: os plugins ligados no cliente e os que a rotina já para. */
	const stopOptions = $derived.by(() => {
		if (!draft) return [];
		const out = formRows
			? stoppable(formRows, draft.className).map((s) => ({
					className: s.row.className,
					name: s.row.name
				}))
			: [];
		for (const c of draft.stop) {
			if (c !== draft.className && !out.some((o) => o.className === c))
				out.push({ className: c, name: pluginLabel(c, names) });
		}
		return out;
	});
	const steps = $derived(draft ? doSteps(draft, names) : []);
	/** O último passo, "Religa ...", quando o run para alguma coisa. */
	const restart = $derived(
		draft && draft.stop.some((c) => c !== draft?.className) ? steps[steps.length - 1] : null
	);
	const lastRuns = $derived(
		current ? fleetRuns(current, (a) => data[a]?.runs ?? []).slice(0, 5) : []
	);
	const hasTrigger = (id: string, event: string) =>
		!!data[id]?.triggers?.triggers.some((t) => t.event === event);
	/** O que falta para salvar: o do formulário, um nome que já é de outra rotina, um gatilho que a conta já tem. */
	const problem = $derived.by(() => {
		if (!draft) return null;
		const d = draft;
		const own = draftProblem(d);
		if (own) return own;
		if (d.kind === 'event') {
			const has = current ? [] : d.accounts.filter((a) => hasTrigger(a, d.event));
			return has.length
				? `${has.map(rsn).join(', ')} já ${has.length === 1 ? 'tem' : 'têm'} este gatilho: abra o dele na lista.`
				: null;
		}
		const key = `recurring:${d.name.trim()}`;
		return key !== current?.key && routines.some((f) => f.key === key)
			? 'Já existe uma rotina com esse nome: abra a dela na lista para pôr em outra conta.'
			: null;
	});
	const plan = $derived(draft && !problem ? savePlan(draft, current, heldAgent) : null);

	function arm(what: string) {
		armed = what;
		clearTimeout(disarm);
		disarm = setTimeout(() => (armed = null), 4000);
	}

	function open(fr: FleetRoutine) {
		sel = fr.key;
		draft = draftOf(fr);
		runAccount =
			fr.members.find((m) => live(m.account) && loggedIn(m.account))?.account ??
			fr.members[0].account;
		note = null;
		armed = null;
	}
	function openRow(account: string, r: RoutineRow) {
		const fr = routines.find((f) =>
			f.members.some((m) => m.account === account && m.row.key === r.key)
		);
		if (fr) open(fr);
	}
	function startNew() {
		sel = null;
		draft = newDraft(accounts.length === 1 ? [accounts[0].id] : []);
		note = null;
		armed = null;
	}

	/** Numa rotina nova, escolher o plugin marca para parar o que estiver ligado (como o Fazer agora). */
	function pick(className: string) {
		if (!draft) return;
		draft.className = className;
		if (!current && formRows) {
			draft.stop = stoppable(formRows, className)
				.filter((s) => s.checked)
				.map((s) => s.row.className);
		} else {
			draft.stop = draft.stop.filter((c) => c !== className);
		}
	}
	function setStop(className: string, on: boolean) {
		if (!draft) return;
		draft.stop = on
			? [...draft.stop.filter((c) => c !== className), className]
			: draft.stop.filter((c) => c !== className);
	}
	function setAccount(id: string, on: boolean) {
		if (!draft) return;
		draft.accounts = on
			? [...draft.accounts.filter((a) => a !== id), id]
			: draft.accounts.filter((a) => a !== id);
	}
	const memberOf = (fr: FleetRoutine | null, account: string) =>
		fr?.members.find((m) => m.account === account)?.row;

	async function act(label: string, fn: () => Promise<void>) {
		if (busy) return;
		busy = label;
		note = null;
		try {
			await fn();
		} catch (e) {
			note = message(e);
		} finally {
			busy = null;
		}
	}

	/** Grava uma cópia como está, só com outro `enabled` (salvar de novo solta a trava dela). */
	async function putEnabled(account: string, r: RoutineRow, enabled: boolean) {
		if (r.kind === 'recurring' && r.recurring) {
			await microbotApi.recurringSave(account, withEnabled(r.recurring, enabled));
		} else if (r.trigger) {
			await microbotApi.triggerSetOf(account, { ...r.trigger, enabled });
		}
	}

	/**
	 * O interruptor vale para todas as contas da rotina. Ligar faz o agent agir sozinho: pede o
	 * segundo clique. Desligar é na hora, menos numa conta em que o último run segura as outras:
	 * gravar ali solta a trava delas.
	 */
	function toggle(fr: FleetRoutine) {
		const target = fr.on < fr.members.length;
		const change = fr.members.filter((m) => m.row.enabled !== target);
		const key = `on:${fr.key}`;
		const frees = !target && change.some((m) => heldAgent.has(m.account));
		if ((target || frees) && armed !== key) return arm(key);
		armed = null;
		void act(key, async () => {
			const failed: string[] = [];
			for (const m of change) {
				try {
					await putEnabled(m.account, m.row, target);
				} catch (e) {
					failed.push(`${rsn(m.account)}: ${message(e)}`);
				}
			}
			await Promise.all(change.map((m) => loadAccount(m.account)));
			if (failed.length) note = failed.join(' · ');
		});
	}

	/** Salvar de novo solta a trava; ligada, ela pode começar logo, então pede o segundo clique. */
	function release(account: string, r: RoutineRow) {
		const key = `release:${account}:${r.key}`;
		if (armed !== key) return arm(key);
		armed = null;
		void act(key, async () => {
			await putEnabled(account, r, r.enabled);
			await loadAccount(account);
		});
	}

	const saveLabel = $derived.by(() => {
		if (!plan) return 'Salvar';
		const out = plan.removes.length;
		const parts = [
			plan.releases.length ? 'liberar' : '',
			out ? `tirar de ${out} ${out === 1 ? 'conta' : 'contas'}` : ''
		].filter(Boolean);
		return parts.length ? `Confirmar: salvar e ${parts.join(' e ')}` : 'Salvar';
	});

	function save() {
		const d = draft;
		const p = plan;
		const before = current;
		if (!d || !p) return;
		if ((p.releases.length || p.removes.length) && armed !== 'save') return arm('save');
		armed = null;
		void act('save', async () => {
			const failed: string[] = [];
			for (const put of p.puts) {
				try {
					if (put.trigger) await microbotApi.triggerSetOf(put.account, put.trigger);
					else if (put.recurring) await microbotApi.recurringSave(put.account, put.recurring);
				} catch (e) {
					failed.push(`${rsn(put.account)}: ${message(e)}`);
				}
			}
			for (const rm of p.removes) {
				try {
					await microbotApi.recurringDelete(rm.account, rm.id);
				} catch (e) {
					failed.push(`${rsn(rm.account)}: ${message(e)}`);
				}
			}
			const touched = new Set([
				...p.puts.map((x) => x.account),
				...p.removes.map((x) => x.account)
			]);
			await Promise.all([...touched].map((a) => loadAccount(a)));
			const key = d.kind === 'event' ? `trigger:${d.event}` : `recurring:${d.name.trim()}`;
			const saved = routines.find((f) => f.key === key);
			if (saved) open(saved);
			const fresh = p.puts.filter((x) => !memberOf(before, x.account)).length;
			note = failed.length
				? `Não salvou em ${failed.join(' · ')}`
				: p.puts.length > 1
					? `Salva em ${p.puts.length} contas.`
					: 'Salva.';
			if (!failed.length && fresh && !before?.on)
				note += ' Desligada: ligue na lista quando quiser que rode sozinha.';
			else if (!failed.length && fresh) note += ' Nas contas novas, nasce desligada.';
		});
	}

	function remove() {
		const fr = current;
		if (!fr || fr.kind !== 'recurring') return;
		if (armed !== 'remove') return arm('remove');
		armed = null;
		void act('remove', async () => {
			const failed: string[] = [];
			for (const m of fr.members) {
				try {
					if (m.row.id) await microbotApi.recurringDelete(m.account, m.row.id);
				} catch (e) {
					failed.push(`${rsn(m.account)}: ${message(e)}`);
				}
			}
			await Promise.all(fr.members.map((m) => loadAccount(m.account)));
			sel = null;
			draft = null;
			note = failed.length
				? `Não excluiu em ${failed.join(' · ')}`
				: 'Excluída; os runs dela ficam no histórico.';
		});
	}

	/** Rodar agora o que a rotina salva roda, numa conta, pelo Fazer agora do agent (para, roda, volta e religa). */
	function runNow() {
		const r = memberOf(current, runAccount);
		if (!r) return;
		if (armed !== 'run') return arm('run');
		armed = null;
		const account = runAccount;
		void act('run', async () => {
			await microbotApi.runStartOf(account, r.run);
			await loadAccount(account);
			note = `Começou em ${rsn(account)}: acompanhe em Últimas.`;
		});
	}

	function iconOf(r: { clock?: string }): string | undefined {
		return itemIcon(CLOCKS.find((c) => c.key === r.clock)?.item);
	}
	function lamp(id: string): string {
		const tone = fleet.tones[id];
		if (tone) return tone;
		return live(id) && loggedIn(id) ? 'ok' : 'off';
	}
	function stateText(fr: FleetRoutine): string {
		const n = fr.members.length;
		if (!fr.on) return 'desligada';
		if (fr.on < n) return `ligada em ${fr.on} de ${n}`;
		return n > 1 ? `ligada em ${n} contas` : 'ligada';
	}
	const dirty = $derived.by(() => {
		if (!draft || !current) return true;
		return JSON.stringify(draftOf(current)) !== JSON.stringify($state.snapshot(draft));
	});
	// A rotina aberta sumiu (excluída em outra aba ou pelo agent): o formulário vai junto.
	$effect(() => {
		if (loaded && sel && !current && !busy) {
			sel = null;
			draft = null;
		}
	});
	const canRun = $derived(!!runAccount && live(runAccount) && loggedIn(runAccount));
</script>

<svelte:head><title>Rotinas · Chronicle</title></svelte:head>

<header class="top">
	<div class="ttl">
		<h1 class="sr">Rotinas</h1>
		<p class="mu">
			As rotinas entram na fila de cada conta pelo relógio do Time Tracking, por um intervalo ou por
			um evento do jogo, e devolvem a conta ao que fazia.
		</p>
	</div>
	<button class="btn btn-p" type="button" disabled={!accounts.length} onclick={startNew}
		><Glyph name="plus" size={14} />Nova rotina</button
	>
</header>

{#if microbotModule.enabled === false}
	<p class="mu">O controle do Microbot está desligado neste servidor.</p>
{:else}
	<div class="cols">
		<div class="list">
			{#each holds as h (`${h.account}:${h.row.key}:${h.row.held}`)}
				{@const key = `release:${h.account}:${h.row.key}`}
				<div class="need7">
					<Glyph name="alert" size={18} />
					{#if h.row.held === 'held'}
						<p>
							<b>{rsn(h.account)}</b>: {h.row.name}
							{h.run?.state === 'canceled' ? 'foi cancelada' : 'falhou'}
						</p>
						<p class="s">
							{#if h.run?.error}{h.run.error}.
							{/if}Fica parada até você salvá-la de novo.
						</p>
					{:else}
						<p><b>{rsn(h.account)}</b>: o último run falhou</p>
						<p class="s">
							{#if h.run}{pluginLabel(h.run.className, names)}{h.run.error
									? `: ${h.run.error}`
									: ''}.
							{/if}As rotinas da conta esperam até você salvar uma delas.
						</p>
					{/if}
					<div class="a">
						<button
							class="btn btn-s"
							class:btn-d={armed === key}
							type="button"
							disabled={!!busy}
							title="Salva a rotina de novo como está; ligada, ela pode começar logo"
							onclick={() => release(h.account, h.row)}
							>{armed === key ? 'Confirmar: liberar' : 'Liberar'}</button
						>
						<button class="btn btn-s btn-g" type="button" onclick={() => openRow(h.account, h.row)}
							>Abrir a rotina</button
						>
					</div>
				</div>
			{/each}

			<section>
				<div class="grp7">
					<h2>Fila</h2>
					<span>o que cada conta tem pela frente</span>
				</div>
				{#each groups as g (g.id)}
					{@const head = queueHead(g.rows)}
					{@const line = g.d?.error
						? 'não deu para ler as rotinas'
						: watcherLine(g.d?.recurring ?? null, now)}
					<div class="job">
						{#if head && iconOf(head)}<img src={iconOf(head)} alt="" />{:else}<span></span>{/if}
						<span class="n"
							><span class="lamp {lamp(g.id)}"></span>
							<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + query da conta -->
							<a href={`${resolve('/visao')}?account=${encodeURIComponent(g.id)}`}><b>{g.rsn}</b></a
							>
							<!-- eslint-enable svelte/no-navigation-without-resolve -->
						</span>
						<span class="q">
							{#if head}
								<button
									class="nx"
									type="button"
									title={head.why}
									onclick={() => openRow(g.id, head)}
									><b>{head.name}</b> <span class="who7 {head.tone}">{head.when}</span></button
								>
							{:else}
								<span class="su"
									>{!g.d
										? 'lendo…'
										: g.rows.length
											? 'nada com hora marcada'
											: 'nenhuma rotina'}</span
								>
							{/if}
							{#if line}<span
									class="ln"
									class:bad={!!g.d?.error}
									title={g.d?.error ??
										g.d?.recurring?.watchError ??
										g.d?.recurring?.clockError ??
										undefined}>{line}</span
								>{/if}
						</span>
						<span class="tag"
							>{g.rows.length
								? `${g.rows.length} ${g.rows.length === 1 ? 'rotina' : 'rotinas'}`
								: 'nenhuma'}</span
						>
					</div>
				{/each}
			</section>

			<section>
				<div class="grp7">
					<h2>Rotinas</h2>
					<span>entram na fila sozinhas</span>
				</div>
				{#each routines as fr (fr.key)}
					{@const on = sel === fr.key}
					{@const top = headline(fr)}
					{@const armKey = `on:${fr.key}`}
					{@const all = fr.on === fr.members.length}
					<div class="rt7" class:on class:dis={!fr.on}>
						<button class="pick" type="button" onclick={() => open(fr)} aria-pressed={on}>
							<span class="ico">
								{#if iconOf(fr)}<img src={iconOf(fr)} alt="" />{:else}<Glyph
										name={fr.kind === 'trigger' ? 'eye' : 'clock'}
									/>{/if}
							</span>
							<span class="x"><b>{fr.name}</b><span>{fr.line}</span></span>
							<span class="who7c">
								<span class="who7"
									>{#each fr.members as m (m.account)}<span
											class="lamp {lamp(m.account)}"
											title="{rsn(m.account)}: {m.row.when || m.row.why}"
										></span>{/each}{fr.members.length}
									{fr.members.length === 1 ? 'conta' : 'contas'}</span
								>
								{#if top?.row.when}<span
										class="who7 {top.row.tone}"
										title="{rsn(top.account)}: {top.row.why}"
										>{fr.members.length > 1 ? `${rsn(top.account)} · ` : ''}{top.row.when}</span
									>{/if}
							</span>
						</button>
						{#if armed === armKey}
							<button
								class="btn btn-s btn-d conf"
								type="button"
								disabled={!!busy}
								onclick={() => toggle(fr)}
								title={all
									? 'Desligar grava a rotina de novo, e isso solta a trava das outras rotinas da conta'
									: 'Ligada, o agent começa o run sozinho no jogo'}
								>{all
									? 'Confirmar: desligar'
									: fr.members.length > 1
										? `Confirmar: ligar em ${fr.members.length - fr.on}`
										: 'Confirmar: ligar'}</button
							>
						{:else}
							<button
								class="sw"
								class:on={all}
								class:part={fr.on > 0 && !all}
								type="button"
								role="switch"
								aria-checked={all}
								aria-label="{fr.name}: {stateText(fr)}"
								title={all
									? 'Desligar: ela para de começar sozinha'
									: fr.on
										? `${stateText(fr)}: ligar nas outras`
										: 'Ligar: o agent começa o run sozinho no jogo'}
								disabled={!!busy}
								onclick={() => toggle(fr)}
							></button>
						{/if}
					</div>
				{/each}
				{#if loaded && !routines.length}
					<p class="mu empty">Nenhuma rotina ainda. Comece por Nova rotina.</p>
				{:else if !loaded}
					<p class="mu empty" aria-busy="true">Lendo as rotinas da frota…</p>
				{/if}
			</section>
		</div>

		{#if draft}
			{@const d = draft}
			<aside class="card">
				<div class="h">
					<h2>
						{#if d.kind === 'clock' && iconOf({ clock: d.clock })}<img
								class="ico-h"
								src={iconOf({ clock: d.clock })}
								alt=""
							/>{/if}{current ? current.name : 'Nova rotina'}
					</h2>
					<span>{current ? stateText(current) : 'nasce desligada'}</span>
				</div>
				<div class="b">
					{#each current?.members.filter((m) => m.row.held) ?? [] as m (m.account)}
						{@const key = `release:${m.account}:${m.row.key}`}
						<div class="warn">
							<Glyph name="alert" />
							<span
								>{current && current.members.length > 1 ? `${rsn(m.account)}: ` : ''}{m.row
									.why}</span
							>
							<button
								class="btn btn-s"
								class:btn-d={armed === key}
								type="button"
								disabled={!!busy}
								onclick={() => release(m.account, m.row)}
								>{armed === key ? 'Confirmar: liberar' : 'Liberar'}</button
							>
						</div>
					{/each}
					{#if current?.differs}
						<div class="warn">
							<Glyph name="alert" /><span
								>As contas têm esta rotina com ajustes diferentes; aqui está a de {rsn(
									current.members[0].account
								)}, e salvar põe ela em todas.</span
							>
						</div>
					{/if}

					{#if d.kind !== 'event'}
						<div class="rule">
							<span class="k">Nome</span>
							<input
								class="in7 wide"
								type="text"
								maxlength={LIMITS.nameMax}
								placeholder="Bird houses"
								aria-label="Nome da rotina"
								bind:value={d.name}
							/>
						</div>
					{/if}

					<div class="rule">
						<span class="k">Quando</span>
						<div class="col">
							<div class="sent">
								<select
									class="pick7"
									aria-label="Tipo de rotina"
									bind:value={d.kind}
									disabled={current?.kind === 'trigger'}
								>
									<option value="clock" disabled={current?.kind === 'trigger'}>relógio</option>
									<option value="interval" disabled={current?.kind === 'trigger'}>intervalo</option>
									<option value="event" disabled={current?.kind === 'recurring'}>evento</option>
								</select>
								{#if d.kind === 'clock'}
									<select class="pick7" aria-label="Relógio" bind:value={d.clock}>
										{#each CLOCKS as c (c.key)}<option value={c.key}>{c.label}</option>{/each}
									</select>
									<span>ficar pronto</span>
								{:else if d.kind === 'interval'}
									<span>a cada</span>
									<input
										class="in7"
										type="number"
										min={LIMITS.intervalMin}
										max={LIMITS.intervalMax}
										aria-label="Intervalo em minutos"
										bind:value={d.interval}
									/>
									<span>min depois do último run</span>
								{:else}
									<select
										class="pick7"
										aria-label="Evento"
										bind:value={d.event}
										disabled={!!current}
									>
										{#each EVENTS as e (e.key)}<option value={e.key}>{e.label}</option>{/each}
									</select>
								{/if}
							</div>
							{#if d.kind === 'clock'}
								<div class="sent sub">
									<span>sem leitura do relógio, a cada</span>
									<input
										class="in7"
										type="number"
										min={LIMITS.intervalMin}
										max={LIMITS.intervalMax}
										placeholder="—"
										aria-label="Intervalo sem leitura do relógio, em minutos"
										bind:value={d.interval}
									/>
									<span>min</span>
								</div>
							{/if}
							{#if d.interval && d.interval >= 60}<span class="su sm"
									>= {intervalText(d.interval)}</span
								>{/if}
						</div>
					</div>

					{#if d.kind !== 'event'}
						<div class="rule">
							<span class="k">Ordem</span>
							<div class="sent">
								<span>prioridade</span>
								<input
									class="in7"
									type="number"
									min="0"
									max={LIMITS.priorityMax}
									aria-label="Prioridade"
									bind:value={d.priority}
								/>
								<span class="su" title="Com várias prontas, a de maior prioridade entra primeiro"
									>a maior entra primeiro</span
								>
							</div>
						</div>
					{/if}

					<div class="rule">
						<span class="k">Faz</span>
						<div class="col">
							<div class="ord">
								<span class="no">1</span>
								<div class="col">
									<span>{steps[0]}</span>
									{#if stopOptions.length}
										<div class="chips">
											{#each stopOptions as o (o.className)}
												<label class="acc7" class:on={d.stop.includes(o.className)}>
													<input
														type="checkbox"
														checked={d.stop.includes(o.className)}
														onchange={(e) => setStop(o.className, e.currentTarget.checked)}
													/>{o.name}
												</label>
											{/each}
										</div>
									{/if}
								</div>
								<span class="w"></span>
							</div>
							<div class="ord">
								<span class="no">2</span>
								<div class="sent">
									<span>Roda</span>
									<select
										class="pick7"
										aria-label="Plugin que a rotina roda"
										value={d.className}
										onchange={(e) => pick(e.currentTarget.value)}
									>
										<option value="" disabled>escolha o plugin</option>
										{#each runOptions as o (o.className)}<option value={o.className}
												>{o.name}</option
											>{/each}
									</select>
									<span>até terminar</span>
								</div>
								<span class="w"
									>limite <input
										class="in7 tiny"
										type="number"
										min={LIMITS.timeoutMin}
										max={LIMITS.timeoutMax}
										placeholder="30"
										aria-label="Limite do plugin em minutos"
										bind:value={d.timeout}
									/> min</span
								>
							</div>
							<label class="ord">
								<span class="no">3</span>
								<span class="chk"
									><input type="checkbox" bind:checked={d.back} />Volta ao lugar de antes</span
								>
								<span class="w"></span>
							</label>
							{#if restart}
								<div class="ord">
									<span class="no">4</span>
									<span>{restart}</span>
									<span class="w"></span>
								</div>
							{/if}
							{#if !formRows && formAccount}
								<span class="su sm"
									>{live(formAccount)
										? 'Lendo os plugins do cliente…'
										: 'Sem um cliente ligado, a lista traz os plugins dos perfis e os que as rotinas já rodam.'}</span
								>
							{/if}
						</div>
					</div>

					{#if d.kind !== 'event'}
						<div class="rule">
							<span class="k">Se falhar</span>
							<div class="sent">
								<span
									title="Um run que falhou ou foi cancelado segura a rotina naquela conta; se foi o último run da conta, as outras rotinas dela esperam também"
									>fica parada na conta até você salvá-la de novo</span
								>
							</div>
						</div>
					{/if}

					<div class="rule">
						<span class="k">Contas</span>
						<div class="chips">
							{#each accounts as a (a.id)}
								{@const m = memberOf(current, a.id)}
								{@const locked = d.kind === 'event' && !!m}
								{@const taken = d.kind === 'event' && !m && hasTrigger(a.id, d.event)}
								<label
									class="acc7"
									class:on={d.accounts.includes(a.id)}
									title={locked
										? 'Um gatilho não se exclui: desligue no interruptor'
										: taken
											? 'Já tem este gatilho: abra o dela na lista'
											: m
												? m.why
												: undefined}
									><input
										type="checkbox"
										checked={d.accounts.includes(a.id)}
										disabled={locked || taken}
										onchange={(e) => setAccount(a.id, e.currentTarget.checked)}
									/>{a.rsn}{#if m && m.when}<span class="when {m.tone}">{m.when}</span>{/if}</label
								>
							{/each}
						</div>
					</div>

					{#if current}
						<div class="rule">
							<span class="k">Últimas</span>
							<div class="runs">
								{#each lastRuns as x (`${x.account}:${x.run.uid ?? x.run.id}`)}
									{@const l = runLine(x.run, now)}
									<span class="rn"
										><span class="mono su">{l.at}</span><span class="tx" title={l.text}
											>{current.members.length > 1 ? `${rsn(x.account)} · ` : ''}{l.text ||
												l.result}</span
										><span class="res {l.tone}">{l.result}</span></span
									>
								{:else}
									<span class="su">Nenhum run ainda.</span>
								{/each}
							</div>
						</div>
					{/if}

					<div class="acts">
						<button
							class="btn btn-p"
							class:btn-d={armed === 'save'}
							type="button"
							disabled={!!busy || !plan || (!!current && !dirty)}
							title={problem ?? undefined}
							onclick={save}>{armed === 'save' ? saveLabel : 'Salvar'}</button
						>
						{#if current}
							{#if current.members.length > 1}
								<select class="pick7" aria-label="Conta em que rodar agora" bind:value={runAccount}>
									{#each current.members as m (m.account)}<option value={m.account}
											>{rsn(m.account)}</option
										>{/each}
								</select>
							{/if}
							<button
								class="btn"
								class:btn-d={armed === 'run'}
								type="button"
								disabled={!!busy || !canRun}
								title={!live(runAccount)
									? 'Precisa do cliente da conta ligado'
									: !loggedIn(runAccount)
										? 'Precisa da conta logada'
										: 'Para o que estiver marcado, roda o plugin, volta e religa'}
								onclick={runNow}
								><Glyph name="play" size={14} />{armed === 'run'
									? `Confirmar: agir em ${rsn(runAccount)}`
									: 'Rodar agora'}</button
							>
							{#if current.kind === 'recurring'}
								<button
									class="btn btn-g"
									class:btn-d={armed === 'remove'}
									type="button"
									disabled={!!busy}
									onclick={remove}
									>{armed === 'remove'
										? current.members.length > 1
											? `Confirmar: excluir das ${current.members.length}`
											: 'Confirmar: excluir'
										: 'Excluir'}</button
								>
							{/if}
						{:else}
							<button
								class="btn btn-g"
								type="button"
								onclick={() => {
									draft = null;
									note = null;
								}}>Descartar</button
							>
						{/if}
					</div>
					{#if note}<p class="note">{note}</p>{/if}
					{#if problem && (d.name || d.className || d.accounts.length)}<p class="su sm">
							{problem}
						</p>{/if}
				</div>
			</aside>
		{/if}
	</div>
{/if}

<p class="foot">{ICON_ATTRIBUTION}. Cliente de terceiros, fora do planner.</p>

<style>
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 10px 16px;
	}
	.ttl {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin-right: auto;
	}
	.ttl h1 {
		font-size: 30px;
	}
	.ttl p {
		margin: 0;
		font-size: 13px;
	}
	.top .btn :global(svg) {
		width: 14px;
		height: 14px;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 28px;
		align-items: flex-start;
	}
	.list {
		flex: 999 1 520px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.grp7 {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 0 2px 8px;
		border-bottom: 2px solid var(--frame);
	}
	.grp7 h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.grp7 span {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.job {
		display: grid;
		grid-template-columns: 28px 170px minmax(0, 1fr) 96px;
		gap: 12px;
		align-items: center;
		min-height: 48px;
		padding: 4px 6px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.job img {
		width: 24px;
		height: 24px;
		image-rendering: pixelated;
		object-fit: contain;
		justify-self: center;
	}
	.job .n {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.job .n a {
		min-width: 0;
		color: var(--ink);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.job .n a:hover {
		text-decoration: underline;
	}
	.job .n b {
		font-weight: 600;
	}
	.q {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.nx {
		display: inline-flex;
		align-items: baseline;
		gap: 8px;
		min-width: 0;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--ink);
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
		text-align: left;
		cursor: pointer;
	}
	.nx b {
		font-weight: 600;
	}
	.nx:hover b {
		text-decoration: underline;
	}
	.q .ln {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.q .ln.bad {
		color: var(--not-started);
	}
	.tag {
		display: inline-flex;
		align-items: center;
		justify-self: start;
		height: 18px;
		padding: 0 6px;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		white-space: nowrap;
	}
	.who7c {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 3px;
	}
	.who7c .who7:first-child {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.when {
		font:
			400 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.when.due {
		color: var(--gold-ink);
		font-weight: 600;
	}
	.when.held {
		color: var(--not-started);
		font-weight: 600;
	}
	.lamp {
		display: inline-block;
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		background: var(--ink-subtle);
		box-sizing: border-box;
	}
	.lamp.ok {
		background: var(--done);
		box-shadow: 0 0 0 3px var(--done-tint);
	}
	.lamp.up {
		background: var(--auto);
	}
	.lamp.rest {
		background: var(--gold);
	}
	.lamp.bad {
		background: var(--not-started);
	}
	.lamp.off,
	.lamp.unk {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	.rt7 {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 12px;
		align-items: center;
		min-height: 60px;
		padding: 0 10px 0 0;
		border-radius: 5px;
		border: 1px solid transparent;
		border-bottom-color: var(--hairline);
	}
	.rt7:hover {
		border-color: var(--border);
	}
	.rt7.on {
		background: var(--gold-tint);
		border-color: var(--gold-ink);
	}
	.rt7 .pick {
		display: grid;
		grid-template-columns: 28px minmax(0, 1fr) auto;
		gap: 12px;
		align-items: center;
		min-height: 58px;
		padding: 6px 0 6px 10px;
		border: 0;
		background: transparent;
		color: var(--ink);
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
		text-align: left;
		cursor: pointer;
	}
	.ico {
		width: 24px;
		height: 24px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		justify-self: center;
		color: var(--ink-subtle);
	}
	.ico img {
		width: 24px;
		height: 24px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.x {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.x b {
		font-weight: 600;
	}
	.x span {
		font-size: 12px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rt7.dis .x b,
	.rt7.dis .ico {
		opacity: 0.55;
	}
	.who7 {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.who7.due {
		color: var(--gold-ink);
		font-weight: 600;
	}
	.who7.held {
		color: var(--not-started);
		font-weight: 600;
	}
	.sw {
		position: relative;
		width: 40px;
		height: 22px;
		border-radius: 11px;
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
		border: 0;
		padding: 0;
		cursor: pointer;
		flex: 0 0 auto;
	}
	.sw::after {
		content: '';
		position: absolute;
		top: 3px;
		left: 3px;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--on-frame-muted);
	}
	.sw.on {
		background: var(--done);
	}
	.sw.on::after {
		left: 21px;
		background: #fff;
	}
	.sw.part {
		background: var(--gold);
	}
	.sw.part::after {
		left: 12px;
		background: #fff;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.conf {
		white-space: nowrap;
	}
	.empty {
		margin: 12px 2px 0;
		font-size: 13px;
	}
	.need7 {
		display: grid;
		grid-template-columns: 22px minmax(0, 1fr);
		gap: 4px 12px;
		align-items: center;
		padding: 14px 16px;
		border-radius: 6px;
		border: 1px solid var(--stale);
		background: var(--surface);
		box-shadow: var(--bevel);
		font-size: 13px;
	}
	.need7 > :global(svg) {
		color: var(--stale);
		grid-row: 1 / 3;
		align-self: start;
		margin-top: 2px;
	}
	.need7 p {
		margin: 0;
		line-height: 1.4;
	}
	.need7 p b {
		font-weight: 600;
	}
	.need7 .s {
		grid-column: 2;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.need7 .a {
		grid-column: 2;
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.card {
		display: flex;
		flex-direction: column;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		min-width: 0;
	}
	aside.card {
		flex: 1 1 400px;
		max-width: 480px;
	}
	.card > .h {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 12px 16px 0;
	}
	.card > .h h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.card > .h span {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.card > .b {
		display: flex;
		flex-direction: column;
		padding: 4px 16px 16px;
	}
	.warn {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		margin-top: 10px;
		padding: 10px 12px;
		border-radius: 4px;
		background: var(--stale-tint);
		border: 1px solid var(--stale);
		font-size: 12px;
	}
	.warn :global(svg) {
		color: var(--stale);
		flex: 0 0 auto;
		margin-top: 1px;
	}
	.warn span {
		flex: 1;
	}
	.rule {
		display: grid;
		grid-template-columns: 68px minmax(0, 1fr);
		gap: 12px;
		padding: 14px 0;
		border-bottom: 1px solid var(--hairline);
		align-items: start;
	}
	.rule > .k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		padding-top: 9px;
	}
	.col {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}
	.sent {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		font-size: 13px;
	}
	.sent.sub {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.pick7 {
		height: 32px;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
		color: var(--ink);
		min-width: 0;
		max-width: 100%;
		cursor: pointer;
	}
	.in7 {
		height: 32px;
		width: 72px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		color: var(--ink);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.in7.wide {
		width: 100%;
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
	}
	.in7.tiny {
		width: 52px;
		height: 28px;
	}
	.ord {
		display: grid;
		grid-template-columns: 22px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		min-height: 36px;
		font-size: 13px;
	}
	.ord .no {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--frame);
		color: var(--gold);
		font:
			700 12px/22px Alegreya,
			Georgia,
			serif;
		text-align: center;
		align-self: start;
		margin-top: 5px;
	}
	.ord .w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.chk {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		cursor: pointer;
	}
	.chk input {
		width: 14px;
		height: 14px;
		margin: 0;
		accent-color: var(--gold-ink);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.acc7 {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		height: 32px;
		padding: 0 10px;
		border-radius: 4px;
		border: 1px solid var(--border);
		font-size: 12px;
		cursor: pointer;
	}
	.acc7 input {
		width: 14px;
		height: 14px;
		margin: 0;
		accent-color: var(--gold-ink);
	}
	.acc7.on {
		border-color: var(--gold-ink);
		background: var(--gold-tint);
	}
	.runs {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 12px;
		padding-top: 8px;
		min-width: 0;
	}
	.rn {
		display: flex;
		gap: 8px;
		min-width: 0;
	}
	.rn .mono {
		flex: 0 0 auto;
	}
	.rn .tx {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.res {
		margin-left: auto;
		font:
			600 11px 'JetBrains Mono',
			monospace;
		text-align: right;
		white-space: nowrap;
	}
	.res.ok {
		color: var(--done);
	}
	.res.bad {
		color: var(--not-started);
	}
	.res.go {
		color: var(--progress);
	}
	.acts {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		padding-top: 14px;
	}
	.acts .btn :global(svg) {
		width: 14px;
		height: 14px;
	}
	.note {
		margin: 10px 0 0;
		font-size: 13px;
		color: var(--ink-muted);
	}
	.sm {
		font-size: 11px;
	}
	.foot {
		font-size: 11px;
	}
	@media (max-width: 900px) {
		.job {
			grid-template-columns: 28px minmax(0, 1fr);
		}
		.job .q {
			grid-column: 2;
		}
		.job .tag {
			display: none;
		}
		.rule {
			grid-template-columns: minmax(0, 1fr);
		}
		.rule > .k {
			padding-top: 0;
		}
	}
</style>
