<script lang="ts" module>
	export interface MenuItem {
		label: string;
		/** Small right-aligned text: a key, a state ("pedra"). */
		hint?: string;
		href?: string;
		disabled?: boolean;
		onselect?: () => void;
	}
</script>

<script lang="ts">
	import Glyph from './Glyph.svelte';

	/**
	 * Overflow menu (v5 limpeza): a "⋯" button that opens a list of actions. Escape and a click
	 * outside close it and focus returns to the button; arrows move between the items.
	 */
	let {
		label,
		items,
		align = 'end',
		cls = 'btn btn-g'
	}: { label: string; items: MenuItem[]; align?: 'start' | 'end'; cls?: string } = $props();

	let open = $state(false);
	let root: HTMLDivElement | undefined = $state();
	let button: HTMLButtonElement | undefined = $state();
	const id = `menu-${Math.random().toString(36).slice(2, 8)}`;

	function entries(): HTMLElement[] {
		return [...(root?.querySelectorAll<HTMLElement>('.menu-it:not(:disabled)') ?? [])];
	}
	async function toggle() {
		open = !open;
		if (open) {
			await Promise.resolve();
			entries()[0]?.focus();
		}
	}
	function close(focus = true) {
		open = false;
		if (focus) button?.focus();
	}
	function keydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			close();
			return;
		}
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			const list = entries();
			const i = list.indexOf(document.activeElement as HTMLElement);
			const n = e.key === 'ArrowDown' ? i + 1 : i - 1;
			list[(n + list.length) % list.length]?.focus();
		}
	}
	function select(it: MenuItem) {
		close(!it.href);
		it.onselect?.();
	}
</script>

<svelte:window
	onkeydown={keydown}
	onclick={(e) => {
		if (open && root && !root.contains(e.target as Node)) close(false);
	}}
/>

<div class="menu" bind:this={root}>
	<button
		bind:this={button}
		class={cls}
		type="button"
		aria-label={label}
		title={label}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls={id}
		onclick={toggle}><Glyph name="more" /></button
	>
	{#if open}
		<ul class="menu-pop" class:start={align === 'start'} role="menu" {id} aria-label={label}>
			{#each items as it (it.label)}
				<li role="none">
					{#if it.href}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass resolve() hrefs -->
						<a class="menu-it" role="menuitem" href={it.href} onclick={() => select(it)}
							>{it.label}{#if it.hint}<span class="hint">{it.hint}</span>{/if}</a
						>
					{:else}
						<button
							class="menu-it"
							role="menuitem"
							type="button"
							disabled={it.disabled}
							onclick={() => select(it)}
							>{it.label}{#if it.hint}<span class="hint">{it.hint}</span>{/if}</button
						>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
