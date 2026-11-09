<script lang="ts">
	import {
		microbotApi,
		MicrobotError,
		type GameCommand,
		type GameItem,
		type GameView
	} from '#lib/api/microbot';
	import { equipmentLayout, shortQty } from '#lib/progress/conta';
	import { iconFor, itemIcon, ICON_ATTRIBUTION } from '#lib/progress/icons';
	import {
		chatLines,
		dialogueLine,
		equipItems,
		invSlots,
		machineRows,
		nearRows,
		loopScripts,
		skillCells,
		SUGGESTED,
		type NearRow
	} from '#lib/progress/jogo';
	import Glyph from '#lib/v6/Glyph.svelte';

	/*
	 * O jogo visto e tocado pelo agent no cockpit (doc 08): GET /api/microbot/game a cada poucos
	 * segundos e as ações tipadas de POST /api/microbot/game/<ação>, todas na auditoria. Inventário
	 * e equipamento usam as células da Conta (V6Conta1440), com os sprites da wiki. Clicar num item,
	 * NPC, objeto ou item do chão abre o painel Ação com o que dá para fazer com ele.
	 */
	let { view, onchange }: { view: GameView | null; onchange?: () => void } = $props();

	const slots = $derived(invSlots(view));
	const equip = $derived(equipmentLayout(equipItems(view)));
	const skills = $derived(skillCells(view));
	const near = $derived(nearRows(view));
	const machines = $derived(machineRows(view));
	const chat = $derived(chatLines(view));
	const dialogue = $derived(dialogueLine(view));
	const heartbeats = $derived(view?.observe?.scripts ?? []);
	const bank = $derived(view?.bank);
	const loops = $derived(loopScripts(view));
	const me = $derived(view?.observe?.player?.position);
	const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
	const short = (c: string) => c.split('.').pop() ?? c;

	type Sel =
		| { kind: 'inv'; item: GameItem }
		| { kind: 'bank'; item: GameItem }
		| { kind: 'near'; row: NearRow };
	let sel = $state<Sel | null>(null);
	let action = $state('');
	let qty = $state('1');
	let walkTo = $state({ x: '', y: '', plane: '0' });
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let failure = $state<string | null>(null);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	function pick(s: Sel) {
		sel = s;
		action = '';
		qty = '1';
		failure = null;
	}
	const key = (r: NearRow) => `${r.kind}:${r.id}:${r.position?.x}:${r.position?.y}`;

	/** Uma ação no jogo; as que perdem coisa (dropar, depositar tudo) pedem um segundo clique. */
	async function act(
		id: string,
		command: GameCommand,
		body?: Record<string, unknown>,
		confirm = false
	) {
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
			await microbotApi.gameAction(command, body);
		} catch (e) {
			failure = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			onchange?.();
		}
	}
	async function pause() {
		busy = 'pause';
		try {
			await microbotApi.plugin({ path: 'scripts/pause' });
		} catch (e) {
			failure = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			onchange?.();
		}
	}
	const lbl = (id: string, text: string) =>
		busy === id ? `${text}…` : armed === id ? `Confirmar: ${text.toLowerCase()}` : text;
	const off = $derived(!!busy);

	function walk(e?: SubmitEvent) {
		e?.preventDefault();
		const x = Number(walkTo.x);
		const y = Number(walkTo.y);
		const plane = Number(walkTo.plane || 0);
		if (!x || !y) return;
		void act('walk', 'walk', { x, y, plane });
	}
	function walkRow(r: NearRow) {
		if (r.position) void act('walk', 'walk', { ...r.position });
	}
	function interact(r: NearRow, a: string) {
		const command: GameCommand = r.kind === 'npc' ? 'npc_interact' : 'object_interact';
		void act(`do:${a}`, command, { id: r.id, action: a });
	}
</script>

