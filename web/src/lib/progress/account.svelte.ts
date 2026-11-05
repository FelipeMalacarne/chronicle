import { page } from '$app/state';

const KEY = 'ironman.account';

/**
 * Conta das rotas de progresso: `?account=` (lembrada neste navegador) ou a última usada.
 * Em AUTH_MODE=dev o servidor aceita qualquer conta; em oidc, só as do usuário da sessão
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

/** Lembra a conta escolhida no setup neste navegador. */
export function rememberAccount(id: string) {
	try {
		localStorage.setItem(KEY, id);
	} catch {
		/* sem storage: a conta segue pela URL */
	}
}

/** Esquece a conta deste navegador (sair em AUTH_MODE=dev). */
export function forgetAccount() {
	try {
		localStorage.removeItem(KEY);
	} catch {
		/* sem storage: nada a esquecer */
	}
}
