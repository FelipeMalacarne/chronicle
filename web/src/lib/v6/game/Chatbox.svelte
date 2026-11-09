<script lang="ts">
	import type { GameView } from '#lib/api/microbot';
	import { chatLines, dialogueLine } from '#lib/progress/jogo';
	import { gameMenu, runAction } from './menu.svelte';

	/*
	 * A caixa de chat do cliente (doc 08): as mensagens do jogo (nunca as de jogadores) e, num
	 * diálogo, o texto com as opções clicáveis e o "Click here to continue", como no jogo.
	 */
	let { view }: { view: GameView } = $props();

	const chat = $derived(chatLines(view, 8));
	const dialogue = $derived(dialogueLine(view));
	let box = $state<HTMLDivElement>();
	$effect(() => {
		void chat.length;
		box?.scrollTo({ top: box.scrollHeight });
	});
</script>

<div class="chat" bind:this={box} aria-live="polite">
	{#if dialogue}
		<div class="cdlg">
			{#if dialogue.options.length}
				<p class="cq">{dialogue.text || 'Select an Option'}</p>
				{#each dialogue.options as o, n (n)}
					<button
						class="copt"
						type="button"
						disabled={gameMenu.busy}
						onclick={() =>
							runAction({
								label: o,
								target: '',
								tone: 'npc',
								command: 'dialogue_select',
								body: { index: n + 1 }
							})}>{o}</button
					>
				{/each}
			{:else}
				<p class="ctxt">{dialogue.text || '…'}</p>
			{/if}
			{#if dialogue.more}
				<button
					class="ccont"
					type="button"
					disabled={gameMenu.busy}
					onclick={() =>
						runAction({ label: 'Continue', target: '', tone: 'npc', command: 'dialogue_continue' })}
					>Click here to continue</button
				>
			{/if}
		</div>
	{:else}
		{#each chat as c (c.seq)}
			<div class="cln"><span class="ct">{c.at}</span>{c.text}</div>
		{:else}
			<div class="cln su">Sem mensagens do jogo.</div>
		{/each}
	{/if}
</div>

<style>
	.chat {
		height: 128px;
		overflow: auto;
		box-sizing: border-box;
		padding: 6px 10px;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 3px;
		font-size: 12.5px;
		color: var(--ink);
	}
	.cln {
		line-height: 1.45;
	}
	.cln .ct {
		font:
			400 10.5px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
		margin-right: 8px;
	}
	.cdlg {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		text-align: center;
	}
	.cq {
		margin: 0 0 4px;
		color: #800000;
		font-weight: 600;
	}
	.ctxt {
		margin: 0 0 4px;
	}
	.copt,
	.ccont {
		border: 0;
		background: none;
		font: inherit;
		cursor: pointer;
		padding: 1px 6px;
		border-radius: 2px;
	}
	.copt {
		color: var(--ink);
	}
	.copt:hover,
	.copt:focus-visible {
		color: #fff;
		background: var(--frame-2);
		outline: none;
	}
	.ccont {
		color: #0000ff;
		margin-top: 4px;
	}
	:global([data-theme='pedra']) .ccont {
		color: #7fa7ff;
	}
	:global([data-theme='pedra']) .cq {
		color: #e86c5c;
	}
	.ccont:hover {
		text-decoration: underline;
	}
</style>
