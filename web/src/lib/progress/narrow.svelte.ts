/**
 * Janela estreita ao lado do jogo (V6*640) ou olhar analítico (V6*1440): os mesmos componentes,
 * só a disposição muda (decisão 3 da v6). O corte é o mesmo do CSS da barra de topo.
 */
export const NARROW_QUERY = '(max-width: 900px)';

export const narrow = $state<{ value: boolean }>({
	value: typeof matchMedia === 'function' ? matchMedia(NARROW_QUERY).matches : false
});

if (typeof matchMedia === 'function') {
	matchMedia(NARROW_QUERY).addEventListener('change', (e) => (narrow.value = e.matches));
}
