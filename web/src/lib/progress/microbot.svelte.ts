import { microbotApi } from '../api/microbot.ts';

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
