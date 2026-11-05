<script lang="ts">
	import Glyph from './Glyph.svelte';
	import { openEntity } from '#lib/progress/entity.svelte';
	import { itemIcon } from '#lib/progress/icons';
	import {
		AHEAD_DEFAULT,
		AHEAD_HORIZONS,
		aheadView,
		ago,
		bankColumns,
		type AheadSlot
	} from '#lib/progress/view';
	import type { Ahead, BankAction, SegmentReadiness } from '#lib/progress/types';

	/**
	 * "Levar do banco" (V6Agora1440/640; regras em V6Fundamentos, átomo 2): uma frase de resumo, a
	 * grade com os que faltam primeiro e uma linha de rodapé. Com você / no banco / falta / sem id
	 * estão na borda do slot. Sem setup do guia é a prévia pelos próximos passos, nunca "pronto"
	 * (a explicação fica no título do link); com setup, a plaque diz "Setup do trecho". Ações de
	 * banco são instrução e nunca viram passo feito; ausência só com container live (regra 8).
	 */
	let {
		readiness,
		ahead = null,
		bankActions,
		firstRef,
		refOf
	}: {
		readiness: SegmentReadiness | null;
		ahead?: Ahead | null;
		bankActions: BankAction[];
		firstRef: string;
		/** Referência do autor de um passo (o id interno nunca aparece). */
		refOf: (id: string) => string;
	} = $props();

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 15_000);
		return () => clearInterval(t);
	});

	let horizon = $state<number>(AHEAD_DEFAULT);
	const view = $derived(ahead ? aheadView(ahead, horizon) : null);
	const options = $derived(
		ahead
			? AHEAD_HORIZONS.filter((n) => n === 0 || n < ahead.steps).map((n) => ({
					n,
					label: n === 0 ? `até o banco · ${ahead.steps}` : `${n} passos`
				}))
			: []
	);
	const active = $derived(ahead && horizon > 0 && horizon < ahead.steps ? horizon : 0);

	/** Setup listado pelo guia: os mesmos slots, num grupo só. */
	const setup = $derived.by((): AheadSlot[] => {
		if (ahead || !readiness?.items.length) return [];
		const cols = bankColumns(readiness);
		return cols.slots.map((s, i) => {
			const it = readiness.items[i];
			const carried = it.carried >= Math.max(1, it.required);
			const held = it.owned?.held ?? 0;
			const need = it.qty_stated || it.required > 1 ? String(it.required) : '';
			return {
				name: s.name,
				itemId: s.itemId,
				ref: it.family
					? `item:family/${it.family}`
					: s.itemId !== undefined
						? `item:${s.itemId}`
						: '',
				qty: s.cls === 'slot-miss' ? (held > 0 ? `${held}/${it.required}` : need) : s.qty,
				held: held || undefined,
				required: it.required,
				cls: carried ? 'slot-you' : s.cls,
				state: carried ? 'com você' : s.suffix || 'no banco',
				ok: carried,
				tip: ''
			};
		});
	});

	interface Group {
		id: string;
		ref: string;
		text: string;
		when: string;
		slots: AheadSlot[];
	}
	const groups = $derived.by((): Group[] => {
		if (view)
			return view.groups.map((g, i) => ({
				id: g.stepId,
				ref: refOf(g.stepId),
				text: g.text,
				// "em 2 passos", depois só "em 4", "em 17" (V6Agora1440).
				when: i === 0 ? g.when : g.when.replace(/ passos$/, ''),
				slots: g.slots
			}));
		if (setup.length)
			return [
				{
					id: readiness?.first_step_id ?? '',
					ref: refOf(readiness?.first_step_id ?? ''),
					text: 'setup do trecho',
					when: 'o guia lista',
					slots: setup
				}
			];
		return [];
	});
	const missing = $derived(
		groups.flatMap((g) =>
			g.slots.filter((s) => s.cls === 'slot-miss').map((s) => ({ s, ref: g.ref }))
		)
	);
	const rest = $derived(
		groups
			.map((g) => ({ ...g, slots: g.slots.filter((s) => s.cls !== 'slot-miss') }))
			.filter((g) => g.slots.length > 0)
	);
	const total = $derived(groups.reduce((n, g) => n + g.slots.length, 0));
	const noId = $derived(
		groups.reduce((n, g) => n + g.slots.filter((s) => s.state === 'sem id').length, 0)
	);
	const partial = $derived(readiness?.loadout_status === 'partial');

	/** Uma linha de frescor: "banco e inventário ao vivo · 40 s". */
	const fresh = $derived.by(() => {
		const f = readiness?.freshness ?? {};
		const live = (['bank', 'inventory'] as const).filter((c) => f[c]?.live);
		const name = { bank: 'banco', inventory: 'inventário' } as const;
		if (live.length === 0) {
			const restored = (['bank', 'inventory'] as const).some((c) => f[c]?.state === 'restored');
			return { text: restored ? 'espelho restaurado' : 'sem leitura do plugin', cls: 'd-w' };
		}
		const at = Math.min(...live.map((c) => Date.parse(f[c]?.observed_at ?? '') || now));
		return {
			text: `${live.map((c) => name[c]).join(' e ')} ao vivo · ${ago(at, now).replace('há ', '')}`,
			cls: ''
		};
	});

	const summary = $derived.by(() => {
		if (view) {
			const span = active
				? `nos próximos ${active} passos`
				: `até ${refOf(ahead?.until_step_id ?? '')}`;
			return { lead: `${total} ${total === 1 ? 'item' : 'itens'}`, span };
		}
		return {
			lead: `${total} ${total === 1 ? 'item' : 'itens'}`,
			span: partial ? 'no setup · listado em parte' : 'no setup do trecho'
		};
	});
	const plaqueRight = $derived(
		view
			? active
				? `${active} passos à frente`
				: `até ${refOf(ahead?.until_step_id ?? '')} · fim do trecho`
			: readiness?.first_step_id
				? `trecho de ${refOf(readiness.first_step_id)}`
				: ''
	);
	const way = $derived(view?.obtained ?? []);
	const SEP = ' · ';
	const HOW =
		'Prévia derivada: o guia não lista o setup deste trecho; soma dos itens que os passos da janela pedem, sem os que um passo anterior obtém. Quantidade sem número conta 1. Nunca diz "pronto".';
	const slotTitle = (s: AheadSlot, ref: string) =>
		[
			s.name,
			s.cls === 'slot-miss' && ref ? `${s.required} no ${ref}` : '',
			s.held ? `você tem ${s.held}` : '',
			s.state,
			s.tip
		]
			.filter(Boolean)
			.join(' · ');
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- os hrefs internos vêm de resolve() + âncora; os externos são da wiki -->

