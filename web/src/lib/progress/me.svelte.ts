import { progressApi } from '#lib/api/client';
import type { AccountState } from './types';

/**
 * Estado da conta compartilhado pelo shell (cartão da conta na sidebar) e pela Conta:
 * relido quando a conta muda e a cada evento de progresso ou sync.
 */
export const me = $state<{ state: AccountState | null }>({ state: null });

let seq = 0;
export async function loadMe(account: string | undefined): Promise<void> {
	const my = ++seq;
	if (!account) {
		me.state = null;
		return;
	}
	try {
		const s = await progressApi.accountState(account);
		if (my === seq) me.state = s;
	} catch {
		/* o indicador global de sync já registrou a falha; o cartão mostra o id */
	}
}

const MODE: Record<string, string> = {
	ironman: 'Ironman',
	hcim: 'Hardcore',
	uim: 'Ultimate',
	gim: 'Group'
};

/** "Ironman · 701 total" no cartão da sidebar. */
export function accountSub(s: AccountState | null): string {
	if (!s) return '';
	const mode = MODE[s.account.mode] ?? s.account.mode;
	return s.total_level ? `${mode} · ${s.total_level} total` : mode;
}
