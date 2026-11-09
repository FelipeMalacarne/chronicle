<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		microbotApi,
		MicrobotError,
		type HubList,
		type HubPlugin,
		type MicrobotStatus,
		type PluginAction,
		type PluginsView,
		type ScriptDetail
	} from '#lib/api/microbot';
	import {
		detailFacts,
		pluginRows,
		rowLine,
		rowVersion,
		simpleClass,
		type PluginRow
	} from '#lib/progress/plugins';
	import Glyph from '#lib/v6/Glyph.svelte';
	import ConfigForm from '#lib/v6/ConfigForm.svelte';
	import { untrack } from 'svelte';

	/*
	 * Automação › Plugins (V6MicrobotPlugins1440), doc 08 fase 3: os plugins carregados no cliente
	 * (GET /scripts) com os do Plugin Hub, iniciar e parar, pausar e retomar os scripts, instalar,
	 * atualizar e remover do Hub, a state machine e os resultados, e a configuração chave/valor (o
	 * Agent Server não lista as opções de um plugin, lacuna do doc 08). Cada ação vai para a auditoria.
	 */
	let status = $state<MicrobotStatus | null>(null);
	let off = $state(false);
	let view = $state<PluginsView | null>(null);
	let loadError = $state<string | null>(null);
	let selected = $state<string | null>(null);
	let detail = $state<ScriptDetail | null>(null);
	let showCore = $state(false);
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let raw = $state<'machines' | 'results' | null>(null);
	let q = $state('');
	let hub = $state<HubList | null>(null);
	let hubBusy = $state(false);

	const running = $derived(!!status?.client?.running);
	const paused = $derived(!!status?.client?.state?.scriptsPaused);
	const rows = $derived(pluginRows(view));
	const visible = $derived(showCore ? rows : rows.filter((r) => r.source !== 'cliente'));
	const coreCount = $derived(rows.filter((r) => r.source === 'cliente').length);
	const sel = $derived(rows.find((r) => r.className === selected) ?? visible[0] ?? null);
	const facts = $derived(detailFacts(detail));
	/** As leituras do escolhido dependem só da classe: a lista se renova a cada 5 s. */
	const selClass = $derived(sel?.className ?? null);

	async function load() {
		try {
			const s = await microbotApi.status();
			if (s === null) {
				off = true;
				return;
			}
			status = s;
			if (s.client?.running) view = await microbotApi.plugins();
			loadError = null;
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		}
	}
	async function loadDetail(className: string) {
		try {
			detail = await microbotApi.scriptDetail(className);
		} catch {
			detail = null;
		}
	}
	$effect(() => {
		void load();
		const t = setInterval(() => {
			if (!document.hidden) void load();
		}, 5000);
		return () => clearInterval(t);
	});
	$effect(() => {
		const c = selClass;
		if (!c || !running) return;
		detail = null;
		raw = null;
		void loadDetail(c);
		const t = setInterval(() => {
			if (!document.hidden) void loadDetail(c);
		}, 5000);
		return () => clearInterval(t);
	});

	/** Ações que mexem no jogo ou apagam algo pedem um segundo clique em até 4 s. */
	let disarm: ReturnType<typeof setTimeout> | undefined;
	async function act(id: string, a: PluginAction, confirm = false) {
		if (busy) return;
		if (confirm && armed !== id) {
			armed = id;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = id;
		failure = null;
		try {
			await microbotApi.plugin(a);
		} catch (e) {
			failure = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			await load();
			if (sel) await loadDetail(sel.className);
		}
	}
	const lbl = (id: string, text: string) =>
		busy === id ? `${text}…` : armed === id ? `Confirmar: ${text.toLowerCase()}` : text;

	async function searchHub(e?: SubmitEvent) {
		e?.preventDefault();
		hubBusy = true;
		failure = null;
		try {
			hub = await microbotApi.hub(q.trim());
		} catch (err) {
			failure = err instanceof MicrobotError ? err.message : String(err);
		} finally {
			hubBusy = false;
		}
	}
	function hubButton(p: HubPlugin): string | null {
		if (!p.installable) return null;
		if (!p.installedVersion) return 'Instalar';
		if (p.latestVersion && p.latestVersion !== p.installedVersion)
			return `Atualizar → ${p.latestVersion}`;
		return null;
	}

	// Configuração chave/valor: o grupo e as chaves de cada plugin ficam neste navegador.
	let group = $state('');
	let keys = $state<string[]>([]);
	let values = $state<Record<string, string | null>>({});
	let edits = $state<Record<string, string>>({});
	let newKey = $state('');
	let cfgError = $state<string | null>(null);
	/** O Dev Bootstrap exportou as opções deste plugin: mostra o formulário; senão, o chave/valor cru. */
	let formOk = $state(false);
	let formWhy = $state<string | null>(null);
	let rawMode = $state(false);
	const cfgStore = (c: string) => `ironman.microbot.config.${c}`;

	$effect(() => {
		const c = selClass;
		if (!c) return;
		let saved: { group?: string; keys?: string[] } = {};
		try {
			saved = JSON.parse(localStorage.getItem(cfgStore(c)) ?? '{}');
		} catch {
			/* sem armazenamento: começa vazio */
		}
		group =
			saved.group ??
			simpleClass(c)
				.replace(/Plugin$/, '')
				.toLowerCase();
		keys = saved.keys ?? [];
		values = {};
		edits = {};
		cfgError = null;
		formOk = false;
		formWhy = null;
		rawMode = false;
		untrack(() => void readCfg());
	});
	function saveCfg() {
		if (!sel) return;
		try {
			localStorage.setItem(cfgStore(sel.className), JSON.stringify({ group, keys }));
		} catch {
			/* sem armazenamento: vale só nesta visita */
		}
	}
	async function readCfg() {
		cfgError = null;
		if (!group || !keys.length || !running) return;
		try {
			values = (await microbotApi.config(group, keys)).values;
			edits = Object.fromEntries(keys.map((k) => [k, values[k] ?? '']));
		} catch (e) {
			cfgError = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	function addKey(e: SubmitEvent) {
		e.preventDefault();
		const k = newKey.trim();
		if (!k || keys.includes(k)) return;
		keys = [...keys, k];
		newKey = '';
		saveCfg();
		void readCfg();
	}
	function dropKey(k: string) {
		keys = keys.filter((x) => x !== k);
		saveCfg();
	}
	async function writeCfg(k: string, value: string | null) {
		await act(`cfg:${k}`, { path: 'config', body: { group, key: k, value } });
		await readCfg();
	}
</script>

<svelte:head>
	<title>Plugins · Automação · Chronicle</title>
</svelte:head>

<header class="mh">
	<div class="ttl">
		<p class="ey">Microbot · controle do cliente · doc 08</p>
		<h1 class="sr">Plugins</h1>
	</div>
	<div class="seg" role="tablist" aria-label="Automação">
		<a href={resolve('/automacao')}>Cockpit</a>
		<a class="on" href={resolve('/automacao/plugins')} aria-current="page">Plugins</a>
		<a href="{resolve('/automacao')}#auditoria">Auditoria</a>
	</div>
	<form class="hubf" onsubmit={searchHub}>
		<label class="inp"
			><Glyph name="search" size={14} /><input
				type="search"
				placeholder="buscar no Plugin Hub…"
				aria-label="Buscar plugins no Hub"
				bind:value={q}
			/></label
		>
		<button class="btn btn-s" type="submit" disabled={!running || hubBusy}
			>{hubBusy ? 'Buscando…' : 'Instalar do Hub'}</button
		>
	</form>
</header>

{#if off}
	<section class="panel pad">
		<p class="sm0">
			O controle do Microbot está desligado neste servidor (<code>MICROBOT_CONTROL=on</code> liga).
		</p>
	</section>
{:else if !status}
	<p class="su sm0" aria-busy="true">{loadError ?? 'Carregando…'}</p>
{:else if !status.agent.connected || !running}
	<section class="panel pad">
		<p class="sm0">
			{!status.agent.connected
				? 'Sem agent: rode `ironman agent` no zaros.'
				: 'O cliente está parado; os plugins aparecem com ele rodando.'}
			<a href={resolve('/automacao')}>Ir ao Cockpit</a>
		</p>
	</section>
{:else}
	{#if failure}
		<div class="band" role="alert"><Glyph name="alert" /><span>{failure}</span></div>
	{/if}

	{#if hub}
		<section class="panel">
			<div class="plaque">
				Plugin Hub<span class="r"
					>{hub.count} de {hub.total}{q.trim() ? ` · “${q.trim()}”` : ''} ·
					<button class="lnk" type="button" onclick={() => (hub = null)}>fechar</button></span
				>
			</div>
			<div class="hubl">
				{#each hub.plugins as p (p.internalName)}
					{@const b = hubButton(p)}
					<div class="hr">
						<div class="hn">
							<span class="nm">{p.name.trim()}</span>
							<span class="d">{p.description ?? ''}</span>
						</div>
						<span class="ver mono"
							>{p.installedVersion ? `instalado ${p.installedVersion}` : p.latestVersion}</span
						>
						{#if b}
							<button
								class="btn btn-s"
								type="button"
								disabled={!!busy}
								onclick={() =>
									act(`hub:${p.internalName}`, {
										path: 'plugins/install',
										body: { name: p.internalName }
									})}>{lbl(`hub:${p.internalName}`, b)}</button
							>
						{:else if !p.installable}
							<span class="su tiny" title={p.health}>não instalável</span>
						{/if}
					</div>
				{:else}
					<p class="su sm0 pad">Nada no Hub com esse nome.</p>
				{/each}
			</div>
		</section>
	{/if}

	<div class="cols">
		<section class="panel list">
			<div class="plaque">
				Instalados<span class="r"
					>GET /scripts · GET /plugins{paused ? ' · scripts pausados' : ''}</span
				>
			</div>
			<div>
				{#each visible as r (r.className)}
					<button
						type="button"
						class="pl"
						class:pl-on={sel?.className === r.className}
						aria-current={sel?.className === r.className}
						onclick={() => (selected = r.className)}
						><span class="dot" class:dot-w={r.tone === 'warn'} class:dot-off={r.tone === 'off'}
						></span><span class="nm">{r.name}</span><span class="d">{rowLine(r)}</span><span
							class="ver">{rowVersion(r)}</span
						></button
					>
				{:else}
					<p class="su sm0 pad">Nenhum plugin do Hub nem seu; “Instalar do Hub” busca.</p>
				{/each}
				{#if coreCount}
					<button class="lnk more" type="button" onclick={() => (showCore = !showCore)}
						>{showCore ? 'esconder' : 'mostrar'} os do cliente ({coreCount})</button
					>
				{/if}
			</div>
		</section>

		{#if sel}
			{@render detailPanel(sel)}
		{/if}
	</div>

	<footer class="su tiny">
		Fora da lista fechada e por isso fora desta tela: /scripts/deploy (Java arbitrário), teclado e
		widgets crus, qualquer exec além do mb-client. Grupos e chaves de segredo (token, senha, PIN) e
		a configuração do Agent Server e do núcleo não são lidos nem gravados daqui.
	</footer>
{/if}

{#snippet detailPanel(r: PluginRow)}
	<section class="panel det">
		<div class="plaque">
			{r.name}<span class="r"
				>{r.version ? `${r.version} · ` : ''}{r.source === 'hub'
					? 'Plugin Hub'
					: r.source === 'meus'
						? 'seu, sideload'
						: 'do cliente'}{r.active ? ' · rodando' : ''}</span
			>
		</div>
		<div class="pad stack">
			<div class="acts">
				{#if r.protected}
					<span class="su sm0">Este plugin fica fora do controle pela web.</span>
				{:else if r.active}
					<button
						class="btn btn-s"
						type="button"
						disabled={!!busy}
						onclick={() => act('pause', { path: paused ? 'scripts/resume' : 'scripts/pause' })}
						title="Pausa ou retoma todos os loops de script do cliente, sem parar plugins"
						>{lbl('pause', paused ? 'Retomar todos' : 'Pausar todos')}</button
					>
					<button
						class="btn btn-s btn-d"
						type="button"
						disabled={!!busy}
						onclick={() =>
							act('stop', { path: 'scripts/stop', body: { className: r.className } }, true)}
						>{lbl('stop', 'Parar')}</button
					>
				{:else}
					<button
						class="btn btn-s"
						class:btn-d={armed === 'start'}
						type="button"
						disabled={!!busy}
						title="Com a conta logada, o plugin começa a agir no jogo"
						onclick={() =>
							act('start', { path: 'scripts/start', body: { className: r.className } }, true)}
						>{lbl('start', 'Iniciar')}</button
					>
				{/if}
				<button
					class="btn btn-g btn-s"
					type="button"
					aria-pressed={raw === 'machines'}
					onclick={() => (raw = raw === 'machines' ? null : 'machines')}
					>Snapshot da state machine</button
				>
				<button
					class="btn btn-g btn-s"
					type="button"
					aria-pressed={raw === 'results'}
					onclick={() => (raw = raw === 'results' ? null : 'results')}>Resultados</button
				>
				{#if r.update && !r.protected}
					<button
						class="btn btn-s"
						type="button"
						disabled={!!busy}
						onclick={() =>
							act('update', { path: 'plugins/install', body: { name: simpleClass(r.className) } })}
						>{lbl('update', `Atualizar → ${r.update}`)}</button
					>
				{/if}
				{#if r.source === 'hub' && !r.protected}
					<button
						class="btn btn-g btn-s push"
						class:btn-d={armed === 'remove'}
						type="button"
						disabled={!!busy}
						onclick={() =>
							act(
								'remove',
								{ path: 'plugins/remove', body: { name: simpleClass(r.className) } },
								true
							)}>{lbl('remove', 'Remover')}</button
					>
				{/if}
			</div>

			<div class="facts">
				{#each facts as f (f.k)}
					<div class="kv"><span class="k">{f.k}</span><span class="val">{f.v}</span></div>
				{:else}
					<span class="su sm0" aria-busy="true">Lendo o plugin…</span>
				{/each}
			</div>

			{#if raw}
				<pre class="raw">{JSON.stringify(
						raw === 'machines' ? detail?.machines : detail?.results,
						null,
						2
					) ?? 'sem dados'}</pre>
			{/if}

			{#if !r.protected}
				<div class="cfg">
					<div class="cfgh">
						<p class="ey">Configuração{formOk && !rawMode ? '' : ' · chave/valor'}</p>
						{#if formOk}
							<button class="lnk" type="button" onclick={() => (rawMode = !rawMode)}
								>{rawMode ? 'formulário' : 'chave/valor cru'}</button
							>
						{:else}
							<span class="badge badge-stale" title={formWhy ?? undefined}
								>sem as opções do plugin: chave/valor cru</span
							>
						{/if}
					</div>
					<ConfigForm
						className={r.className}
						{running}
						onschema={(ok, why) => {
							formOk = ok;
							formWhy = why ?? null;
						}}
						onwrite={() => void load()}
					/>
				</div>
				{#if !formOk || rawMode}
					<div class="cfg">
						<div class="cfgh">
							<label class="grp"
								><span class="k">grupo</span><input
									class="cell-in mono"
									bind:value={group}
									onchange={() => {
										saveCfg();
										void readCfg();
									}}
								/></label
							>
							<button
								class="btn btn-g btn-s"
								type="button"
								onclick={readCfg}
								disabled={!keys.length}>Ler</button
							>
						</div>
						{#if cfgError}<p class="err tiny">{cfgError}</p>{/if}
						<table class="cfgt">
							<thead><tr><th>chave</th><th>valor</th><th></th></tr></thead>
							<tbody>
								{#each keys as k (k)}
									<tr>
										<td class="mono">{k}</td>
										<td
											><input
												class="cell-in mono"
												aria-label="Valor de {k}"
												placeholder={k in values && values[k] === null ? '(padrão)' : ''}
												bind:value={edits[k]}
											/></td
										>
										<td class="rowacts">
											<button
												class="btn btn-s"
												type="button"
												disabled={!!busy || (edits[k] ?? '') === (values[k] ?? '')}
												onclick={() => writeCfg(k, edits[k] ?? '')}
												>{lbl(`cfg:${k}`, 'Gravar')}</button
											>
											<button
												class="lnk"
												type="button"
												title="Volta ao padrão do plugin (apaga a chave)"
												disabled={!!busy || values[k] === null}
												onclick={() => writeCfg(k, null)}>padrão</button
											>
											<button
												class="lnk"
												type="button"
												title="Tira a chave desta lista (não muda o cliente)"
												onclick={() => dropKey(k)}>×</button
											>
										</td>
									</tr>
								{/each}
								<tr>
									<td colspan="3">
										<form class="addk" onsubmit={addKey}>
											<input
												class="cell-in mono"
												placeholder="keyName"
												aria-label="Nova chave"
												bind:value={newKey}
											/>
											<button class="btn btn-g btn-s" type="submit" disabled={!newKey.trim()}
												>+ chave</button
											>
										</form>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				{/if}
			{/if}
		</div>
	</section>
{/snippet}

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
		margin-right: 6px;
	}
	.ttl h1 {
		font-size: 24px;
	}
	.seg a {
		display: inline-flex;
		align-items: center;
		height: 32px;
		padding: 0 10px;
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		color: var(--ink-muted);
		text-decoration: none;
		background: var(--elevated);
	}
	.seg a + a {
		border-left: 1px solid var(--border);
	}
	.seg a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.hubf {
		margin-left: auto;
		display: flex;
		gap: 8px;
		align-items: center;
		flex-wrap: wrap;
	}
	.inp {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 32px;
		padding: 0 10px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--ground);
		color: var(--ink-subtle);
	}
	.inp input {
		border: 0;
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-size: 13px;
		width: 200px;
		outline: none;
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 20px;
		align-items: flex-start;
	}
	.list {
		flex: 1 1 420px;
		min-width: 0;
	}
	.det {
		flex: 999 1 460px;
		min-width: 0;
	}
	.pad {
		padding: 12px;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: 14px;
	}
	.pl {
		display: flex;
		align-items: center;
		gap: 12px;
		box-sizing: border-box;
		width: 100%;
		min-height: 48px;
		padding: 0 12px;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-size: 13px;
		text-align: left;
		cursor: pointer;
	}
	.pl:hover {
		background: var(--elevated);
	}
	.pl-on {
		background: var(--gold-tint);
		box-shadow: inset 0 0 0 1px var(--gold-ink);
	}
	.pl .nm {
		font-weight: 500;
		width: 180px;
		flex: 0 0 auto;
	}
	.hr .nm {
		font-size: 13px;
		font-weight: 500;
	}
	.pl .d,
	.hr .d {
		flex: 1;
		min-width: 0;
		font-size: 12px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pl .ver,
	.hr .ver {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.lnk.more {
		display: block;
		padding: 10px 14px;
	}
	.hubl {
		max-height: 340px;
		overflow: auto;
	}
	.hr {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 14px;
		border-bottom: 1px solid var(--hairline);
	}
	.hn {
		display: flex;
		flex-direction: column;
		gap: 2px;
		flex: 1 1 auto;
		min-width: 0;
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
	}
	.push {
		margin-left: auto;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.facts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(150px, 100%), 1fr));
		gap: 10px;
	}
	.kv {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.k {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.kv .val {
		font-size: 13px;
		font-weight: 500;
	}
	.raw {
		margin: 0;
		max-height: 260px;
		overflow: auto;
		padding: 10px;
		border-radius: 4px;
		background: var(--frame);
		color: var(--on-frame);
		font:
			400 11px 'JetBrains Mono',
			monospace;
	}
	.cfg {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.cfgh {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 10px;
	}
	.cfgh .ey {
		margin: 0;
	}
	.grp {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.cell-in {
		height: 28px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font-size: 12px;
		min-width: 0;
		width: 100%;
	}
	.grp .cell-in {
		width: 160px;
	}
	.cfgt {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
	}
	.cfgt th {
		text-align: left;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		padding: 4px 6px;
		border-bottom: 1px solid var(--hairline);
	}
	.cfgt td {
		padding: 5px 6px;
		border-bottom: 1px solid var(--hairline);
		vertical-align: middle;
	}
	.rowacts {
		white-space: nowrap;
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.addk {
		display: flex;
		gap: 8px;
		align-items: center;
		max-width: 360px;
	}
	.lnk {
		border: 0;
		background: none;
		padding: 0;
		color: var(--gold-ink);
		font: inherit;
		font-size: 12px;
		cursor: pointer;
		text-decoration: underline;
	}
	.lnk:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.mono {
		font-family: 'JetBrains Mono', monospace;
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
	.tiny {
		font-size: 11px;
		margin: 0;
	}
	.err {
		color: var(--not-started);
	}
	@media (max-width: 900px) {
		.hubf {
			margin-left: 0;
		}
		.grp {
			margin-left: 0;
		}
	}
</style>
