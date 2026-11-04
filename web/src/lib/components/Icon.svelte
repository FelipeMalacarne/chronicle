<script lang="ts">
	/** Ícones inline em traço (stroke), 24x24, herdam currentColor. */
	export type IconName =
		| 'pin'
		| 'check'
		| 'skip'
		| 'note'
		| 'search'
		| 'home'
		| 'book'
		| 'user'
		| 'target'
		| 'graph'
		| 'chevron'
		| 'external'
		| 'alert'
		| 'bag';

	let { name, size = 16, label }: { name: IconName; size?: number; label?: string } = $props();

	const paths: Record<IconName, string> = {
		pin: 'M12 21s-6-5.3-6-11a6 6 0 1 1 12 0c0 5.7-6 11-6 11z M12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
		check: 'M5 12.5l4.5 4.5L19 7.5',
		skip: 'M5 5l9 7-9 7V5z M18 5v14',
		note: 'M4 5.5h12M4 10h16M4 14.5h16M4 19h9',
		search: 'M10.5 17a6.5 6.5 0 1 0 0-13 6.5 6.5 0 0 0 0 13z M20 20l-4.6-4.6',
		home: 'M4 11l8-7 8 7v9a1 1 0 0 1-1 1h-5v-6h-4v6H5a1 1 0 0 1-1-1v-9z',
		book: 'M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2V5z M4 19a2 2 0 0 1 2-2h13',
		user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8z M4 21a8 8 0 0 1 16 0',
		target: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z M12 12h.01',
		graph:
			'M6 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4z M18 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4z M12 21a2 2 0 1 0 0-4 2 2 0 0 0 0 4z M7 7l4 10 M17 7l-4 10',
		chevron: 'M9 6l6 6-6 6',
		external: 'M14 4h6v6 M20 4l-9 9 M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5',
		alert: 'M12 3l10 18H2L12 3z M12 10v5 M12 18h.01',
		bag: 'M5 8h14l-1 12H6L5 8z M9 8V6a3 3 0 0 1 6 0v2'
	};
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.8"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden={label ? undefined : 'true'}
	role={label ? 'img' : undefined}
	aria-label={label}
	focusable="false"
>
	<path d={paths[name]} />
</svg>
