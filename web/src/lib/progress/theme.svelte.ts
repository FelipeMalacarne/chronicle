export type Theme = 'pergaminho' | 'pedra';

const KEY = 'ironman.theme';

function initial(): Theme {
	try {
		const saved = localStorage.getItem(KEY);
		if (saved === 'pergaminho' || saved === 'pedra') return saved;
	} catch {
		/* sem storage: segue o sistema */
	}
	return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
		? 'pedra'
		: 'pergaminho';
}

/** Tema Pergaminho (dia) ou Pedra (noite); lembrado neste navegador. */
export const theme = $state<{ value: Theme }>({ value: initial() });

export function setTheme(value: Theme) {
	theme.value = value;
	try {
		localStorage.setItem(KEY, value);
	} catch {
		/* conveniência local apenas */
	}
}
