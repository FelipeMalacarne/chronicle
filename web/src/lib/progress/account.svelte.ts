import { page } from '$app/state';

const KEY = 'ironman.account';

/**
 * Conta das rotas de progresso: `?account=` (lembrada neste navegador) ou a última usada.
 * Conveniência local até existir sessão; o servidor só aceita isso em AUTH_MODE=dev
 * (CLAUDE.md, regra 10). Contas de teste: `just account-create <rsn>`.
 */
export function currentAccount(): string | undefined {
	const raw = page.url.searchParams.get('account')?.trim();
	try {
		if (raw) localStorage.setItem(KEY, raw);
		return raw || localStorage.getItem(KEY) || undefined;
	} catch {
		return raw || undefined;
	}
}
