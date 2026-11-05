<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { itemIcon } from '#lib/progress/icons';
	import { AHEAD_DEFAULT, AHEAD_HORIZONS, aheadView } from '#lib/progress/view';
	import type { Ahead, BankAction } from '#lib/progress/types';

	/**
	 * Prévia pelos próximos passos quando o guia não lista o setup do trecho (artboard
	 * V4AgoraOzirisBanco). É derivada, nunca readiness: não diz "pronto" e as ações de banco
	 * continuam instrução (regra 8).
	 */
	let {
		ahead,
		bankActions,
		firstRef,
		bankLabel,
		bankCls
	}: {
		ahead: Ahead;
		bankActions: BankAction[];
		firstRef: string;
		bankLabel: string;
		bankCls: string;
	} = $props();

	let horizon = $state<number>(AHEAD_DEFAULT);
	const v = $derived(aheadView(ahead, horizon));
	const options = $derived(
		AHEAD_HORIZONS.filter((n) => n === 0 || n < ahead.steps).map((n) => ({
			n,
			label: n === 0 ? `até o banco · ${ahead.steps}` : `${n} passos`
		}))
	);
	const active = $derived(horizon > 0 && horizon < ahead.steps ? horizon : 0);
</script>

<div class="top">
	<div class="sum">
		<span class="small">O que os passos pedem a partir de {ahead.from_step_id}, até {v.until}.</span
		>
		<div class="cnt">
			<span><b>{v.counts.total}</b> para levar</span>
			<span><b>{v.counts.you}</b> com você</span>
			<span><b>{v.counts.have}</b> no banco</span>
			<span class="miss"><b>{v.counts.miss}</b> faltam</span>
			<span><b>{v.counts.unk}</b> não verificáveis</span>
		</div>
	</div>
	{#if options.length > 1}
		<div class="seg" role="group" aria-label="Quantos passos olhar">
			{#each options as o (o.n)}
				<button
					type="button"
					class:on={active === o.n}
					aria-pressed={active === o.n}
					onclick={() => (horizon = o.n)}>{o.label}</button
				>
			{/each}
		</div>
	{/if}
	<span class="badge badge-unk">prévia · nunca “pronto”</span>
</div>
<div class="cols">
	<div class="col">
		<div class="h">
			<p class="ey">Levar do banco</p>
			<span class="fr {bankCls} push"><span class="d"></span>{bankLabel}</span>
		</div>
		{#each v.groups as g (g.stepId)}
			<div class="grp">
				<div class="gh">
					<span class="id">{g.stepId}</span><span class="tx">{g.text}</span><span class="w"
						>{g.when}</span
					>
				</div>
				<div class="slots">
					{#each g.slots as s, i (i)}
						{@const icon = itemIcon(s.itemId)}
						<span class="slot {s.cls}" title={s.tip || undefined}>
							<span class="bx">
								{#if icon}<img class="sp" src={icon} alt="" />{/if}
								<span class="q">{s.qty}</span>
								{#if s.ok}<span class="ok"><Glyph name="check" size={10} /></span>{/if}
								{#if s.cls === 'slot-unk'}<span class="qm">?</span>{/if}
							</span>
							<span class="nm">{s.name}</span>
							<span class="stt">{s.state}</span>
						</span>
					{/each}
				</div>
			</div>
		{:else}
			<span class="su small">Nenhum passo desta janela lista itens.</span>
		{/each}
	</div>
	<div class="col side">
		<div class="col">
			<div class="h"><p class="ey">Consegue no caminho</p></div>
			{#each v.obtained as o, i (i)}
				{@const icon = itemIcon(o.itemId)}
				<div class="way">
					{#if icon}<img src={icon} alt="" />{/if}{o.name}<span class="w"
						>{o.from} obtém<br />usa no {o.use}</span
					>
				</div>
			{:else}
				<span class="su small">Nada nesta janela.</span>
			{/each}
			<span class="su tiny"
				>Um passo anterior da janela obtém o item. Não precisa sair do banco com ele.</span
			>
		</div>
		<div class="col">
			<div class="h"><p class="ey">O guia manda</p></div>
			{#each bankActions as a, i (i)}
				<span class="quote">“{a.text}”</span>
			{:else}
				<span class="su small">Nenhuma ação de banco neste trecho.</span>
			{/each}
			{#if bankActions.length}
				<span class="su tiny">instrução do passo {firstRef} · nunca é marcada como feita</span>
			{/if}
		</div>
	</div>
</div>
<p class="su tiny foot">
	Prévia derivada, não instrução do guia: soma dos itens listados nos passos da janela, sem os que
	um passo anterior dela obtém. Quantidade sem número conta 1. Itens de quest que o guia não anota
	como obtidos aparecem como falta até você pegá-los.
</p>

<style>
	.top {
		padding: 14px 18px 4px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 16px;
	}
	.sum {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1 1 360px;
		min-width: 0;
	}
	.cnt {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 14px;
		font:
			400 11px 'JetBrains Mono',
			ui-monospace,
			monospace;
		color: var(--ink-subtle);
	}
	.cnt b {
		font-weight: 600;
		color: var(--ink);
	}
	.cnt .miss,
	.cnt .miss b {
		color: var(--not-started);
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--border);
		border-radius: 4px;
		overflow: hidden;
		box-shadow: var(--bevel);
	}
	.seg button {
		height: 30px;
		padding: 0 12px;
		border: 0;
		background: var(--elevated);
		color: var(--ink-muted);
		font-family: inherit;
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
	}
	.seg button + button {
		border-left: 1px solid var(--border);
	}
	.seg .on {
		background: var(--frame);
		color: var(--gold);
	}
	.badge {
		border: 1px dotted var(--ink-subtle);
	}
	.cols {
		padding: 10px 18px 12px;
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
		gap: 20px;
	}
	@media (max-width: 760px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.col {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}
	.side {
		gap: 14px;
	}
	.h {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.push {
		margin-left: auto;
	}
	.grp {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 0;
		border-top: 1px solid var(--hairline);
	}
	.grp:first-of-type {
		border-top: 0;
		padding-top: 0;
	}
	.gh {
		display: flex;
		align-items: baseline;
		gap: 10px;
		min-width: 0;
	}
	.gh .id,
	.gh .w {
		font:
			400 11px 'JetBrains Mono',
			ui-monospace,
			monospace;
		color: var(--ink-subtle);
		flex: 0 0 auto;
	}
	.gh .tx {
		font-size: 13px;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.gh .w {
		margin-left: auto;
		white-space: nowrap;
	}
	.slots {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 6px;
	}
	.stt {
		font:
			500 10px 'JetBrains Mono',
			ui-monospace,
			monospace;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		text-align: center;
	}
	.slot-you .stt {
		color: var(--done);
	}
	.slot-miss .stt {
		color: var(--not-started);
	}
	.way {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 32px;
		font-size: 13px;
	}
	.way img {
		width: 22px;
		height: 22px;
		object-fit: contain;
		image-rendering: pixelated;
		flex: 0 0 auto;
	}
	.way .w {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			ui-monospace,
			monospace;
		color: var(--ink-subtle);
		text-align: right;
	}
	.small {
		font-size: 12px;
	}
	.tiny {
		font-size: 11px;
	}
	.quote {
		font-size: 13px;
	}
	.foot {
		margin: 0;
		padding: 0 18px 14px;
		max-width: 110ch;
	}
</style>