{#snippet cell(i: GameItem, onpick?: () => void)}
	{@const icon = itemIcon(i.id)}
	{@const q = i.quantity ?? 1}
	{@const on = sel && sel.kind !== 'near' && sel.item.id === i.id && sel.item.slot === i.slot}
	<button
		class="it"
		class:you={on}
		type="button"
		disabled={!onpick}
		onclick={onpick}
		title={`${i.name}${q > 1 ? ` ×${q.toLocaleString('pt-BR')}` : ''}${i.actions?.length ? ` · ${i.actions.join(', ')}` : ''}`}
		>{#if icon}<img src={icon} alt="" />{:else}<span class="ab">{i.name.slice(0, 3)}</span
			>{/if}{#if q > 1}<span class="q">{shortQty(q)}</span>{/if}</button
	>
{/snippet}

{#if loops.length}
	<div class="band" role="status" title={loops.join(', ')}>
		<Glyph name="alert" /><span
			>Scripts de loop ativos ({loops.length}). Se algum estiver jogando, ele disputa o mouse com as
			ações daqui.</span
		>
		<button class="btn btn-s" type="button" disabled={!!busy} onclick={pause}
			>{busy === 'pause' ? 'Pausando…' : 'Pausar todos os scripts'}</button
		>
	</div>
{/if}
{#if failure}
	<div class="band" role="alert"><Glyph name="alert" /><span>{failure}</span></div>
{/if}

<div class="top">
	<section class="panel">
		<div class="plaque">
			Inventário<span class="r">{slots.filter(Boolean).length} de 28</span>
		</div>
		<div class="cbody ctr">
			<div class="g4">
				{#each slots as s, n (n)}
					{#if s}{@render cell(s, () => pick({ kind: 'inv', item: s }))}{:else}<span
							class="it e"
							aria-hidden="true"
						></span>{/if}
				{/each}
			</div>
		</div>
	</section>
	<section class="panel">
		<div class="plaque">Equipamento</div>
		<div class="cbody ctr">
			<div class="eq">
				{#each equip.cells as c (c.slot)}
					<div class="slotcell" style="grid-row: {c.row}; grid-column: {c.col};">
						{#if c.item}{@render cell({
								id: c.item.id,
								name: c.item.name,
								quantity: c.item.qty
							})}{:else}<span class="it e" title="{c.label}: vazio"></span>{/if}
					</div>
				{/each}
			</div>
		</div>
	</section>
	<section class="panel act">
		<div class="plaque">
			Ação<span class="r"
				>{sel ? (sel.kind === 'near' ? sel.row.name : sel.item.name) : 'escolha algo'}</span
			>
		</div>
		<div class="pad stack">
			{#if !sel}
				<span class="su sm0">Clique num item, NPC, objeto ou item do chão.</span>
			{:else if sel.kind === 'inv'}
				{@const it = sel.item}
				<div class="acts">
					{#each (it.actions ?? []).filter((a) => a && a !== 'Drop') as a (a)}
						<button
							class="btn btn-s"
							type="button"
							disabled={off}
							onclick={() => act(`use:${a}`, 'inventory_use', { name: it.name, action: a })}
							>{lbl(`use:${a}`, a)}</button
						>
					{/each}
					{#if bank?.open}
						<button
							class="btn btn-s"
							type="button"
							disabled={off}
							onclick={() => act('dep', 'bank_deposit', { name: it.name })}
							>{lbl('dep', 'Depositar')}</button
						>
					{/if}
					<button
						class="btn btn-s btn-d"
						type="button"
						disabled={off}
						onclick={() => act('drop', 'inventory_drop', { name: it.name }, true)}
						>{lbl('drop', 'Dropar')}</button
					>
				</div>
			{:else if sel.kind === 'bank'}
				{@const it = sel.item}
				<form
					class="acts"
					onsubmit={(e) => {
						e.preventDefault();
						void act('wd', 'bank_withdraw', {
							name: it.name,
							quantity: Math.max(1, Number(qty) || 1)
						});
					}}
				>
					{#each ['1', '5', '10', '28'] as n (n)}
						<button class="btn btn-g btn-s" type="button" onclick={() => (qty = n)}>{n}</button>
					{/each}
					<input class="num" inputmode="numeric" aria-label="Quantidade" bind:value={qty} />
					<button class="btn btn-s" type="submit" disabled={off}>{lbl('wd', 'Sacar')}</button>
				</form>
			{:else}
				{@const r = sel.row}
				<span class="su tiny"
					>{r.kind} · {r.distance ?? '?'} tiles{r.position
						? ` · ${r.position.x}, ${r.position.y}, ${r.position.plane}`
						: ''}</span
				>
				<div class="acts">
					{#if r.position}
						<button class="btn btn-s" type="button" disabled={off} onclick={() => walkRow(r)}
							>{lbl('walk', 'Andar até')}</button
						>
					{/if}
					{#if r.kind === 'chão'}
						<button
							class="btn btn-s"
							type="button"
							disabled={off}
							onclick={() => act('pick', 'ground_pickup', { id: r.id })}
							>{lbl('pick', 'Pegar')}</button
						>
					{:else}
						{#each SUGGESTED[r.kind] as a (a)}
							<button
								class="btn btn-g btn-s"
								type="button"
								disabled={off}
								onclick={() => interact(r, a)}>{lbl(`do:${a}`, a)}</button
							>
						{/each}
					{/if}
				</div>
				{#if r.kind !== 'chão'}
					<form
						class="acts"
						onsubmit={(e) => {
							e.preventDefault();
							if (action.trim()) interact(r, action.trim());
						}}
					>
						<input
							class="txt"
							placeholder="outra ação do menu"
							aria-label="Ação"
							bind:value={action}
						/>
						<button class="btn btn-s" type="submit" disabled={off || !action.trim()}>Fazer</button>
					</form>
				{/if}
			{/if}
			<form class="acts walk" onsubmit={walk}>
				<span class="k">andar até</span>
				<input class="num" placeholder="x" aria-label="x" bind:value={walkTo.x} />
				<input class="num" placeholder="y" aria-label="y" bind:value={walkTo.y} />
				<input class="num sm" placeholder="0" aria-label="plano" bind:value={walkTo.plane} />
				<button class="btn btn-s" type="submit" disabled={off || !walkTo.x || !walkTo.y}
					>{lbl('walk', 'Andar')}</button
				>
				{#if me}<span class="su tiny" title="Posição atual">você: {me.x}, {me.y}, {me.plane}</span
					>{/if}
			</form>
		</div>
	</section>
</div>

<div class="jogo">
	{#if dialogue}
		<section class="panel wide">
			<div class="plaque">Diálogo</div>
			<div class="pad stack">
				<p class="sm0">{dialogue.text || '…'}</p>
				<div class="acts">
					{#each dialogue.options as o, n (n)}
						<button
							class="btn btn-g btn-s"
							type="button"
							disabled={off}
							onclick={() => act(`opt:${n}`, 'dialogue_select', { index: n + 1 })}
							>{lbl(`opt:${n}`, `${n + 1}. ${o}`)}</button
						>
					{/each}
					{#if dialogue.more}
						<button
							class="btn btn-s"
							type="button"
							disabled={off}
							onclick={() => act('cont', 'dialogue_continue')}>{lbl('cont', 'Continuar')}</button
						>
					{/if}
				</div>
			</div>
		</section>
	{/if}
	<section class="panel">
		<div class="plaque">Por perto<span class="r">até 15 tiles</span></div>
		<div class="list">
			{#each near as r (key(r))}
				<button
					class="nr"
					class:on={sel?.kind === 'near' && key(sel.row) === key(r)}
					type="button"
					onclick={() => pick({ kind: 'near', row: r })}
					title={r.position ? `${r.position.x}, ${r.position.y}, ${r.position.plane}` : undefined}
				>
					<span class="kind">{r.kind}</span><span class="nm">{r.name}</span><span class="d"
						>{r.detail}</span
					><span class="dist">{r.distance ?? '?'}</span>
				</button>
			{:else}
				<p class="su sm0 pad">Nada por perto.</p>
			{/each}
		</div>
	</section>
	<section class="panel">
		<div class="plaque">
			Script ativo<span class="r">state machines</span>
		</div>
		<div class="pad stack">
			{#each machines as m (m.name)}
				<div class="kv">
					<span class="k">{m.name}</span><span class="val mono">{m.state}</span><span
						class="su tiny">{m.detail}</span
					>
				</div>
			{/each}
			{#each heartbeats as h (h.scriptClassName)}
				<div class="kv">
					<span class="k">{short(h.scriptClassName)}</span><span class="val"
						>{h.status === 'NO_HEARTBEAT'
							? 'sem heartbeat'
							: (h.status ?? 'ok').toLowerCase()}{h.loopCount
							? ` · ${h.loopCount} loops`
							: ''}</span
					>
				</div>
			{/each}
			{#if !machines.length && !heartbeats.length}
				<span class="su sm0">Nenhum script de loop rodando.</span>
			{/if}
		</div>
	</section>
	<section class="panel wide">
		<div class="plaque">
			Banco<span class="r"
				>{bank?.open ? `aberto · ${bank.count ?? bank.items?.length ?? 0} itens` : 'fechado'}</span
			>
		</div>
		<div class="pad stack">
			<div class="acts">
				{#if bank?.open}
					<button
						class="btn btn-s btn-d"
						type="button"
						disabled={off}
						onclick={() => act('depall', 'bank_deposit', { all: true }, true)}
						>{lbl('depall', 'Depositar tudo')}</button
					>
					<button
						class="btn btn-g btn-s"
						type="button"
						disabled={off}
						onclick={() => act('close', 'bank_close')}>{lbl('close', 'Fechar banco')}</button
					>
				{:else}
					<button
						class="btn btn-s"
						type="button"
						disabled={off}
						onclick={() => act('open', 'bank_open')}
						>{lbl('open', 'Abrir o banco mais perto')}</button
					>
				{/if}
			</div>
			{#if bank?.open}
				<div class="ctr">
					<div class="g8">
						{#each bank.items ?? [] as i (i.id)}{@render cell(i, () =>
								pick({ kind: 'bank', item: i })
							)}{/each}
					</div>
				</div>
			{/if}
		</div>
	</section>
	<section class="panel wide">
		<div class="plaque">
			Skills<span class="r"
				>{view?.skills?.totalLevel ? `${view.skills.totalLevel} total` : ''}</span
			>
		</div>
		{#if skills.length}
			<div class="sk">
				{#each skills as s (s.skill)}
					{@const icon = iconFor(`skill:${s.skill}`)}
					<span
						class="skc"
						title={`${cap(s.skill)} ${s.level} · ${s.xp.toLocaleString('pt-BR')} xp`}
						>{#if icon}<img src={icon} alt="" />{/if}<span class="nm">{cap(s.skill)}</span><span
							class="lv"
							class:boost={s.boosted !== s.level}
							>{s.boosted !== s.level ? `${s.boosted}/${s.level}` : s.level}</span
						></span
					>
				{/each}
			</div>
		{:else}
			<div class="emp"><span>sem leitura</span></div>
		{/if}
	</section>
	<section class="panel wide">
		<div class="plaque">
			Chat do jogo<span class="r">mensagens do jogo, nunca de jogadores</span>
		</div>
		<div class="pad chat">
			{#each chat as c (c.seq)}
				<div class="cl"><span class="t">{c.at}</span><span>{c.text}</span></div>
			{:else}
				<span class="su sm0">Sem mensagens.</span>
			{/each}
		</div>
	</section>
</div>
<p class="su tiny attr">{ICON_ATTRIBUTION}</p>

<style>
	.jogo {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 14px;
		align-items: start;
	}
	.wide {
		grid-column: 1 / -1;
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		gap: 14px;
		align-items: stretch;
		margin-bottom: 14px;
	}
	.top > :global(.panel) {
		flex: 0 0 auto;
	}
	.top > .act {
		flex: 1 1 300px;
		min-width: 0;
	}
	.pad {
		padding: 12px;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
	}
	.walk {
		border-top: 1px solid var(--hairline);
		padding-top: 10px;
	}
	.num,
	.txt {
		height: 30px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.num {
		width: 64px;
	}
	.num.sm {
		width: 36px;
	}
	.txt {
		flex: 1 1 140px;
		min-width: 0;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.g8 {
		display: grid;
		grid-template-columns: repeat(auto-fill, 40px);
		gap: 3px;
		width: 100%;
		max-height: 260px;
		overflow: auto;
	}
	.ctr .it:disabled {
		cursor: default;
	}
	.ctr button.it {
		cursor: pointer;
		padding: 0;
	}
	.slotcell {
		display: flex;
	}
	.lv.boost {
		color: var(--gold-ink);
	}
	.list {
		max-height: 260px;
		overflow: auto;
	}
	.nr {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		box-sizing: border-box;
		padding: 6px 12px;
		border: 0;
		border-bottom: 1px solid var(--hairline);
		background: transparent;
		color: var(--ink);
		font: inherit;
		font-size: 12px;
		text-align: left;
		cursor: pointer;
	}
	.nr:hover,
	.nr.on {
		background: var(--gold-tint);
	}
	.nr .kind {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-subtle);
		width: 46px;
		flex: 0 0 auto;
	}
	.nr .nm {
		font-weight: 500;
	}
	.nr .d {
		flex: 1;
		min-width: 0;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nr .dist {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
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
	.val {
		font-size: 13px;
		font-weight: 500;
	}
	.mono {
		font-family: 'JetBrains Mono', monospace;
	}
	.chat {
		display: flex;
		flex-direction: column;
		gap: 4px;
		max-height: 180px;
		overflow: auto;
	}
	.cl {
		display: flex;
		gap: 10px;
		font-size: 12px;
	}
	.cl .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		flex: 0 0 auto;
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
	.tiny {
		font-size: 11px;
		margin: 0;
	}
	.attr {
		margin-top: 6px;
	}
</style>
