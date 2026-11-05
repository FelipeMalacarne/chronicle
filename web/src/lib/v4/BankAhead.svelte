<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { itemIcon } from '#lib/progress/icons';
	import { AHEAD_DEFAULT, AHEAD_HORIZONS, aheadView } from '#lib/progress/view';
	import type { Ahead, BankAction } from '#lib/progress/types';
	import type { AheadSlot } from '#lib/progress/view';

	/**
	 * Prévia pelos próximos passos quando o guia não lista o setup do trecho, num registro só
	 * (v6, V6Agora640): uma frase de resumo, o horizonte, os que faltam primeiro, os grupos por
	 * passo com o estado na borda do slot, e uma linha de rodapé só quando há algo a dizer. É
	 * derivada, nunca readiness: não diz "pronto" e as ações de banco continuam instrução (regra 8).
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
	const HOW =
		'Prévia derivada, não instrução do guia: soma dos itens que os passos da janela pedem, sem os que um passo anterior dela obtém. Quantidade sem número conta 1. Itens de quest que o guia não anota como obtidos aparecem como falta até você pegá-los. Nunca diz "pronto".';
	const foot = $derived.by(() => {
		const parts: string[] = [];
		if (v.obtained.length)
			parts.push(
				`No caminho, sem passar no banco: ${v.obtained.map((o) => `${o.name} (${o.from})`).join(', ')}.`
			);
		if (bankActions.length)
			parts.push(
				`O guia manda: ${bankActions.map((a) => `“${a.text}”`).join(' · ')} no ${firstRef}.`
			);
		return parts.join(' ');
	});
</script>

{#snippet slot(s: AheadSlot)}
	{@const icon = itemIcon(s.itemId)}
	<span class="slot {s.cls}" title={[s.name, s.state, s.tip].filter(Boolean).join(' · ')}>
		<span class="bx">
			{#if icon}<img class="sp" src={icon} alt="" />{/if}
			<span class="q">{s.qty}</span>
			{#if s.ok}<span class="ok"><Glyph name="check" size={11} /></span>{/if}
		</span>
		<span class="nm">{s.name}</span>
	</span>
{/snippet}

<div class="sum">
	<span class="small"
		><b>{v.summary.items}</b>
		{v.summary.window}{#if v.summary.miss}
			· <b class="red">{v.summary.miss}</b>{/if}{#if v.summary.unk}
			· {v.summary.unk}{/if}</span
	>
	<span class="fr {bankCls} push"><span class="d"></span>{bankLabel}</span>
</div>
<div class="ctl">
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
	<button class="how push small" type="button" title={HOW} aria-label="Como a prévia é calculada"
		>prévia do guia, não setup · como é calculada</button
	>
</div>
<div class="groups">
	{#if v.missing.length}
		<div class="grp first">
			<div class="gh">
				<span class="lbl red">Faltam</span><span class="w">pegue antes de sair do banco</span>
			</div>
			<div class="slots">
				{#each v.missing as s, i (i)}{@render slot(s)}{/each}
			</div>
		</div>
	{/if}
	{#each v.groups as g, gi (g.stepId)}
		<div class="grp" class:first={gi === 0 && !v.missing.length}>
			<div class="gh">
				<span class="id">{g.stepId}</span><span class="tx">{g.text}</span><span class="w"
					>{g.when}</span
				>
			</div>
			<div class="slots">
				{#each g.slots as s, i (i)}{@render slot(s)}{/each}
			</div>
		</div>
	{:else}
		<span class="su small">Nenhum passo desta janela lista itens.</span>
	{/each}
	{#if foot}<p class="su small foot">{foot}</p>{/if}
</div>

<style>
	.sum {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 14px;
		padding: 12px 16px 6px;
	}
	.sum b {
		font-weight: 600;
	}
	.ctl {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 0 16px 10px;
	}
	.how {
		all: unset;
		font-size: 13px;
		color: var(--gold-ink);
		text-decoration: underline dotted;
		text-underline-offset: 3px;
		cursor: help;
	}
	.push {
		margin-left: auto;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--border);
		border-radius: 4px;
		overflow: hidden;
		box-shadow: var(--bevel);
	}
	.seg button {
		height: 32px;
		padding: 0 10px;
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
		box-shadow: var(--bevel-frame);
	}
	.groups {
		padding: 0 16px 12px;
		display: flex;
		flex-direction: column;
	}
	.grp {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 8px 0;
		border-top: 1px solid var(--hairline);
	}
	.grp.first {
		border-top: 0;
		padding-top: 0;
	}
	.gh {
		display: flex;
		align-items: baseline;
		gap: 8px;
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
		gap: 8px 4px;
	}
	.small {
		font-size: 13px;
	}
	.foot {
		margin: 8px 0 0;
		font-size: 12px;
		border-top: 1px solid var(--hairline);
		padding-top: 8px;
	}
</style>
