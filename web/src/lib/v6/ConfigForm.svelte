<script lang="ts">
	import {
		microbotApi,
		MicrobotError,
		type PluginSchema,
		type SchemaItem
	} from '#lib/api/microbot';
	import { fieldError, formSections } from '#lib/progress/plugins';

	/*
	 * Formulário de config de um plugin (doc 08, fase D): as opções vêm do plugin Chronicle Link do
	 * microbot/, que exporta os descritores do RuneLite; os valores são lidos e gravados pelo
	 * /settings/plugin (chave/valor) do Agent Server. Gravar vai para a auditoria; opção com aviso
	 * do plugin pede um segundo clique. Sem o export, `onschema(false)` e a página mostra o
	 * chave/valor cru.
	 */
	let {
		className,
		running,
		onschema,
		onwrite
	}: {
		className: string;
		running: boolean;
		onschema?: (ok: boolean, why?: string) => void;
		onwrite?: () => void;
	} = $props();

	let schema = $state<PluginSchema | null>(null);
	let values = $state<Record<string, string | null>>({});
	let edits = $state<Record<string, string>>({});
	let open = $state<Record<string, boolean>>({});
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let error = $state<string | null>(null);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	const sections = $derived(formSections(schema));

	async function readValues(s: PluginSchema) {
		const keys = s.items.filter((i) => !i.hidden).map((i) => i.key);
		const out: Record<string, string | null> = {};
		for (let i = 0; i < keys.length; i += 50) {
			const chunk = await microbotApi.config(s.group, keys.slice(i, i + 50));
			Object.assign(out, chunk.values);
		}
		values = out;
		edits = Object.fromEntries(keys.map((k) => [k, out[k] ?? '']));
	}

	/** The plugin to load, "" while the client is down; a string, so the poll's new snippet
	 * arguments (same class) do not reload the form and lose the edits. */
	const target = $derived(running ? className : '');

	$effect(() => {
		const c = target;
		if (!c) return;
		let gone = false;
		schema = null;
		error = null;
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
				const why = e instanceof MicrobotError ? e.message : String(e);
				onschema?.(false, why);
			}
		})();
		return () => {
			gone = true;
		};
	});

	async function save(it: SchemaItem, value: string | null) {
		if (!schema || busy) return;
		if (it.warning && value !== null && armed !== it.key) {
			armed = it.key;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = it.key;
		error = null;
		try {
			await microbotApi.plugin({
				path: 'config',
				body: { group: schema.group, key: it.key, value }
			});
			await readValues(schema);
			onwrite?.();
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
		}
	}
	const changed = (k: string) => (edits[k] ?? '') !== (values[k] ?? '');
</script>

{#if schema}
	<div class="form">
		{#if error}<p class="err">{error}</p>{/if}
		{#each sections as sec (sec.key)}
			<div class="sec">
				<button
					class="sech"
					type="button"
					aria-expanded={open[sec.key]}
					title={sec.description}
					onclick={() => (open[sec.key] = !open[sec.key])}
					><span class="car">{open[sec.key] ? '▾' : '▸'}</span>{sec.name}<span class="n"
						>{sec.items.length}</span
					></button
				>
				{#if open[sec.key]}
					{#each sec.items as it (it.key)}
						{@const bad = changed(it.key) ? fieldError(it, edits[it.key] ?? '') : null}
						<div class="row" class:dirty={changed(it.key)}>
							<label class="lbl" for="cf-{it.key}" title={it.description || it.key}
								>{it.name}<span class="key">{it.key}</span></label
							>
							<div class="ctl">
								{#if it.type === 'boolean'}
									<input
										id="cf-{it.key}"
										type="checkbox"
										checked={(edits[it.key] ?? '') === 'true'}
										onchange={(e) => (edits[it.key] = String(e.currentTarget.checked))}
									/>
								{:else if it.type === 'enum'}
									<select id="cf-{it.key}" class="in" bind:value={edits[it.key]}>
										{#if !values[it.key]}<option value="">(padrão)</option>{/if}
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
										placeholder={values[it.key] === null ? '(padrão)' : ''}
										value={edits[it.key] ?? ''}
										oninput={(e) => (edits[it.key] = e.currentTarget.value)}
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
										placeholder={values[it.key] === null ? '(padrão)' : ''}
										bind:value={edits[it.key]}
									/>
								{/if}
							</div>
							<div class="acts">
								{#if bad}<span class="err tiny">{bad}</span>{/if}
								<button
									class="btn btn-s"
									class:btn-d={armed === it.key}
									type="button"
									disabled={!!busy || !changed(it.key) || !!bad}
									title={it.warning}
									onclick={() => save(it, edits[it.key] ?? '')}
									>{busy === it.key
										? 'Gravando…'
										: armed === it.key
											? 'Confirmar'
											: 'Gravar'}</button
								>
								<button
									class="lnk"
									type="button"
									title="Volta ao padrão do plugin (apaga a chave)"
									disabled={!!busy || values[it.key] === null}
									onclick={() => save(it, null)}>padrão</button
								>
							</div>
						</div>
					{/each}
				{/if}
			</div>
		{/each}
	</div>
{/if}

<style>
	.form {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.sec {
		border: 1px solid var(--hairline);
		border-radius: 4px;
	}
	.sech {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		padding: 8px 10px;
		border: 0;
		background: var(--elevated);
		color: var(--ink);
		font: inherit;
		font-size: 12px;
		font-weight: 600;
		text-align: left;
		cursor: pointer;
	}
	.sech .car {
		color: var(--ink-subtle);
		width: 10px;
	}
	.sech .n {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.row {
		display: grid;
		grid-template-columns: minmax(140px, 1fr) minmax(120px, 1.2fr) auto;
		gap: 8px;
		align-items: center;
		padding: 6px 10px;
		border-top: 1px solid var(--hairline);
		font-size: 12px;
	}
	.row.dirty {
		background: var(--gold-tint);
	}
	@media (max-width: 640px) {
		.row {
			grid-template-columns: 1fr;
		}
	}
	.lbl {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
	}
	.key {
		font:
			400 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.ctl {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
	}
	.in {
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
	.in.num {
		width: 110px;
	}
	.mono {
		font-family: 'JetBrains Mono', monospace;
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
	.acts {
		display: flex;
		align-items: center;
		gap: 8px;
		white-space: nowrap;
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
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.err {
		margin: 0;
		color: var(--not-started);
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
</style>
