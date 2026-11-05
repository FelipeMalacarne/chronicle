<script lang="ts">
	/** Glifos da v4 (traço 2px, como nos artboards). Decorativos: o texto ao lado diz o que é. */
	type Name =
		| 'agora'
		| 'rotas'
		| 'mundo'
		| 'conta'
		| 'search'
		| 'moon'
		| 'check'
		| 'chevron'
		| 'alert'
		| 'clock'
		| 'more';
	let { name, size = 16 }: { name: Name; size?: number } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width={name === 'check' ? 3 : 2}
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	{#if name === 'agora'}
		<circle cx="12" cy="12" r="8" /><circle cx="12" cy="12" r="2.5" fill="currentColor" />
	{:else if name === 'rotas'}
		<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01" />
	{:else if name === 'mundo'}
		<circle cx="12" cy="12" r="9" /><path
			d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"
		/>
	{:else if name === 'conta'}
		<circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" />
	{:else if name === 'search'}
		<circle cx="10.5" cy="10.5" r="6.5" /><path d="M20 20l-4.6-4.6" />
	{:else if name === 'moon'}
		<path d="M12 3a9 9 0 1 0 9 9 7 7 0 0 1-9-9z" />
	{:else if name === 'check'}
		<path d="M5 12.5l4.5 4.5L19 7.5" />
	{:else if name === 'chevron'}
		<path d="M9 6l6 6-6 6" />
	{:else if name === 'alert'}
		<path d="M12 3l10 18H2L12 3z M12 10v5 M12 18h.01" />
	{:else if name === 'clock'}
		<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
	{:else if name === 'more'}
		<circle cx="5" cy="12" r="1.2" fill="currentColor" /><circle
			cx="12"
			cy="12"
			r="1.2"
			fill="currentColor"
		/><circle cx="19" cy="12" r="1.2" fill="currentColor" />
	{/if}
</svg>
