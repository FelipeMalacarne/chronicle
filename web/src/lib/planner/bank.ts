import type { BankFreshness, BankStatus } from './types.ts';

/** Acima disto um banco "live" deixa de merecer confiança sem nova captura. */
export const LIVE_MAX_AGE_MS = 5 * 60_000;

export interface FreshnessView {
	freshness: BankFreshness;
	label: string;
	/** Texto de apoio; sempre explica o que a quantidade mostrada vale. */
	detail: string;
	/** Só `live` autoriza tratar as quantidades como atuais. */
	trustworthy: boolean;
	tone: 'success' | 'warning' | 'error';
}

function ago(at: number, now: number): string {
	const s = Math.max(0, Math.round((now - at) / 1000));
	if (s < 60) return `há ${Math.max(1, s)} s`;
	const m = Math.round(s / 60);
	if (m < 60) return `há ${m} min`;
	const h = Math.round(m / 60);
	return h < 48 ? `há ${h} h` : `há ${Math.round(h / 24)} d`;
}

export function describeBank(bank: BankStatus, now: number): FreshnessView {
	switch (bank.freshness) {
		case 'live':
			return {
				freshness: 'live',
				label: bank.capturedAt === null ? 'ao vivo' : `ao vivo · ${ago(bank.capturedAt, now)}`,
				detail: 'Lido pelo plugin; as quantidades valem para agora.',
				trustworthy: true,
				tone: 'success'
			};
		case 'restored':
			return {
				freshness: 'restored',
				label:
					bank.capturedAt === null ? 'restaurado' : `restaurado · ${ago(bank.capturedAt, now)}`,
				detail: `Banco restaurado do último estado salvo${
					bank.capturedAt === null ? '' : `, ${ago(bank.capturedAt, now)}`
				}: desatualizado. As quantidades podem ter mudado; confira no jogo antes de sair.`,
				trustworthy: false,
				tone: 'warning'
			};
		default:
			return {
				freshness: 'unavailable',
				label: 'banco indisponível',
				detail:
					'Sem leitura do banco. Abra o banco com o plugin ligado ou confira cada item à mão.',
				trustworthy: false,
				tone: 'error'
			};
	}
}
