import { page } from '$app/state';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { createFixtureAdapter, type PlannerAdapter } from './adapter';
import { createApiAdapter } from './apiAdapter';
import { planQuery } from './navigation';
import type { MockScenario } from './types';

const SCENARIOS: MockScenario[] = ['ready', 'loading', 'error', 'empty'];

/** Cenário do mock vindo de `?mock=`; qualquer outro valor cai em `ready`. */
export function scenarioFromUrl(): MockScenario {
	const raw = page.url.searchParams.get('mock');
	return SCENARIOS.find((s) => s === raw) ?? 'ready';
}

export function bankFromUrl() {
	const raw = page.url.searchParams.get('bank');
	return raw === 'unavailable' || raw === 'restored' || raw === 'live' ? raw : undefined;
}

const ACCOUNT_KEY = 'ironman.account';

/**
 * Conta da readiness: `?account=` (lembrada neste navegador) ou a última usada. Só
 * conveniência local até existir sessão; o servidor só aceita isso em AUTH_MODE=dev.
 */
export function accountFromUrl(): string | undefined {
	const raw = page.url.searchParams.get('account')?.trim();
	try {
		if (raw) localStorage.setItem(ACCOUNT_KEY, raw);
		return raw || localStorage.getItem(ACCOUNT_KEY) || undefined;
	} catch {
		return raw || undefined;
	}
}

/** API real por padrão; `?mock=` ou `?bank=` trocam para as fixtures de revisão visual. */
export function currentAdapter(): PlannerAdapter {
	const params = page.url.searchParams;
	if (!params.has('mock') && !params.has('bank')) {
		return createApiAdapter({ accountId: accountFromUrl() });
	}
	const bank = bankFromUrl();
	return createFixtureAdapter({
		scenario: scenarioFromUrl(),
		latencyMs: 200,
		bank: bank ? { 'oziris-main': bank, 'bruhsailer-main': bank } : undefined,
		microbotControl: page.url.searchParams.get('control') !== 'off'
	});
}

/** Atualiza a query preservando os demais parâmetros (guia, mock, bank). */
export function setParams(path: '/plan' | '/compare', patch: Record<string, string | null>) {
	const merged: Record<string, string | null> = {};
	page.url.searchParams.forEach((v, k) => (merged[k] = v));
	Object.assign(merged, patch);
	// eslint-disable-next-line svelte/no-navigation-without-resolve -- query montada sobre resolve()
	return goto(resolve(path) + planQuery(merged), { replace: true, reset: false });
}
