<script lang="ts">
	import {
		liveConfigStore,
		microbotApi,
		MicrobotError,
		type ConfigStore,
		type PluginSchema,
		type SchemaItem
	} from '#lib/api/microbot';
	import { fieldError, formSections } from '#lib/progress/plugins';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * Formulário de config de um plugin (doc 08, fase D; V7ClienteGaveta1440 e V7PluginsManutencao1440):
	 * as opções vêm do plugin Chronicle Link, que exporta os descritores do RuneLite; os valores são
	 * lidos e gravados pelo `store` (o cliente ao vivo, ou um perfil em /plugins). Um Salvar só grava
	 * todas as mudanças; a opção com aviso do plugin pede um segundo clique antes de entrar. Sem o
	 * export, `onschema(false)` e quem usa mostra o chave/valor cru.
	 */
	let {
		className,
		running,
		store = liveConfigStore,
		layout = 'stack',
		profile = {},
		dirty = $bindable(0),
		onschema,
		onsaved
	}: {
		className: string;
		running: boolean;
		store?: ConfigStore;
		/** "stack": rótulo em cima (a gaveta); "grid": rótulo ao lado (/plugins). */
		layout?: 'stack' | 'grid';
		/** O valor do perfil de cada opção que o cliente tem diferente, por chave. */
		profile?: Record<string, string>;
		/** Quantas mudanças ainda não salvas. */
		dirty?: number;
		onschema?: (ok: boolean, why?: string) => void;
		onsaved?: () => void;
	} = $props();

	let schema = $state<PluginSchema | null>(null);
	let values = $state<Record<string, string | null>>({});
	let edits = $state<Record<string, string | null>>({});
	let confirmed = $state<Record<string, boolean>>({});
	let open = $state<Record<string, boolean>>({});
	let busy = $state(false);
	let error = $state<string | null>(null);
	let savedAt = $state<number | null>(null);

	const sections = $derived(formSections(schema));
	const items = $derived(sections.flatMap((s) => s.items));
	const changed = (k: string) => (edits[k] ?? null) !== (values[k] ?? null);
	const changes = $derived(items.filter((it) => changed(it.key)));
	/** Uma opção com aviso mudada espera o segundo clique antes de poder salvar. */
	const waiting = (it: SchemaItem) =>
		!!it.warning && changed(it.key) && edits[it.key] !== null && !confirmed[it.key];
	const bad = (it: SchemaItem) =>
		changed(it.key) && edits[it.key] !== null ? fieldError(it, edits[it.key] ?? '') : null;
	const blocked = $derived(changes.some((it) => waiting(it) || !!bad(it)));
	$effect(() => {
		dirty = changes.length;
	});

	async function readValues(s: PluginSchema) {
		const keys = s.items.filter((i) => !i.hidden).map((i) => i.key);
		const out = await store.read(s.group, keys);
		values = out;
		edits = Object.fromEntries(keys.map((k) => [k, out[k] ?? null]));
		confirmed = {};
	}

	/** The plugin to load, "" while the client is down; a string, so a poll's new props (same
	 * class) do not reload the form and lose the edits. */
	const target = $derived(running ? className : '');

	$effect(() => {
		const c = target;
		if (!c) return;
		let gone = false;
		schema = null;
		error = null;
		savedAt = null;
		void (async () => {
			try {
				const s = await microbotApi.configSchema(c);
				if (gone) return;
				schema = s;
				open = Object.fromEntries(formSections(s).map((x) => [x.key, !x.closed]));
				onschema?.(true);
				await readValues(s);
			} catch (e) {
				if (gone) return;
				onschema?.(false, e instanceof MicrobotError ? e.message : String(e));
			}
		})();
		return () => {
			gone = true;
		};
	});

	/** Grava todas as mudanças; false quando algo impediu (aviso sem confirmar, valor recusado, erro). */
	export async function save(): Promise<boolean> {
		if (!schema || busy || blocked) return false;
		const entries = changes.map((it) => ({ key: it.key, value: edits[it.key] ?? null }));
		if (!entries.length) return true;
		busy = true;
		error = null;
		try {
			if (store.writeMany) await store.writeMany(schema.group, entries);
			else for (const e of entries) await store.write(schema.group, e.key, e.value);
			await readValues(schema);
			savedAt = Date.now();
			onsaved?.();
			return true;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
			return false;
		} finally {
			busy = false;
		}
	}

	export function discard() {
		edits = Object.fromEntries(Object.keys(edits).map((k) => [k, values[k] ?? null]));
		confirmed = {};
		error = null;
	}

	function set(it: SchemaItem, v: string | null) {
		edits[it.key] = v;
		confirmed[it.key] = false;
		savedAt = null;
	}
	const hhmm = (ms: number) =>
		new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const label = (it: SchemaItem, v: string) =>
		it.type === 'enum' ? (it.values?.find((x) => x.value === v)?.label ?? v) : v;
