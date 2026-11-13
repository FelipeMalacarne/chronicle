<script lang="ts">
	import type { Snippet } from 'svelte';
	import { iconFor } from '#lib/progress/icons';
	import { recurringRows, timerRef } from '#lib/progress/relogios';
	import type { TimerView } from '#lib/progress/types';
	import { ago } from '#lib/progress/view';

	/**
	 * O cartão Relógios da v7 (V7Rota1440, V7Rota640, V7Fundamentos): os relógios do Time Tracking
	 * que o plugin mandou, do que está pronto ao que falta mais, com a idade da leitura. Sem relógio
	 * contando, o cartão some (ou diz `empty`, quando a tela pede uma linha).
	 */
	let {
		timers,
		now,
		title = 'Relógios',
		short = false,
		empty,
		children
	}: {
		timers: TimerView[];
		/** Milissegundos desde a época. */
		now: number;
		title?: string;
		/** A 640: "há 12 s" sem o nome da fonte. */
		short?: boolean;
		empty?: string;
		children?: Snippet;
	} = $props();

	const rows = $derived(
		recurringRows(
			timers.map((t) => ({
				key: t.key,
				label: t.label ?? t.key,
				state: t.state,
				readyAt: t.ready_at
			})),
			now / 1000
		)
	);
	const src = $derived.by(() => {
		const at = timers.reduce((m, t) => Math.max(m, Date.parse(t.observed_at) || 0), 0);
		if (!at) return '';
		return short ? ago(at, now) : `Time Tracking · ${ago(at, now)}`;
	});
	const hm = (s: number) =>
		new Date(s * 1000).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
</script>

{#if rows.length || empty || children}
	<section class="card">
		<div class="h">
			<h2>{title}</h2>
			{#if src}<span>{src}</span>{/if}
		</div>
		<div class="b">
			{#if rows.length}
				<div>
					{#each rows as r (r.key)}
						{@const icon = iconFor(timerRef(r.key))}
						<div class="cl7" title={r.at ? `pronto às ${hm(r.at)}` : undefined}>
							{#if icon}<img src={icon} alt="" />{:else}<span></span>{/if}<span>{r.label}</span
							><span class="w" class:rdy={r.ready}>{r.when.replace(/^pronto em /, '')}</span>
						</div>
					{/each}
				</div>
			{:else if empty}
				<p class="mu">{empty}</p>
			{/if}
			{@render children?.()}
		</div>
	</section>
{/if}
