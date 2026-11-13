<script lang="ts">
	import { resolve } from '$app/paths';
	import { microbotApi, MicrobotError, type GameView, type ScriptEntry } from '#lib/api/microbot';
	import { scriptRows } from '#lib/progress/jogo';
	import { ago } from '#lib/progress/view';

	/*
	 * O cartão Script do Cliente (V7Cliente1440): o plugin que joga, com o estado da state machine e
	 * as voltas, e Pausar (o Agent Server pausa todos os loops), Parar e Configurar. Os plugins que
	 * ficam sempre ligados (Break Handler, Chronicle Link) não aparecem.
	 */
	let {
		scripts,
		view,
		paused,
		onchange
	}: {
		scripts: ScriptEntry[];
		view: GameView | null;
		paused: boolean;
		onchange?: () => void;
	} = $props();

	const rows = $derived(scriptRows(scripts, view));
	let busy = $state<string | null>(null);
	let armed = $state<string | null>(null);
	let error = $state<string | null>(null);
	let started = $state<number | null>(null);
	let now = $state(Date.now());
	let disarm: ReturnType<typeof setTimeout> | undefined;

	// Desde quando o primeiro script joga (o status do Agent Server), relido a cada minuto.
	const first = $derived(rows[0]?.className ?? '');
	$effect(() => {
		const cls = first;
		started = null;
		if (!cls) return;
		const read = async () => {
			try {
				const at = (await microbotApi.scriptDetail(cls)).status?.startedAt;
				started = at ? Date.parse(at) : null;
			} catch {
				started = null;
			}
			now = Date.now();
		};
		void read();
		const t = setInterval(() => {
			if (!document.hidden) void read();
		}, 60_000);
		return () => clearInterval(t);
	});

	async function act(key: string, run: () => Promise<unknown>, confirm = false) {
		if (busy) return;
		if (confirm && armed !== key) {
			armed = key;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = key;
		error = null;
		try {
			await run();
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			onchange?.();
		}
	}
	const pause = () =>
		act('pause', () => microbotApi.plugin({ path: paused ? 'scripts/resume' : 'scripts/pause' }));
	const stop = (className: string) =>
		act(
			`stop:${className}`,
			() => microbotApi.plugin({ path: 'scripts/stop', body: { className } }),
			true
		);
</script>

<section class="card">
	<div class="h">
		<h2>Script</h2>
		{#if started}<span>{ago(started, now)}</span>{/if}
	</div>
	<div class="b">
		{#each rows as r (r.className)}
			<div class="sc">
				<b>{r.name}</b>
				{#if r.detail}<span class="mu">{r.detail}</span>{/if}
			</div>
			<div class="acts">
				<button class="btn btn-s" type="button" disabled={!!busy} onclick={pause}
					>{busy === 'pause' ? '…' : paused ? 'Retomar' : 'Pausar'}</button
				>
				<button
					class="btn btn-s"
					class:btn-d={armed === `stop:${r.className}`}
					type="button"
					disabled={!!busy}
					onclick={() => stop(r.className)}
					>{armed === `stop:${r.className}` ? 'Confirmar' : 'Parar'}</button
				>
				<a class="btn btn-s btn-g" href={resolve('/automacao/plugins')}>Configurar</a>
			</div>
		{:else}
			<p class="mu">Nenhum script jogando.</p>
			<div class="acts">
				<a class="btn btn-s btn-g" href={resolve('/automacao/plugins')}>Plugins</a>
			</div>
		{/each}
		{#if error}<p class="err">{error}</p>{/if}
	</div>
</section>

<style>
	.sc {
		display: flex;
		flex-direction: column;
		line-height: 1.3;
		min-width: 0;
	}
	.sc b {
		font-weight: 600;
	}
	.sc .mu {
		font-size: 12px;
	}
	.acts {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.err {
		font-size: 12px;
		color: var(--not-started);
	}
</style>
