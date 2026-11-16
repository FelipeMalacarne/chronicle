<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		driftApi,
		liveConfigStore,
		microbotApi,
		MicrobotError,
		type SettingDrift
	} from '#lib/api/microbot';
	import { drawerMeta, profileValues, type CardRow } from '#lib/progress/clientplugins';
	import { simpleClass } from '#lib/progress/plugins';
	import { gameMap, gameMenu } from '#lib/v6/game/menu.svelte';
	import ConfigForm from '#lib/v6/ConfigForm.svelte';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * A gaveta de configuração da aba Cliente (V7ClienteGaveta1440, V7ClienteGaveta640 e os estados
	 * de V7ClienteGavetaEstados1440; seção 13 do brief). Abre sobre a coluna da direita, com o jogo,
	 * o chat e o painel de itens à vista; a 640 ocupa a tela, com "Voltar ao jogo". Salvar vale no
	 * cliente que roda, na hora. O desvio do perfil da conta aparece com "Salvar no perfil" e
	 * "Descartar", só pelo botão (W9). Trocar de plugin é pelo nome no topo, porque o cartão fica
	 * embaixo da gaveta; com mudanças não salvas, fechar e trocar perguntam antes.
	 */
	let {
		row,
		rows,
		account,
		hasProfile,
		drift,
		inGame,
		wide,
		accountName,
		onclose,
		onswitch,
		onchange
	}: {
		row: CardRow;
		/** Todos os plugins do cartão, para trocar pelo nome no topo. */
		rows: { label: string; items: CardRow[] }[];
		account: string;
		/** A conta tem perfil próprio: "Salvar no perfil" vai para ele; sem perfil, para a conta. */
		hasProfile: boolean;
		drift: SettingDrift[];
		inGame: boolean;
		wide: boolean;
		accountName: string;
		onclose: () => void;
		onswitch: (className: string) => void;
		onchange: () => void;
	} = $props();

	let form = $state<ConfigForm | null>(null);
	let dirty = $state(0);
	let formOk = $state<boolean | null>(null);
	let picking = $state(false);
	let ask = $state<{ kind: 'close' } | { kind: 'switch'; to: CardRow } | null>(null);
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let error = $state<string | null>(null);
	/** Muda depois de salvar no perfil ou descartar: o formulário relê os valores do cliente. */
	let reload = $state(0);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	const profile = $derived(profileValues(drift));
	const dirtyAll = $derived(dirty + kvDirty());

	function requestClose() {
		if (dirtyAll) ask = { kind: 'close' };
		else onclose();
	}
	function requestSwitch(to: CardRow) {
		picking = false;
		if (to.className === row.className) return;
		if (dirtyAll) ask = { kind: 'switch', to };
		else onswitch(to.className);
	}
	async function resolveAsk(how: 'save' | 'discard') {
		const a = ask;
		if (!a) return;
		if (how === 'save') {
			const ok = formOk === false ? await kvSave() : await form?.save();
			if (!ok) {
				ask = null;
				return;
			}
		}
		ask = null;
		if (a.kind === 'close') onclose();
		else onswitch(a.to.className);
	}

	// Esc fecha primeiro o menu do jogo e o mapa (eles mesmos ouvem a tecla), depois a gaveta.
	function key(e: KeyboardEvent) {
		if (e.key !== 'Escape' || gameMenu.open || gameMenu.selected || gameMap.open) return;
		if (picking) picking = false;
		else if (ask) ask = null;
		else requestClose();
	}

	async function act(k: string, run: () => Promise<unknown>, confirm = false) {
		if (busy) return;
		if (confirm && armed !== k) {
			armed = k;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = k;
		error = null;
		try {
			await run();
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			onchange();
		}
	}
	const keys = $derived(drift.map((d) => ({ group: d.group, key: d.key })));
	const saveDrift = () =>
		act('drift-save', async () => {
			await driftApi.save(account, hasProfile ? 'profile' : 'account', keys);
			reload++;
		});
	const discardDrift = () =>
		act('drift-discard', async () => {
			const r = await driftApi.discard(account, keys);
			if (r.problems.length) throw new Error(r.problems.join(' · '));
			reload++;
		});

	// Sem o esquema do plugin: chave/valor cru, com as chaves lembradas neste navegador.
	let group = $state('');
	let kvKeys = $state<string[]>([]);
	let kvValues = $state<Record<string, string | null>>({});
	let kvEdits = $state<Record<string, string>>({});
	let newKey = $state('');
	let kvBusy = $state(false);
	const kvStore = (c: string) => `chronicle.microbot.config.${c}`;
	function kvDirty() {
		return formOk === false
			? kvKeys.filter((k) => (kvEdits[k] ?? '') !== (kvValues[k] ?? '')).length
			: 0;
	}
	$effect(() => {
		if (formOk !== false) return;
		const c = row.className;
		let saved: { group?: string; keys?: string[] } = {};
		try {
			saved = JSON.parse(localStorage.getItem(kvStore(c)) ?? '{}');
		} catch {
			/* sem armazenamento: começa vazio */
		}
		group =
			saved.group ??
			simpleClass(c)
				.replace(/Plugin$/, '')
				.toLowerCase();
		kvKeys = saved.keys ?? [];
		void kvRead();
	});
	function kvRemember() {
		try {
			localStorage.setItem(kvStore(row.className), JSON.stringify({ group, keys: kvKeys }));
		} catch {
			/* sem armazenamento: vale só nesta visita */
		}
	}
	async function kvRead() {
		if (!group || !kvKeys.length) {
			kvValues = {};
			kvEdits = {};
			return;
		}
		try {
			kvValues = await liveConfigStore.read(group, kvKeys);
			kvEdits = Object.fromEntries(kvKeys.map((k) => [k, kvValues[k] ?? '']));
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		}
	}
	async function kvSave(): Promise<boolean> {
		const changed = kvKeys.filter((k) => (kvEdits[k] ?? '') !== (kvValues[k] ?? ''));
		kvBusy = true;
		error = null;
		try {
			for (const k of changed) await liveConfigStore.write(group, k, kvEdits[k] ?? '');
			await kvRead();
			onchange();
			return true;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
			return false;
		} finally {
			kvBusy = false;
		}
	}
	function kvAdd(e: SubmitEvent) {
		e.preventDefault();
		const k = newKey.trim();
		if (!k || kvKeys.includes(k)) return;
		kvKeys = [...kvKeys, k];
		newKey = '';
		kvRemember();
		void kvRead();
	}
	function kvDrop(k: string) {
		kvKeys = kvKeys.filter((x) => x !== k);
		kvRemember();
	}
</script>

<svelte:window onkeydown={key} />

{#snippet body()}
	<div class="hd">
		<div class="t8">
			{#if row.protected}<Glyph name="lock" size={14} />{:else}<span class="lamp {row.lamp}"
				></span>{/if}
			<button
				class="who"
				type="button"
				aria-haspopup="listbox"
				aria-expanded={picking}
				title="Trocar de plugin"
				onclick={() => (picking = !picking)}
				><span>{row.name}</span><span class="car" class:on={picking}
					><Glyph name="chevron" size={12} /></span
				></button
			>
			{#if wide}
				<button
					class="ib ib-g"
					type="button"
					aria-label="Fechar"
					title="Fechar"
					onclick={requestClose}><Glyph name="x" size={14} /></button
				>
			{/if}
		</div>
		<div class="m8">{drawerMeta(row)}</div>
		<div class="a8">
			{#if row.active}
				<button
					class="btn btn-s btn-d"
					type="button"
					disabled={!!busy}
					title="Pede um segundo clique"
					onclick={() =>
						act(
							'stop',
							() =>
								microbotApi.plugin({ path: 'scripts/stop', body: { className: row.className } }),
							true
						)}
					><Glyph name="stop" size={12} />{armed === 'stop' ? 'Confirmar: parar' : 'Parar'}</button
				>
			{:else if !row.protected}
				<button
					class="btn btn-s"
					class:btn-d={armed === 'start'}
					type="button"
					disabled={!!busy || !inGame}
					title={inGame
						? 'Age no jogo com a conta logada; pede um segundo clique'
						: 'Entre no jogo primeiro'}
					onclick={() =>
						act(
							'start',
							() =>
								microbotApi.plugin({ path: 'scripts/start', body: { className: row.className } }),
							true
						)}
					><Glyph name="play" size={12} />{armed === 'start' ? 'Confirmar: ligar' : 'Ligar'}</button
				>
			{/if}
			<a class="lnk" href={resolve('/plugins')}>em Plugins</a>
		</div>
	</div>

	{#if picking}
		<div class="lst" role="listbox" aria-label="Plugins do cliente">
			{#each rows as g (g.label)}
				{#if g.items.length}
					<span class="k">{g.label}</span>
					{#each g.items.filter((r) => !r.protected) as r (r.className)}
						<button
							class="o"
							class:on={r.className === row.className}
							type="button"
							role="option"
							aria-selected={r.className === row.className}
							onclick={() => requestSwitch(r)}
							><span class="lamp {r.lamp}"></span><span class="nm">{r.name}</span><span class="w"
								>{r.className === row.className ? 'aberto' : r.active ? 'ligado' : 'parado'}</span
							></button
						>
					{/each}
				{/if}
			{/each}
		</div>
	{/if}

	<div class="bd">
		{#if error}<p class="err" role="alert">{error}</p>{/if}
		{#if drift.length}
			<div class="dft" role="status">
				<b
					>{drift.length === 1
						? '1 opção diferente do perfil'
						: `${drift.length} opções diferentes do perfil`}</b
				>
				<span class="s"
					>O cliente roda com elas agora; o perfil da conta vale no próximo início.</span
				>
				<span class="a">
					<button
						class="btn btn-s"
						type="button"
						disabled={!!busy}
						title={hasProfile
							? 'Grava os valores do cliente no perfil da conta'
							: 'Grava os valores do cliente na conta'}
						onclick={saveDrift}>{busy === 'drift-save' ? 'Salvando…' : 'Salvar no perfil'}</button
					>
					<button
						class="btn btn-s btn-g"
						type="button"
						disabled={!!busy}
						title="Devolve ao cliente os valores do perfil"
						onclick={discardDrift}>{busy === 'drift-discard' ? 'Descartando…' : 'Descartar'}</button
					>
				</span>
			</div>
		{/if}

		{#key `${row.className}|${reload}`}
			<ConfigForm
				bind:this={form}
				bind:dirty
				className={row.className}
				running={true}
				{profile}
				onschema={(ok) => (formOk = ok)}
				onsaved={onchange}
			/>
		{/key}

		{#if formOk === false}
			<p class="mu tiny">
				Este plugin não diz quais opções tem. Chaves de senha e PIN não aparecem.
			</p>
			<label class="grp"
				><span class="kh">grupo</span><input
					class="in mono"
					bind:value={group}
					onchange={() => {
						kvRemember();
						void kvRead();
					}}
				/></label
			>
			<div class="kv">
				<span class="kh">chave</span><span class="kh">valor</span><span></span>
				{#each kvKeys as k (k)}
					<span class="in mono ro">{k}</span>
					<input
						class="in mono"
						class:ch={(kvEdits[k] ?? '') !== (kvValues[k] ?? '')}
						aria-label="Valor de {k}"
						placeholder={kvValues[k] === null ? '(padrão)' : ''}
						bind:value={kvEdits[k]}
					/>
					<button
						class="ib ib-g"
						type="button"
						aria-label="Tirar {k}"
						title="Tirar a chave desta lista"
						onclick={() => kvDrop(k)}><Glyph name="x" size={12} /></button
					>
				{/each}
			</div>
			<form class="add" onsubmit={kvAdd}>
				<input class="in mono" placeholder="chave" aria-label="Nova chave" bind:value={newKey} />
				<button class="btn btn-s btn-g" type="submit" disabled={!newKey.trim()}
					><Glyph name="plus" size={12} />Chave</button
				>
			</form>
			<div class="kft">
				<span class="n8" class:none={!kvDirty()}
					>{kvDirty()
						? `${kvDirty()} ${kvDirty() === 1 ? 'mudança' : 'mudanças'}`
						: 'sem mudanças'}</span
				>
				{#if kvDirty()}<button class="btn btn-g" type="button" onclick={kvRead}>Descartar</button
					>{/if}
				<button
					class="btn btn-p"
					type="button"
					disabled={kvBusy || !kvDirty()}
					onclick={() => void kvSave()}>{kvBusy ? 'Salvando…' : 'Salvar'}</button
				>
			</div>
		{:else if formOk === null}
			<p class="mu tiny" aria-busy="true">Lendo as opções do plugin…</p>
		{/if}
	</div>

	{#if ask}
		<div class="ask" role="alertdialog" aria-label="Mudanças não salvas">
			<span
				>{#if ask.kind === 'close'}<b
						>{dirtyAll === 1 ? '1 mudança não salva.' : `${dirtyAll} mudanças não salvas.`}</b
					>
					Fechar mesmo assim?{:else}Abrir o <b>{ask.to.name}</b>? O {row.name} tem {dirtyAll === 1
						? '1 mudança'
						: `${dirtyAll} mudanças`}.{/if}</span
			>
			<span class="a">
				<button class="btn btn-p" type="button" onclick={() => resolveAsk('save')}
					>{ask.kind === 'close' ? 'Salvar e fechar' : 'Salvar e abrir'}</button
				>
				<button class="btn" type="button" onclick={() => resolveAsk('discard')}
					>{ask.kind === 'close' ? 'Descartar' : 'Descartar e abrir'}</button
				>
				<button class="btn btn-g" type="button" onclick={() => (ask = null)}
					>{ask.kind === 'close' ? 'Voltar' : 'Ficar'}</button
				>
			</span>
		</div>
	{/if}
{/snippet}

<!-- One element at both widths: crossing 700 px must not remount the form and lose the edits. -->
<div
	class="drawer"
	class:sheet={!wide}
	role="dialog"
	aria-modal={!wide}
	aria-label="Configuração de {row.name}"
>
	{#if !wide}
		<div class="bar">
			<button class="bk" type="button" onclick={requestClose}
				><span class="back"><Glyph name="chevron" size={14} /></span>Voltar ao jogo</button
			>
			<span class="live"><span class="lamp {inGame ? 'ok' : 'off'}"></span>{accountName}</span>
		</div>
	{/if}
	<div class="dw" class:full={!wide}>{@render body()}</div>
</div>

<style>
	/* 1440: sobre a coluna da direita, na largura dela (V7ClienteGaveta1440); quem usa põe o pai em
	   position: relative. */
	.drawer:not(.sheet) {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		width: 320px;
		min-height: 560px;
		z-index: 5;
		display: flex;
		flex-direction: column;
	}
	.drawer:not(.sheet) > .dw {
		flex: 1 1 auto;
	}
	.dw {
		display: flex;
		flex-direction: column;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--gold-ink);
		box-shadow:
			var(--bevel),
			0 0 0 2px var(--gold-tint),
			-12px 0 28px rgba(43, 33, 20, 0.22);
		min-width: 0;
		overflow: hidden;
		box-sizing: border-box;
	}
	.dw.full {
		flex: 1 1 auto;
		border: 0;
		border-radius: 0;
		box-shadow: none;
	}
	.hd {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 10px 10px 12px 14px;
		border-bottom: 1px solid var(--border);
	}
	.t8 {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		color: var(--ink-subtle);
	}
	.who {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		min-height: 36px;
		padding: 0 6px 0 0;
		border: 0;
		background: transparent;
		color: var(--ink);
		cursor: pointer;
		font:
			700 18px/1.15 Alegreya,
			Georgia,
			serif;
		text-align: left;
	}
	.who > span:first-child {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.car {
		display: inline-flex;
		color: var(--ink-subtle);
		transform: rotate(90deg);
	}
	.car.on {
		transform: rotate(-90deg);
	}
	.t8 .ib {
		margin-left: auto;
	}
	.m8 {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.a8 {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		padding-top: 2px;
	}
	.a8 .lnk {
		margin-left: auto;
		font-size: 12px;
		color: var(--gold-ink);
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.lst {
		display: flex;
		flex-direction: column;
		margin: 10px 10px 0 14px;
		border-radius: 5px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: 0 6px 16px rgba(43, 33, 20, 0.16);
	}
	.lst .k {
		padding: 8px 12px 2px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.lst .o {
		display: grid;
		grid-template-columns: 9px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		min-height: 40px;
		padding: 0 12px;
		border: 0;
		border-top: 1px solid var(--hairline);
		background: transparent;
		color: var(--ink);
		font:
			400 13px Inter,
			system-ui,
			sans-serif;
		text-align: left;
		cursor: pointer;
	}
	.lst .o.on {
		background: var(--gold-tint);
		font-weight: 600;
	}
	.lst .nm {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.lst .w {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.bd {
		display: flex;
		flex-direction: column;
		gap: 14px;
		padding: 12px 14px 0;
		flex: 1 1 auto;
		overflow-y: auto;
		min-height: 0;
	}
	.dft {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 10px 12px;
		border-radius: 4px;
		background: var(--stale-tint);
		border: 1px solid var(--stale);
		font-size: 13px;
		line-height: 1.4;
	}
	.dft b {
		font-weight: 600;
	}
	.dft .s {
		font-size: 12px;
		color: var(--ink-muted);
	}
	.dft .a {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		padding-top: 2px;
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.tiny {
		margin: 0;
		font-size: 12px;
	}
	.kh {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.grp {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.kv {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 28px;
		gap: 6px;
		align-items: center;
	}
	.kv .ib {
		width: 28px;
		height: 28px;
	}
	.in {
		height: 32px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		box-shadow: var(--bevel);
		color: var(--ink);
		font-size: 12px;
		min-width: 0;
		width: 100%;
	}
	.in.ro {
		display: flex;
		align-items: center;
		overflow: hidden;
		text-overflow: ellipsis;
		box-shadow: none;
		background: transparent;
	}
	.in.ch {
		border-color: var(--gold-ink);
	}
	.mono {
		font-family: 'JetBrains Mono', monospace;
	}
	.add {
		display: flex;
		gap: 6px;
	}
	.kft {
		position: sticky;
		bottom: 0;
		display: flex;
		align-items: center;
		gap: 8px;
		margin: auto -14px 0;
		padding: 10px 14px;
		border-top: 1px solid var(--border);
		background: var(--elevated);
		box-shadow: var(--bevel);
	}
	.kft .btn {
		height: 36px;
	}
	.n8 {
		margin-right: auto;
		font:
			500 11px 'JetBrains Mono',
			monospace;
		color: var(--gold-ink);
	}
	.n8.none {
		color: var(--ink-subtle);
	}
	.ask {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 14px;
		border-top: 1px solid var(--gold-ink);
		background: var(--gold-tint);
		font-size: 13px;
	}
	.ask b {
		font-weight: 600;
	}
	.ask .a {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.ask .btn {
		height: 36px;
	}
	/* 640: a configuração ocupa a tela (V7ClienteGaveta640). */
	.sheet {
		position: fixed;
		inset: 0;
		z-index: 40;
		display: flex;
		flex-direction: column;
		background: var(--surface);
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 52px;
		padding: 4px 8px;
		background: var(--frame);
		box-shadow: var(--bevel-frame);
		color: var(--on-frame);
	}
	.bk {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 44px;
		padding: 0 12px 0 8px;
		border: 0;
		border-radius: 4px;
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
		color: var(--gold);
		font:
			500 13px Inter,
			system-ui,
			sans-serif;
		cursor: pointer;
	}
	.back {
		display: inline-flex;
		transform: rotate(180deg);
	}
	.live {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding-right: 8px;
		font-size: 12px;
		color: var(--on-frame-muted);
	}
</style>
