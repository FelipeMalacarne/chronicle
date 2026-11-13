<script lang="ts">
	import { microbotApi, type AuditEntry } from '#lib/api/microbot';
	import { auditLine, auditWho } from '#lib/progress/automacao';

	/*
	 * O que se fez no cliente desta conta (o antigo painel Auditoria do cockpit): quem, quando e o
	 * quê, a mais nova primeiro. Provisório na Visão geral até o Histórico (fatia V7), que junta
	 * execuções e ações do dono numa linha do tempo.
	 */
	let { version = 0 }: { version?: number } = $props();

	const FIRST = 8;
	let entries = $state<AuditEntry[] | null>(null);
	let all = $state(false);

	async function load() {
		try {
			entries = (await microbotApi.audit(30)).entries;
		} catch {
			entries ??= [];
		}
	}
	$effect(() => {
		void version;
		void load();
		const t = setInterval(() => {
			if (!document.hidden) void load();
		}, 30_000);
		return () => clearInterval(t);
	});
	const shown = $derived(entries ? (all ? entries : entries.slice(0, FIRST)) : []);
	const hhmm = (iso: string) =>
		new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
	const day = (iso: string) =>
		new Date(iso).toLocaleString('pt-BR', {
			day: '2-digit',
			month: '2-digit',
			hour: '2-digit',
			minute: '2-digit'
		});
</script>

<section class="card">
	<div class="h">
		<h2>Ações no cliente</h2>
		<span>quem · quando · o quê</span>
	</div>
	<div class="b">
		{#if !entries}
			<p class="mu" aria-busy="true">Lendo…</p>
		{:else}
			<div>
				{#each shown as e (e.id)}
					<div class="log" class:fail={!e.ok}>
						<span class="t" title={day(e.at)}>{hhmm(e.at)}</span><span class="who"
							>{auditWho(e)}</span
						><span>{auditLine(e)}</span>
					</div>
				{:else}
					<p class="mu">Nenhuma ação ainda.</p>
				{/each}
			</div>
			{#if entries.length > FIRST}
				<button class="more" type="button" onclick={() => (all = !all)}
					>{all ? 'ver menos' : `ver as ${entries.length}`}</button
				>
			{/if}
		{/if}
	</div>
</section>

<style>
	.log {
		display: flex;
		gap: 10px;
		align-items: flex-start;
		padding: 6px 0;
		border-bottom: 1px solid var(--hairline);
		font-size: 12px;
	}
	.log:last-child {
		border-bottom: 0;
	}
	.log .t {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		width: 40px;
		flex: 0 0 auto;
		padding-top: 1px;
	}
	.log .who {
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--ink-subtle);
		width: 54px;
		flex: 0 0 auto;
		padding-top: 2px;
	}
	.log.fail span:last-child {
		color: var(--not-started);
	}
	.more {
		align-self: flex-start;
		border: 0;
		background: none;
		padding: 0;
		font-size: 12px;
		color: var(--gold-ink);
		text-decoration: underline;
		cursor: pointer;
	}
</style>
