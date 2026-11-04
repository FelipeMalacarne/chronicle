import type { GuideSummary } from './types';

/**
 * Seletor de guia exibido no cabeçalho global. A página guia-ciente (/plan) registra
 * aqui as opções e o callback; o TopBar só renderiza quando há algo registrado.
 */
export const guideSwitch = $state<{
	guides: GuideSummary[];
	value: string;
	onchange: ((id: string) => void) | null;
}>({ guides: [], value: '', onchange: null });

export function registerGuideSwitch(
	guides: GuideSummary[],
	value: string,
	onchange: (id: string) => void
) {
	guideSwitch.guides = guides;
	guideSwitch.value = value;
	guideSwitch.onchange = onchange;
}

export function clearGuideSwitch() {
	guideSwitch.guides = [];
	guideSwitch.onchange = null;
}
