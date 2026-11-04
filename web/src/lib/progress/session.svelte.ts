import { progressApi } from '#lib/api/client';
import type { ProgressSummary } from './types';

/**
 * Estado compartilhado pelo shell e pelas telas: a conta e o resumo das rotas (rota ativa,
 * cursores, cobertura, fontes). Recarregado quando a conta muda e depois de cada escrita.
 */
export const session = $state<{
	account?: string;
	summary: ProgressSummary | null;
	error: Error | null;
	loading: boolean;
}>({ summary: null, error: null, loading: false });

let seq = 0;

export async function loadSummary(account: string | undefined): Promise<void> {
	const my = ++seq;
	session.account = account;
	if (!account) {
		session.summary = null;
		session.error = null;
		session.loading = false;
		return;
	}
	session.loading = true;
	try {
		const summary = await progressApi.summary(account);
		if (my !== seq) return;
		session.summary = summary;
		session.error = null;
	} catch (e) {
		if (my !== seq) return;
		session.error = e instanceof Error ? e : new Error(String(e));
	} finally {
		if (my === seq) session.loading = false;
	}
}

export function setSummary(summary: ProgressSummary) {
	seq++;
	session.summary = summary;
	session.error = null;
	session.loading = false;
}
