<script lang="ts">
	import { resolve } from '$app/paths';
	import type { AgentInfo } from '#lib/api/microbot';
	import { chooseCockpitAccount, cockpit, loadCockpitAccount } from '#lib/progress/microbot.svelte';

	/*
	 * Abas da Automação (página 7 do canvas: Workers é a primeira) e, no Cockpit e em Plugins, o
	 * seletor da conta controlada (docs/11: tudo é endereçado pela conta). Funcional primeiro, por
	 * decisão do dono (2026-11-12); a aparência fica para a revisão das telas.
	 */
	let {
		current,
		agents = [],
		onaccount
	}: {
		current: 'workers' | 'cockpit' | 'plugins';
		agents?: AgentInfo[];
		onaccount?: () => void;
	} = $props();

	loadCockpitAccount();

	const options = $derived(
		agents
			.filter((a) => a.account)
			.map((a) => ({
				account: a.account as string,
				label: `${a.account} · ${a.kind === 'managed' ? 'pod ' + (a.worker ?? '') : (a.host ?? 'anexado')}`
			}))
	);

	function choose(e: Event) {
		chooseCockpitAccount((e.currentTarget as HTMLSelectElement).value);
		onaccount?.();
	}
</script>

<div class="tabs">
	<div class="seg" role="tablist" aria-label="Automação">
		<a class:on={current === 'workers'} href={resolve('/automacao/workers')}>Workers</a>
		<a class:on={current === 'cockpit'} href={resolve('/automacao')}>Cockpit</a>
		<a class:on={current === 'plugins'} href={resolve('/automacao/plugins')}>Plugins</a>
	</div>
	{#if current !== 'workers'}
		<label class="pick" title="A conta cujo cliente esta tela controla">
			<span>Conta</span>
			<select value={cockpit.account} onchange={choose}>
				<option value="">{agents.length <= 1 ? 'o agent conectado' : 'escolha uma conta'}</option>
				{#each options as o (o.account)}
					<option value={o.account}>{o.label}</option>
				{/each}
				{#if cockpit.account && !options.some((o) => o.account === cockpit.account)}
					<option value={cockpit.account}>{cockpit.account} · sem agent</option>
				{/if}
			</select>
		</label>
	{/if}
</div>

<style>
	.tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		align-items: center;
	}
	.seg a {
		display: inline-flex;
		align-items: center;
		height: 32px;
		padding: 0 10px;
		font:
			500 12px Inter,
			system-ui,
			sans-serif;
		color: var(--ink-muted);
		text-decoration: none;
		background: var(--elevated);
	}
	.seg a + a {
		border-left: 1px solid var(--border);
	}
	.seg a.on {
		background: var(--frame);
		color: var(--gold);
		box-shadow: var(--bevel-frame);
	}
	.pick {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.pick select {
		height: 32px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--ground);
		color: var(--ink);
		font: inherit;
		padding: 0 6px;
	}
</style>