{#snippet slot(s: AheadSlot, ref: string)}
	{@const icon = itemIcon(s.itemId)}
	<button
		class="slot {s.cls}"
		type="button"
		title={slotTitle(s, ref)}
		disabled={!s.ref}
		onclick={() => openEntity(s.ref)}
	>
		<span class="bx">
			{#if icon}<img class="sp" src={icon} alt="" />{/if}
			{#if s.qty}<span class="q">{s.qty}</span>{/if}
			{#if s.ok}<span class="ok"><Glyph name="check" size={11} /></span>{/if}
		</span>
		<span class="nm">{s.name}</span>
	</button>
{/snippet}

{#if total > 0 || bankActions.length}
	<section class="panel">
		<div class="plaque">
			{view || !setup.length ? 'Levar do banco' : 'Setup do trecho'}<span class="r"
				>{plaqueRight}</span
			>
		</div>
		<div class="sum">
			<span
				><b>{summary.lead}</b>
				{summary.span}{#if missing.length}{SEP}<b class="red"
						>{missing.length} {missing.length === 1 ? 'falta' : 'faltam'}</b
					>{/if}{#if noId}{` · ${noId} sem id`}{/if}</span
			>
			<span class="fr push"><span class="d {fresh.cls}"></span>{fresh.text}</span>
		</div>
		{#if view}
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
				<span class="how push" title={HOW}>prévia do guia, não setup · como é calculada</span>
			</div>
		{/if}
		<div class="grid">
			{#if missing.length}
				<div class="grp first">
					<div class="gh">
						<span class="lbl red">Faltam</span><span class="w">pegue antes de sair do banco</span>
					</div>
					<div class="slots">
						{#each missing as m, i (i)}{@render slot(m.s, m.ref)}{/each}
					</div>
				</div>
			{/if}
			{#each rest as g, i (g.id + i)}
				<div class="grp" class:first={i === 0 && !missing.length}>
					<div class="gh">
						<span class="id">{g.ref}</span><span class="tx" title={g.text}>{g.text}</span><span
							class="w">{g.when}</span
						>
					</div>
					<div class="slots">
						{#each g.slots as s, j (j)}{@render slot(s, g.ref)}{/each}
					</div>
				</div>
			{/each}
			{#if way.length || bankActions.length}
				<p class="su foot">
					{#if way.length}No caminho, sem passar no banco: {way
							.map((o) => `${o.name} (${refOf(o.from)})`)
							.join(', ')}.{/if}
					{#if bankActions.length}O guia manda: {bankActions.map((a) => `“${a.text}”`).join(', ')} no
						{firstRef}.{/if}
				</p>
			{/if}
		</div>
	</section>
{/if}

<style>
	section {
		display: flex;
		flex-direction: column;
	}
	.sum {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 14px;
		padding: 12px 16px 6px;
		font-size: 13px;
	}
	.ctl {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 0 16px 10px;
	}
	.push {
		margin-left: auto;
	}
	.how {
		font-size: 12px;
		color: var(--gold-ink);
		text-decoration: underline;
		text-underline-offset: 3px;
		cursor: help;
	}
	.grid {
		padding: 0 16px 12px;
		display: flex;
		flex-direction: column;
	}
	.first {
		border-top: 0;
		padding-top: 0;
	}
	.slots {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 4px;
	}
	button.slot {
		border: 0;
		padding: 0;
	}
	button.slot:disabled {
		cursor: default;
	}
	.foot {
		margin: 8px 0 0;
		font-size: 12px;
		border-top: 1px solid var(--hairline);
		padding-top: 8px;
	}
</style>
