import { progressApi } from '#lib/api/client';
import type { LaneTone } from './frota';
import type { AccountView } from './types';

/**
 * As contas do usuário, compartilhadas pela barra lateral e pela Frota (página 8 do canvas). A
 * Frota grava o estado de cada uma em `tones`; fora dela a lâmpada fica como estava.
 */
export const fleet = $state<{ accounts: AccountView[] | null; tones: Record<string, LaneTone> }>({
	accounts: null,
	tones: {}
});

let loading: Promise<void> | null = null;

export function loadFleet(force = false): Promise<void> {
	if (loading && !force) return loading;
	loading = progressApi
		.me()
		.then((me) => {
			fleet.accounts = me.accounts ?? [];
		})
		.catch(() => {
			/* o indicador global de sync já registrou a falha; a lista fica como estava */
		});
	return loading;
}