</script>

{#if schema}
	<div class="form" class:grid={layout === 'grid'}>
		{#if error}<p class="err" role="alert">{error}</p>{/if}
		{#each sections as sec (sec.key)}
			{@const drifted = sec.items.filter((it) => it.key in profile).length}
			<div class="sec">
				<button
					class="sh"
					type="button"
					aria-expanded={open[sec.key]}
					title={sec.description}
					onclick={() => (open[sec.key] = !open[sec.key])}
					><span class="car" class:on={open[sec.key]}><Glyph name="chevron" size={12} /></span
					>{sec.name}<span class="n" class:dv={!open[sec.key] && drifted}
						>{!open[sec.key] && drifted ? `${drifted} fora do perfil` : sec.items.length}</span
					></button
				>
				{#if open[sec.key]}
					{#each sec.items as it (it.key)}
						{@const err = bad(it)}
						<div class="fld" class:ch={changed(it.key)} class:bad={!!err}>
							<label for="cf-{it.key}" title={it.description || it.key}>{it.name}</label>
							<div class="ctl">
								{#if it.type === 'boolean'}
									<input
										id="cf-{it.key}"
										type="checkbox"
										checked={(edits[it.key] ?? '') === 'true'}
										onchange={(e) => set(it, String(e.currentTarget.checked))}
									/>
								{:else if it.type === 'enum'}
									<select
										id="cf-{it.key}"
										class="in"
										value={edits[it.key] ?? ''}
										onchange={(e) => set(it, e.currentTarget.value)}
									>
										{#if !edits[it.key]}<option value="">(padrão)</option>{/if}
										{#each it.values ?? [] as v (v.value)}
											<option value={v.value}>{v.label}</option>
										{/each}
									</select>
								{:else if it.type === 'int' || it.type === 'double'}
									<input
										id="cf-{it.key}"
										class="in num"
										type="number"
										step={it.type === 'int' ? 1 : 'any'}
										min={it.min}
										max={it.max}
										placeholder={edits[it.key] === null ? '(padrão)' : ''}
										aria-invalid={!!err}
										value={edits[it.key] ?? ''}
										oninput={(e) => set(it, e.currentTarget.value)}
									/>{#if it.units}<span class="units">{it.units.trim()}</span>{/if}
								{:else}
									{#if it.type === 'color' && edits[it.key]}<span
											class="sw"
											style="background: {edits[it.key]}"
										></span>{/if}
									<input
										id="cf-{it.key}"
										class="in mono"
										title={it.type === 'keybind' || it.type === 'other'
											? `Valor cru do RuneLite (${it.type})`
											: undefined}
										placeholder={edits[it.key] === null ? '(padrão)' : ''}
										value={edits[it.key] ?? ''}
										oninput={(e) => set(it, e.currentTarget.value)}
									/>
								{/if}
								{#if layout === 'grid' && values[it.key] !== null && values[it.key] !== undefined}
									<button
										class="lnk"
										type="button"
										title="Volta ao padrão do plugin (apaga a chave ao salvar)"
										onclick={() => set(it, null)}>padrão</button
									>
								{/if}
							</div>
							{#if err}<p class="msg err">{err}</p>{/if}
							{#if it.key in profile && !changed(it.key)}
								<span class="was" title="Valor no perfil da conta; o cliente roda com o de cima"
									>perfil: {label(it, profile[it.key])}</span
								>
							{/if}
							{#if waiting(it)}
								<div class="arm" role="alert">
									<span>{it.warning} Ligar mesmo assim?</span>
									<span class="a">
										<button
											class="btn btn-s btn-d"
											type="button"
											onclick={() => (confirmed[it.key] = true)}>Confirmar</button
										>
										<button
											class="btn btn-s btn-g"
											type="button"
											onclick={() => (edits[it.key] = values[it.key] ?? null)}>Cancelar</button
										>
									</span>
								</div>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		{/each}
		<div class="ft">
			<span
				class="n8"
				class:ok={!changes.length && savedAt}
				class:none={!changes.length && !savedAt}
				>{changes.length
					? `${changes.length} ${changes.length === 1 ? 'mudança' : 'mudanças'}`
					: savedAt
						? `salvo · ${hhmm(savedAt)}`
						: 'sem mudanças'}</span
			>
			{#if changes.length}
				<button class="btn btn-g" type="button" disabled={busy} onclick={discard}>Descartar</button>
			{/if}
			<button
				class="btn btn-p"
				type="button"
				disabled={busy || !changes.length || blocked}
				title={blocked ? 'Confirme ou corrija o campo marcado' : undefined}
				onclick={() => void save()}>{busy ? 'Salvando…' : 'Salvar'}</button
			>
		</div>
	</div>
{/if}

<style>
	.form {
		flex: 1 1 auto;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.sec {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.sh {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 32px;
		padding: 0;
		border: 0;
		border-bottom: 1px solid var(--border);
		background: transparent;
		cursor: pointer;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		text-align: left;
	}
	.sh .car {
		display: inline-flex;
	}
	.sh .car.on {
		transform: rotate(90deg);
	}
	.sh .n {
		margin-left: auto;
		letter-spacing: 0;
		text-transform: none;
		font-size: 11px;
	}
	.sh .n.dv {
		color: var(--stale);
	}
	.fld {
		display: flex;
		flex-direction: column;
		gap: 5px;
		min-width: 0;
	}
	.fld > label {
		font-size: 12px;
		font-weight: 500;
		color: var(--ink-muted);
	}
	.fld.ch {
		padding-left: 9px;
		box-shadow: inset 3px 0 0 var(--gold-ink);
	}
	.fld.ch > label::after {
		content: ' · mudou';
		font-weight: 400;
		color: var(--gold-ink);
	}
	.grid .fld {
		display: grid;
		grid-template-columns: 150px minmax(0, 1fr);
		gap: 4px 14px;
		align-items: center;
	}
	.grid .fld > :not(label, .ctl) {
		grid-column: 2;
	}
	.ctl {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.in {
		height: 32px;
		box-sizing: border-box;
		padding: 0 10px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		color: var(--ink);
		font:
			400 12px Inter,
			system-ui,
			sans-serif;
		min-width: 0;
		width: 100%;
	}
	.in.mono {
		font-family: 'JetBrains Mono', monospace;
	}
	.in.num {
		width: 120px;
	}
	.fld.ch .in {
		border-color: var(--gold-ink);
	}
	.fld.bad .in {
		border-color: var(--not-started);
	}
	.ctl input[type='checkbox'] {
		width: 16px;
		height: 16px;
		margin: 0;
		accent-color: var(--gold-ink);
	}
	.units {
		color: var(--ink-subtle);
		font-size: 11px;
	}
	.sw {
		width: 16px;
		height: 16px;
		border-radius: 3px;
		border: 1px solid var(--border);
		flex: 0 0 auto;
	}
	.was {
		display: flex;
		align-items: center;
		gap: 6px;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--stale);
	}
	.was::before {
		content: '';
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--stale);
		flex: 0 0 auto;
	}
	.arm {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		border-radius: 4px;
		border: 1px solid var(--not-started);
		background: var(--stale-tint);
		font-size: 12px;
		line-height: 1.4;
	}
	.arm .a {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.msg {
		margin: 0;
		font-size: 12px;
		font-weight: 500;
	}
	.err {
		color: var(--not-started);
		margin: 0;
		font-size: 12px;
	}
	.lnk {
		border: 0;
		background: none;
		padding: 0;
		color: var(--gold-ink);
		font-size: 11px;
		cursor: pointer;
		text-decoration: underline;
		white-space: nowrap;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	/* O rodapé gruda no fim do contêiner que rola (a gaveta). */
	.ft {
		position: sticky;
		bottom: 0;
		z-index: 1;
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		margin: auto -14px 0;
		padding: 10px 14px;
		border-top: 1px solid var(--border);
		background: var(--elevated);
		box-shadow: var(--bevel);
	}
	.grid .ft {
		position: static;
		margin: 0;
		padding: 12px 0 0;
		border-top: 1px solid var(--hairline);
		background: transparent;
		box-shadow: none;
	}
	.ft .btn {
		height: 36px;
	}
	.n8 {
		margin-right: auto;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--gold-ink);
	}
	.n8.ok {
		color: var(--done);
	}
	.n8.none {
		color: var(--ink-subtle);
	}
</style>
