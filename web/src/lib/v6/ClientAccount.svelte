<script lang="ts">
	import { microbotApi, MicrobotError, type ClientAccount } from '#lib/api/microbot';

	/*
	 * O personagem com que o cliente de dev sobe (doc 08, "logs e conta"): `mb-account show` e
	 * `list` pelo agent, só os nomes. Trocar exige o cliente parado (vale no próximo início) e pede
	 * um segundo clique; fica na auditoria.
	 */
	let { running, onchange }: { running: boolean; onchange?: () => void } = $props();

	let acc = $state<ClientAccount | null>(null);
	let loading = $state(false);
	let busy = $state<number | null>(null);
	let armed = $state<number | null>(null);
	let error = $state<string | null>(null);
	let disarm: ReturnType<typeof setTimeout> | undefined;

	async function load() {
		loading = true;
		try {
			acc = await microbotApi.account();
			error = acc.error ?? null;
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			loading = false;
		}
	}
	$effect(() => {
		void load();
	});

	async function use(n: number) {
		if (busy !== null) return;
		if (armed !== n) {
			armed = n;
			clearTimeout(disarm);
			disarm = setTimeout(() => (armed = null), 4000);
			return;
		}
		armed = null;
		busy = n;
		error = null;
		try {
			await microbotApi.accountUse(n);
		} catch (e) {
			error = e instanceof MicrobotError ? e.message : String(e);
		} finally {
			busy = null;
			await load();
			onchange?.();
		}
	}
</script>

<section class="card">
	<div class="h">
		<h2>Conta do cliente</h2>
		<span>vale no próximo início</span>
	</div>
	<div class="pad stack">
		{#if !acc}
			<span class="su sm0" aria-busy="true">{loading ? 'Lendo…' : (error ?? '')}</span>
		{:else}
			<div class="kv">
				<span class="k">Personagem</span><span class="val">{acc.current || '—'}</span>
			</div>
			{#if acc.characters?.length}
				<div class="list">
					{#each acc.characters ?? [] as c, i (i)}
						<div class="row">
							<span class="nm" class:cur={c === acc.current}>{c}</span>
							{#if c !== acc.current}
								<button
									class="btn btn-g btn-s"
									class:btn-d={armed === i + 1}
									type="button"
									disabled={running || busy !== null}
									title={running
										? 'Pare o cliente para trocar: o personagem vale no próximo início'
										: 'O cliente sobe com este personagem no próximo início'}
									onclick={() => use(i + 1)}
									>{busy === i + 1 ? 'Trocando…' : armed === i + 1 ? 'Confirmar' : 'Usar'}</button
								>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
			{#if error}<p class="err">{error}</p>{/if}
			<button class="btn btn-g btn-s self" type="button" disabled={loading} onclick={load}
				>{loading ? 'Lendo…' : 'Reler'}</button
			>
		{/if}
	</div>
</section>

<style>
	.pad {
		padding: 12px;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: 10px;
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
	.list {
		display: flex;
		flex-direction: column;
	}
	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 5px 0;
		border-bottom: 1px solid var(--hairline);
		font-size: 12px;
	}
	.nm.cur {
		color: var(--gold-ink);
		font-weight: 600;
	}
	.btn-d {
		border-color: var(--not-started);
		color: var(--not-started);
	}
	.self {
		align-self: flex-start;
	}
	.err {
		margin: 0;
		font-size: 12px;
		color: var(--not-started);
	}
	.sm0 {
		margin: 0;
		font-size: 13px;
	}
</style>
