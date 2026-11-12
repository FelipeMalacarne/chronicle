import { microbotApi, setMicrobotAccount } from '../api/microbot.ts';

/**
 * Se o servidor tem o módulo do Microbot ligado (MICROBOT_CONTROL=on, doc 08): null até a primeira
 * consulta. A navegação só mostra "Automação" quando ele existe.
 */
export const microbotModule = $state<{ enabled: boolean | null }>({ enabled: null });

let probing = false;

export async function probeMicrobot(): Promise<void> {
	if (probing || microbotModule.enabled !== null) return;
	probing = true;
	try {
		microbotModule.enabled = (await microbotApi.status()) !== null;
	} catch {
		microbotModule.enabled = false;
	} finally {
		probing = false;
	}
}

const ACCOUNT_KEY = 'chronicle.cockpit.account';

function storedAccount(): string {
	try {
		return localStorage.getItem(ACCOUNT_KEY) ?? '';
	} catch {
		return '';
	}
}

/**
 * A conta que o Cockpit e Plugins controlam (docs/11): vazia é o único agent conectado. Lembrada
 * neste navegador; ler `cockpit.account` num template o torna reativo à troca.
 */
export const cockpit = $state<{ account: string }>({ account: '' });

let accountLoaded = false;

/** Lê a conta lembrada (uma vez) e a aplica às rotas do Microbot. */
export function loadCockpitAccount(): void {
	if (accountLoaded) return;
	accountLoaded = true;
	chooseCockpitAccount(storedAccount());
}

export function chooseCockpitAccount(account: string): void {
	cockpit.account = account;
	setMicrobotAccount(account);
	try {
		if (account) localStorage.setItem(ACCOUNT_KEY, account);
		else localStorage.removeItem(ACCOUNT_KEY);
	} catch {
		/* sem armazenamento: vale só nesta aba */
	}
}
