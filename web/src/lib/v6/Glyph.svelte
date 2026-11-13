<script lang="ts">
	/** Glifos da v6 (traço 2px, como nos artboards). Decorativos: o texto ao lado diz o que é. */
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
		| 'sun'
		| 'sync'
		| 'book'
		| 'automacao'
		| 'x'
		| 'frota'
		| 'plus'
		| 'eye'
		| 'play'
		| 'atividade'
		| 'porta'
		| 'bank'
		| 'pause'
		| 'stop'
		| 'more';
	let { name, size = 16 }: { name: Name; size?: number } = $props();
</script>

<svg
	width={size}
	height={size}
	fill="none"
	stroke="currentColor"
	stroke-width={name === 'check' ? 3 : name === 'book' ? 2.4 : 2}
	viewBox={name === 'book' ? '0 0 40 40' : '0 0 24 24'}
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
	{:else if name === 'sun'}
		<circle cx="12" cy="12" r="4" /><path
			d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
		/>
	{:else if name === 'sync'}
		<path d="M20 12a8 8 0 1 1-2.3-5.7M20 4v5h-5" />
	{:else if name === 'book'}
		<path d="M11 7h16a2 2 0 0 1 2 2v24H13a2 2 0 0 1-2-2z" /><path
			d="M11 29a2 2 0 0 1 2-2h16"
		/><path d="M20 12l3 5-3 5-3-5z" />
	{:else if name === 'automacao'}
		<rect x="3" y="4" width="18" height="12" rx="1" /><path
			d="M8 20h8M12 16v4M9 10h.01M15 10h.01"
		/>
	{:else if name === 'x'}
		<path d="M6 6l12 12M18 6L6 18" />
	{:else if name === 'frota'}
		<rect x="3" y="4" width="18" height="5" rx="1" /><rect
			x="3"
			y="10"
			width="18"
			height="5"
			rx="1"
		/><rect x="3" y="16" width="18" height="5" rx="1" />
	{:else if name === 'plus'}
		<path d="M12 5v14M5 12h14" />
	{:else if name === 'eye'}
		<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" />
	{:else if name === 'play'}
		<path d="M8 5v14l11-7z" fill="currentColor" />
	{:else if name === 'atividade'}
		<path d="M3 12h4l3-7 4 14 3-7h4" />
	{:else if name === 'porta'}
		<path d="M14 4h5v16h-5M4 12h11M11 8l4 4-4 4" />
	{:else if name === 'bank'}
		<path d="M3 10l9-6 9 6M5 10v8M19 10v8M9 10v8M15 10v8M3 20h18" />
	{:else if name === 'pause'}
		<path d="M7 5h4v14H7zM13 5h4v14h-4z" fill="currentColor" stroke="none" />
	{:else if name === 'stop'}
		<rect x="6" y="6" width="12" height="12" rx="1" fill="currentColor" stroke="none" />
	{:else if name === 'more'}
		<circle cx="5" cy="12" r="1.8" fill="currentColor" stroke="none" /><circle
			cx="12"
			cy="12"
			r="1.8"
			fill="currentColor"
			stroke="none"
		/><circle cx="19" cy="12" r="1.8" fill="currentColor" stroke="none" />
	{/if}
</svg>
