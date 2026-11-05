/**
 * Painel de entidade (V6Entidade): qualquer chip, slot ou linha de entidade abre o mesmo painel
 * por cima da tela, sem sair da página (decisão 7 da v6). O layout o desenha uma vez.
 */
export const entityPanel = $state<{ ref: string | null }>({ ref: null });

export function openEntity(ref: string) {
	if (ref) entityPanel.ref = ref;
}

export function closeEntity() {
	entityPanel.ref = null;
}
