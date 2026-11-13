<script lang="ts">
	import {
		fleetPluginsApi,
		microbotApi,
		MicrobotError,
		setMicrobotAccount,
		type ConfigStore,
		type HubPlugin
	} from '#lib/api/microbot';
	import {
		accountProfileName,
		FLEET_MODEL,
		GROUPS,
		outdated,
		ownAdjust,
		pluginClass,
		pluginId,
		pluginTitle,
		present,
		settingsOf,
		statusText,
		update,
		withPlugin,
		withSetting,
		withoutPlugin,
		type FleetPlugin,
		type FleetPluginAccount,
		type FleetPlugins,
		type ProfileLayer
	} from '#lib/progress/fleetplugins';
	import ConfigForm from '#lib/v6/ConfigForm.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Plugins da frota (v7, fatia V8; artboard V7Plugins1440). O padrão da frota é o perfil modelo
	 * `frota`; cada conta tem o seu perfil sobre ele, com o ajuste próprio (decisão do dono de
	 * 2026-11-13). Os grupos saem do uso. Mudar um perfil vale no próximo boot do cliente; "Aplicar
	 * agora" põe o perfil nos clientes ligados.
	 */
	let data = $state<FleetPlugins | null>(null);
	let error = $state<string | null>(null);
	let busy = $state<string | null>(null);
	let note = $state<string | null>(null);
	let selected = $state<string | null>(null);
	let tab = $state<string>(FLEET_MODEL);
	let group = $state<string | null>(null);
	let query = $state('');
	let found = $state<HubPlugin[] | null>(null);

	async function load() {
		try {
			data = await fleetPluginsApi.list();
			error = null;
			if (!selected || !data.plugins.some((p) => pluginId(p) === selected)) {
				selected = data.plugins[0] ? pluginId(data.plugins[0]) : null;
			}
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	$effect(() => {
		void load();
	});

	const plugin = $derived(data?.plugins.find((p) => pluginId(p) === selected) ?? null);
	const byGroup = $derived(
		GROUPS.map((g) => ({ ...g, items: (data?.plugins ?? []).filter((p) => p.group === g.key) }))
	);
	/** Uma conta com o cliente ligado responde pelo esquema de config e pelo Hub. */
	const live = $derived(data?.accounts.find((a) => a.connected) ?? null);
	$effect(() => {
		if (live) setMicrobotAccount(live.account);
	});
	const accountOf = (id: string) => data?.accounts.find((a) => a.account === id);
	const rsn = (id: string) => accountOf(id)?.rsn ?? id;

	// O grupo de config do plugin aberto, para o "ajuste próprio" e o formulário.
	$effect(() => {
		group = null;
		const p = plugin;
		if (!p || !live || !pluginClass(p)) return;
		let gone = false;
		void microbotApi
			.configSchema(pluginClass(p))
			.then((s) => {
				if (!gone) group = s.group;
			})
			.catch(() => {});
		return () => {
			gone = true;
		};
	});

	function modelLayer(): ProfileLayer {
		const m = data?.model;
		return { plugins: m?.plugins ?? [], settings: m?.settings ?? [] };
	}

	async function ensureModel() {
		if (!data?.model) await fleetPluginsApi.putProfile(FLEET_MODEL, { plugins: [], settings: [] });
	}

	/** O perfil da conta, criado sobre o modelo da frota quando ela ainda não tem um. */
	async function ensureAccountProfile(
		a: FleetPluginAccount
	): Promise<{ name: string; layer: ProfileLayer }> {
		if (a.profile) {
			return {
				name: a.profile,
				layer: { parent: a.parent, plugins: a.own_plugins, settings: a.own }
			};
		}
		await ensureModel();
		const name = accountProfileName(a.account);
		const layer: ProfileLayer = { parent: FLEET_MODEL, plugins: [], settings: [] };
		await fleetPluginsApi.putProfile(name, layer);
		const current = await fleetPluginsApi.accountClient(a.account);
		await fleetPluginsApi.setAccountClient(a.account, name, current.settings);
		return { name, layer };
	}

	async function run(label: string, fn: () => Promise<string | void>) {
		if (busy) return;
		busy = label;
		note = null;
		try {
			const out = await fn();
			if (out) note = out;
			await load();
		} catch (e) {
			note = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
		}
	}

	/** Aplica o perfil nos clientes ligados das contas, e diz o que deu errado. */
	async function applyTo(accounts: string[]): Promise<string> {
		const problems: string[] = [];
		let n = 0;
		for (const id of accounts) {
			if (!accountOf(id)?.connected) continue;
			const r = await fleetPluginsApi.apply(id);
			n++;
			for (const p of r.problems) problems.push(`${rsn(id)}: ${p}`);
		}
		if (problems.length) return problems.join(' · ');
		return n
			? `Aplicado em ${n} ${n === 1 ? 'conta' : 'contas'}.`
			: 'Vale no próximo início de cada cliente.';
	}

	function search() {
		const q = query.trim();
		if (!q || !live) {
			found = null;
			return;
		}
		void microbotApi
			.hub(q)
			.then((r) => (found = r.plugins))
			.catch((e) => (note = e instanceof MicrobotError ? e.message : String(e)));
	}

	/** Instalar vale para a frota: entra no modelo, e as contas sem perfil ganham o seu sobre ele. */
	const install = (h: HubPlugin) =>
		run('install', async () => {
			await ensureModel();
			await fleetPluginsApi.putProfile(
				FLEET_MODEL,
				withPlugin(modelLayer(), { source: 'hub', name: h.internalName, start: false })
			);
			for (const a of data?.accounts ?? []) if (!a.profile) await ensureAccountProfile(a);
			found = null;
			query = '';
			selected = `hub:${h.internalName}`;
			return applyTo((data?.accounts ?? []).map((a) => a.account));
		});

	/** Uma conta passa a ter (ou deixa de ter) o plugin no perfil dela; o do modelo vale para todas. */
	const toggle = (p: FleetPlugin, a: FleetPluginAccount, on: boolean) =>
		run(`toggle:${a.account}`, async () => {
			const { name, layer } = await ensureAccountProfile(a);
			const next = on
				? withPlugin(layer, {
						source: p.source,
						name: p.name,
						class_name: p.class_name,
						start: false
					})
				: withoutPlugin(layer, pluginId(p));
			await fleetPluginsApi.putProfile(name, next);
			return on ? applyTo([a.account]) : 'Sai do perfil; o cliente o mantém até ser removido nele.';
		});

	const updateAll = (p: FleetPlugin) =>
		run('update', async () => {
			const version = update(p);
			if (!version) return;
			if (p.in_model && p.model) {
				await fleetPluginsApi.putProfile(
					FLEET_MODEL,
					withPlugin(modelLayer(), { ...p.model, version })
				);
			}
			for (const a of data?.accounts ?? []) {
				const mine = a.own_plugins.find((x) => pluginId(x) === pluginId(p));
				if (mine && a.profile) {
					await fleetPluginsApi.putProfile(
						a.profile,
						withPlugin(
							{ parent: a.parent, plugins: a.own_plugins, settings: a.own },
							{ ...mine, version }
						)
					);
				}
			}
			return applyTo(outdated(p));
		});

	const remove = (p: FleetPlugin) =>
		run('remove', async () => {
			if (p.in_model)
				await fleetPluginsApi.putProfile(FLEET_MODEL, withoutPlugin(modelLayer(), pluginId(p)));
			for (const a of data?.accounts ?? []) {
				if (a.profile && a.own_plugins.some((x) => pluginId(x) === pluginId(p))) {
					await fleetPluginsApi.putProfile(
						a.profile,
						withoutPlugin(
							{ parent: a.parent, plugins: a.own_plugins, settings: a.own },
							pluginId(p)
						)
					);
				}
			}
			return 'Fora dos perfis; os clientes o mantêm até ser removido neles.';
		});

	/** O formulário de config lê e grava no perfil da aba: o modelo da frota ou o da conta. */
	const store = $derived.by((): ConfigStore => {
		const t = tab;
		return {
			read: async (g, keys) => {
				if (t === FLEET_MODEL) return settingsOf(data?.model?.settings ?? [], g, keys);
				return settingsOf(accountOf(t)?.own ?? [], g, keys);
			},
			write: async (g, key, value) => {
				if (t === FLEET_MODEL) {
					await ensureModel();
					await fleetPluginsApi.putProfile(FLEET_MODEL, withSetting(modelLayer(), g, key, value));
				} else {
					const a = accountOf(t);
					if (!a) return;
					const { name, layer } = await ensureAccountProfile(a);
					await fleetPluginsApi.putProfile(name, withSetting(layer, g, key, value));
				}
				await load();
			}
		};
	});

	function lamp(s: { active?: boolean; installed?: string; in_profile: boolean }): string {
		if (s.active) return 'ok';
		if (s.installed !== undefined || s.active === false) return 'rest';
		return '';
	}
</script>

<svelte:head><title>Plugins · Chronicle</title></svelte:head>

<header class="top">
	<div class="ttl">
		<h1 class="sr">Plugins</h1>
		<p class="mu">
			O que as contas sabem fazer. Instalar e configurar vale para a frota; cada conta pode ter o
			seu ajuste.
		</p>
	</div>
	<form
		class="field"
		onsubmit={(e) => {
			e.preventDefault();
			search();
		}}
	>
		<Glyph name="search" />
		<input
			type="text"
			placeholder="buscar no Plugin Hub"
			aria-label="Buscar no Plugin Hub"
			bind:value={query}
			disabled={!live}
			title={live ? undefined : 'A busca no Hub passa por um cliente ligado'}
		/>
	</form>
</header>

{#if error}<p class="err">{error}</p>{/if}
{#if note}<p class="note">{note}</p>{/if}

{#if found}
	<section class="card hub">
		<div class="h">
			<h2>No Plugin Hub</h2>
			<span>{found.length} {found.length === 1 ? 'plugin' : 'plugins'}</span>
		</div>
		<div class="b">
			{#each found as h (h.internalName)}
				<div class="hrow">
					<span class="x"><b>{h.name}</b><span>{h.description}</span></span>
					<span class="ver">{h.latestVersion}</span>
					<button
						class="btn btn-s"
						type="button"
						disabled={!!busy || !h.installable}
						onclick={() => install(h)}>Instalar na frota</button
					>
				</div>
			{:else}
				<p class="mu">Nada com esse nome.</p>
			{/each}
		</div>
	</section>
{/if}

{#if data}
	<div class="cols">
		<div class="list">
			{#each byGroup as g (g.key)}
				{#if g.items.length}
					<section>
						<div class="grp7">
							<h2>{g.title}</h2>
							<span>{g.hint}</span>
						</div>
						{#each g.items as p (pluginId(p))}
							{@const up = update(p)}
							<button
								class="pl7"
								class:on={pluginId(p) === selected}
								type="button"
								onclick={() => {
									selected = pluginId(p);
									tab = FLEET_MODEL;
								}}
							>
								<span class="ico"><Glyph name="automacao" /></span>
								<span class="x"
									><b>{pluginTitle(p)}</b><span
										>{p.source === 'hub'
											? 'Plugin Hub'
											: p.source === 'own'
												? 'deste repositório'
												: 'do cliente'}{p.in_model ? ' · no padrão da frota' : ''}</span
									></span
								>
								<span class="r">
									<span class="lp">
										{#each present(p) as s (s.account)}<span
												class="lamp {lamp(s)}"
												title={rsn(s.account)}
											></span>{/each}
									</span>
									{statusText(p, data.accounts.length)}
									{#if up}<span class="badge badge-stale">{up} disponível</span>{/if}
								</span>
							</button>
						{/each}
					</section>
				{/if}
			{:else}
				<p class="mu">Nenhum plugin ainda. Busque no Plugin Hub para instalar na frota.</p>
			{/each}
			{#if !data.plugins.length}
				<p class="mu">
					Nenhum plugin nos perfis nem nos clientes ligados. Busque no Plugin Hub para instalar na
					frota.
				</p>
			{/if}
		</div>

		{#if plugin}
			{@const up = update(plugin)}
			{@const versions = [...new Set(plugin.accounts.map((s) => s.installed).filter(Boolean))]}
			<aside class="card">
				<div class="h">
					<h2>{pluginTitle(plugin)}</h2>
					{#if versions.length || up}<span>{versions.join(' · ')}{up ? ` → ${up}` : ''}</span>{/if}
				</div>
				<div class="b">
					<div>
						<p class="lbl">Nas contas</p>
						{#each data.accounts as a (a.account)}
							{@const s = plugin.accounts.find((x) => x.account === a.account)}
							<div class="inst">
								<input
									type="checkbox"
									checked={s?.in_profile ?? false}
									disabled={!!busy || plugin.in_model}
									title={plugin.in_model ? 'Vem do padrão da frota' : 'No perfil desta conta'}
									aria-label="{plugin.in_model ? 'Vem do padrão da frota' : 'No perfil'}: {a.rsn}"
									onchange={(e) => toggle(plugin, a, e.currentTarget.checked)}
								/>
								<span><span class="lamp {s ? lamp(s) : ''}"></span>{a.rsn}</span>
								<span class="ver"
									>{s?.installed ??
										(s?.active !== undefined
											? 'no cliente'
											: a.connected
												? 'não instalado'
												: 'cliente desligado')}</span
								>
								<span
									>{#if ownAdjust(a, group)}<span class="badge badge-gold">ajuste próprio</span
										>{/if}</span
								>
							</div>
						{/each}
						<div class="acts">
							{#if up}
								<button
									class="btn btn-s"
									type="button"
									disabled={!!busy}
									onclick={() => updateAll(plugin)}
									>Atualizar {outdated(plugin).length === 1 ? 'na' : 'nas'}
									{outdated(plugin).length}</button
								>
							{/if}
							<button
								class="btn btn-s"
								type="button"
								disabled={!!busy}
								onclick={() => run('apply', () => applyTo(present(plugin).map((s) => s.account)))}
								>Aplicar agora</button
							>
						</div>
					</div>

					<div>
						<div class="cfgh">
							<p class="lbl">Configuração</p>
							<div class="seg">
								<button
									type="button"
									class:on={tab === FLEET_MODEL}
									onclick={() => (tab = FLEET_MODEL)}>Padrão da frota</button
								>
								{#each data.accounts.filter( (a) => present(plugin).some((s) => s.account === a.account) ) as a (a.account)}
									<button
										type="button"
										class:on={tab === a.account}
										onclick={() => (tab = a.account)}>{a.rsn}</button
									>
								{/each}
							</div>
						</div>
						{#if !pluginClass(plugin)}
							<p class="mu">A configuração aparece depois que o plugin rodar num cliente ligado.</p>
						{:else if !live}
							<p class="mu">
								A configuração vem do próprio plugin: ligue o cliente de uma conta para editá-la.
							</p>
						{:else}
							{#key `${selected}|${tab}`}
								<ConfigForm className={pluginClass(plugin)} running={true} {store} />
							{/key}
						{/if}
					</div>

					<div class="acts">
						<button class="btn btn-g" type="button" disabled={!!busy} onclick={() => remove(plugin)}
							>Remover da frota</button
						>
					</div>
				</div>
			</aside>
		{/if}
	</div>
{:else if !error}
	<p class="mu">Lendo os plugins da frota…</p>
{/if}

<p class="foot">
	Imagens do jogo © Jagex via Old School RuneScape Wiki, CC BY-NC-SA 3.0. Cliente de terceiros, fora
	do planner.
</p>

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
	.field {
		display: flex;
		align-items: center;
		gap: 10px;
		height: 44px;
		flex: 0 1 280px;
		padding: 0 12px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		box-sizing: border-box;
	}
	.field input {
		flex: 1;
		min-width: 0;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--ink);
		font:
			500 14px Inter,
			system-ui,
			sans-serif;
	}
	.field :global(svg) {
		width: 14px;
		height: 14px;
		color: var(--ink-subtle);
	}
	.cols {
		display: flex;
		flex-wrap: wrap;
		gap: 28px;
		align-items: flex-start;
	}
	.list {
		flex: 999 1 480px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 22px;
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
		flex: 1 1 420px;
		max-width: 500px;
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
		gap: 16px;
		padding: 12px 16px 16px;
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
	.pl7 {
		display: grid;
		grid-template-columns: 36px minmax(0, 1fr) auto;
		gap: 12px;
		align-items: center;
		width: 100%;
		min-height: 64px;
		padding: 8px 12px;
		border-radius: 5px;
		border: 1px solid transparent;
		border-bottom-color: var(--hairline);
		background: transparent;
		font-size: 13px;
		color: var(--ink);
		text-align: left;
		cursor: pointer;
	}
	.pl7 .ico {
		width: 36px;
		height: 36px;
		border-radius: 4px;
		background: var(--ground);
		border: 1px solid var(--hairline);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ink-subtle);
	}
	.pl7 .ico :global(svg) {
		width: 18px;
		height: 18px;
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
	.pl7 .r {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 4px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.lp {
		display: flex;
		gap: 4px;
		align-items: center;
	}
	.pl7.on {
		background: var(--gold-tint);
		border-color: var(--gold-ink);
	}
	.pl7:hover {
		border-color: var(--border);
	}
	.lamp {
		display: inline-block;
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		background: var(--ink-subtle);
		box-sizing: border-box;
		margin-right: 6px;
		vertical-align: 0;
	}
	.lp .lamp {
		margin-right: 0;
	}
	.lamp.ok {
		background: var(--done);
		box-shadow: 0 0 0 3px var(--done-tint);
	}
	.lamp.rest {
		background: var(--gold);
	}
	.inst {
		display: grid;
		grid-template-columns: 16px minmax(0, 1fr) auto auto;
		gap: 10px;
		align-items: center;
		min-height: 36px;
		border-bottom: 1px solid var(--hairline);
		font-size: 13px;
	}
	.inst:last-of-type {
		border-bottom: 0;
	}
	.inst input {
		width: 16px;
		height: 16px;
		margin: 0;
		accent-color: var(--gold-ink);
	}
	.ver {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.lbl {
		padding-bottom: 4px;
	}
	.acts {
		display: flex;
		gap: 8px;
		padding-top: 10px;
		flex-wrap: wrap;
	}
	.cfgh {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
		padding-bottom: 10px;
	}
	.hub .hrow {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto auto;
		gap: 12px;
		align-items: center;
		min-height: 44px;
		border-bottom: 1px solid var(--hairline);
	}
	.err,
	.note {
		margin: 0;
		font-size: 13px;
	}
	.err {
		color: var(--not-started);
	}
	.note {
		color: var(--ink-muted);
	}
	.foot {
		font-size: 11px;
	}
</style>
