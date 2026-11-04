<script lang="ts">
	import type { Step, StepStatus } from '#lib/api/types';
	import { ruleBadges } from '#lib/api/rules';
	import StatusCircle from './StatusCircle.svelte';
	import Icon from './Icon.svelte';

	let {
		step,
		status = 'pending',
		current = false
	}: { step: Step; status?: StepStatus; current?: boolean } = $props();

	let open = $state(false);
	const badges = $derived(ruleBadges(step));
	const hasNote = $derived(Boolean(step.note || step.update_note || step.links?.length));
	const circle = $derived(
		current
			? 'current'
			: status === 'pending'
				? 'pending'
				: status === 'skipped'
					? 'skipped'
					: 'done'
	);
</script>

<li class="row" class:current id="step-{step.id}">
	<div class="main">
		<StatusCircle state={circle} />
		<span class="data id">{step.id}</span>
		<div class="body">
			<p class="text">
				{step.text}
				{#if step.layer === 'enhanced'}
					<span class="badge badge-accent">enhanced</span>
				{/if}
				{#each badges as b (b.label)}
					<span class="badge" class:badge-accent={b.tone === 'accent'}>{b.label}</span>
				{/each}
			</p>
			{#if step.items?.length}
				<ul class="items" aria-label="Levar">
					{#each step.items as it (it)}
						<li class="chip chip-mono item">{it}</li>
					{/each}
				</ul>
			{/if}
			{#if open}
				<div class="note">
					{#if step.note}<p>{step.note}</p>{/if}
					{#if step.update_note}
						<p class="update">
							<span class="badge badge-warning">atualização</span>
							{step.update_note}
						</p>
					{/if}
					{#if step.links?.length}
						<p class="links">
							{#each step.links as l (l.url)}
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- link externo (wiki/imgur) -->
								<a href={l.url} target="_blank" rel="noopener noreferrer"
									>{l.text ?? l.type} <Icon name="external" size={12} /></a
								>
							{/each}
						</p>
					{/if}
				</div>
			{/if}
		</div>
		{#if hasNote}
			<button
				class="toggle"
				type="button"
				aria-expanded={open}
				aria-controls="note-{step.id}"
				onclick={() => (open = !open)}
				title={open ? 'Ocultar nota' : 'Ver nota'}
			>
				<span class="visually-hidden">{open ? 'Ocultar nota' : 'Ver nota'}</span>
				<span class="chev" class:open><Icon name="chevron" size={14} /></span>
			</button>
		{/if}
	</div>
</li>

<style>
	.row {
		list-style: none;
		border-radius: var(--r-row);
		border: 1px solid transparent;
	}
	.row.current {
		box-shadow: 0 0 0 2px var(--accent);
		background: var(--accent-tint);
	}
	.main {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		min-height: var(--h-row);
		padding: var(--sp-3) var(--sp-4);
	}
	.main > :global(.circle) {
		margin-top: 2px;
	}
	.id {
		color: var(--subtle);
		flex: 0 0 auto;
		min-width: 46px;
		padding-top: 3px;
	}
	.body {
		flex: 1 1 auto;
		min-width: 0;
	}
	.text {
		line-height: 1.45;
	}
	.text .badge {
		vertical-align: 2px;
		margin-left: 6px;
	}
	.items {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin: 8px 0 0;
		padding: 0;
	}
	.item {
		height: 24px;
		list-style: none;
	}
	.note {
		margin-top: 8px;
		padding: 10px 12px;
		border-radius: var(--r-chip);
		background: var(--bg);
		color: var(--muted);
		font-size: var(--fs-label);
		display: grid;
		gap: 6px;
	}
	.update .badge {
		vertical-align: 1px;
		margin-right: 4px;
	}
	.links {
		display: flex;
		gap: var(--sp-3);
		flex-wrap: wrap;
	}
	.links a {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.toggle {
		flex: 0 0 auto;
		width: 28px;
		height: 28px;
		border-radius: var(--r-chip);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--muted);
	}
	.toggle:hover {
		background: var(--elevated);
		color: var(--text);
	}
	.chev {
		display: inline-flex;
		transition: transform 120ms ease;
	}
	.chev.open {
		transform: rotate(90deg);
	}
</style>
