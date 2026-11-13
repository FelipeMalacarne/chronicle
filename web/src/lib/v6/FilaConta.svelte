<script lang="ts">
	import { resolve } from '$app/paths';
	import { microbotApi, type BreakState, type Run } from '#lib/api/microbot';
	import { itemIcon } from '#lib/progress/icons';
	import { microbotModule } from '#lib/progress/microbot.svelte';
	import { accountQueue, accountRows, CLOCKS, type RoutineRow } from '#lib/progress/rotinas';
	import Glyph from './Glyph.svelte';

	/*
	 * A Fila da conta na Visão geral (v7, fatia V6; artboard V7Conta1440): o que acontece agora, as
	 * rotinas que entram a seguir e com hora, a próxima pausa do Break Handler e os gatilhos. Só com
	 * o módulo Microbot ligado; as rotinas se editam na tela Rotinas. Sem a atividade principal e sem
	 * "Pôr na fila", que o servidor não tem (decisoes-canvas.md, seção 7.8).
	 */
	let { account }: { account: string } = $props();

	let rows = $state<RoutineRow[]>([]);
	let runs = $state<Run[]>([]);
	let brk = $state<BreakState | undefined>(undefined);
	let loaded = $state(false);
	let now = $state(new Date());

	async function load(id: string) {
		const quiet = <T,>(p: Promise<T>): Promise<T | null> => p.catch(() => null);
		const [recurring, triggers, history, status] = await Promise.all([
			quiet(microbotApi.recurringOf(id)),
			quiet(microbotApi.triggersOf(id)),
			quiet(microbotApi.runsOf(id)),
			quiet(microbotApi.statusOf(id))
		]);
		if (id !== account) return;
		now = new Date();
		runs = history?.runs ?? [];
		rows = accountRows(recurring, triggers, runs, now);
		brk = status?.client?.break;
		loaded = true;
	}
	$effect(() => {
		const id = account;
		if (!id || microbotModule.enabled !== true) return;
		loaded = false;
		void load(id);
		const t = setInterval(() => {
			if (!document.hidden) void load(id);
		}, 30_000);
		return () => clearInterval(t);
	});

	const items = $derived(accountQueue(rows, runs, brk, now));
	const icon = (clock?: string) => itemIcon(CLOCKS.find((c) => c.key === clock)?.item);
	const link = (key: string) => `${resolve('/rotinas')}?rotina=${encodeURIComponent(key)}`;
</script>

{#if microbotModule.enabled}
	<section class="card">
		<div class="h">
			<h2>Fila</h2>
			<span>o que esta conta vai fazer</span>
		</div>
		<div class="b">
			{#if items.length}
				<div class="tl">
					{#each items as i (i.key)}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() + query da rotina -->
						<svelte:element
							this={i.routine ? 'a' : 'div'}
							class="tl-i"
							class:cur={i.cur}
							href={i.routine ? link(i.routine) : undefined}
						>
							<span class="t">{i.at}</span>
							<span class="n"
								>{#if icon(i.clock)}<img src={icon(i.clock)} alt="" />{:else}<Glyph
										name={i.glyph}
										size={12}
									/>{/if}</span
							>
							<span class="x"><b>{i.title}</b><span>{i.sub}</span></span>
						</svelte:element>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{/each}
				</div>
			{:else}
				<p class="mu">
					{loaded ? 'Nada na fila: nenhuma rotina ligada com hora.' : 'Lendo a fila…'}
				</p>
			{/if}
			<div class="row">
				<a class="btn btn-s btn-g" href={resolve('/rotinas')}>Editar rotinas</a>
			</div>
		</div>
	</section>
{/if}

<style>
	.card {
		display: flex;
		flex-direction: column;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		min-width: 0;
	}
	.h {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 12px 16px 0;
	}
	.h h2 {
		font:
			700 17px/1.2 Alegreya,
			Georgia,
			serif;
		margin: 0;
	}
	.h span {
		margin-left: auto;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		white-space: nowrap;
	}
	.b {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 12px 16px 16px;
		font-size: 13px;
	}
	.b p {
		margin: 0;
		line-height: 1.45;
	}
	.row {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.tl {
		display: flex;
		flex-direction: column;
		position: relative;
	}
	.tl-i {
		display: grid;
		grid-template-columns: 72px 26px minmax(0, 1fr);
		gap: 10px;
		align-items: center;
		min-height: 52px;
		position: relative;
		color: var(--ink);
		text-decoration: none;
	}
	a.tl-i:hover .x b {
		text-decoration: underline;
	}
	.tl-i .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		text-align: right;
	}
	.tl-i .n {
		position: relative;
		width: 26px;
		height: 26px;
		border-radius: 50%;
		background: var(--ground);
		border: 1px solid var(--border);
		display: flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		z-index: 1;
		color: var(--ink-subtle);
	}
	.tl-i .n img {
		width: 16px;
		height: 16px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.tl-i:not(:last-child)::after {
		content: '';
		position: absolute;
		left: 94px;
		top: 39px;
		bottom: -13px;
		width: 2px;
		background: var(--hairline);
	}
	.tl-i .x {
		display: flex;
		flex-direction: column;
		min-width: 0;
		line-height: 1.3;
	}
	.tl-i .x b {
		font-size: 13px;
		font-weight: 600;
	}
	.tl-i .x span {
		font-size: 11px;
		color: var(--ink-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tl-i.cur .n {
		border-color: var(--gold-ink);
		box-shadow: 0 0 0 3px var(--gold-tint);
	}
	.tl-i.cur .t {
		color: var(--gold-ink);
		font-weight: 600;
	}
</style>
