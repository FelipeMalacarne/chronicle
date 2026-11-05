import type { ContentStep } from './types.ts';

/**
 * Pedaços puros da Rota da v6 (V6Rota1440/640, nota "Rota · decisões" do canvas): o cabeçalho do
 * trecho resume as quests concluídas e os locais principais, com a lista completa no título.
 */

export interface LegSummary {
	/** "Lumbridge → Al Kharid, 24 quests" ou "Varrock, Shield of Arrav". */
	title: string;
	/** "Grand Tree, Biohazard, Murder Mystery, Merlin’s Crystal, Lost Tribe e mais 19". */
	quests: string;
	/** "por Varrock, Gnome Stronghold, Ardougne, Camelot, Falador". */
	places: string;
	/** A lista completa para o atributo title. */
	full: string;
}

const SHOWN = 5;

function uniq(xs: string[]): string[] {
	return xs.filter((x, i) => x && xs.indexOf(x) === i);
}

/** Resumo de um trecho a partir dos passos: locais na ordem da rota e quests que ele conclui. */
export function legSummary(
	steps: Pick<ContentStep, 'location' | 'objectives' | 'names'>[],
	range: string
): LegSummary {
	const places = uniq(steps.map((s) => s.location ?? ''));
	const quests = uniq(
		steps.flatMap((s) =>
			(s.objectives ?? [])
				.filter((o) => o.role === 'completes' && o.key.startsWith('quest:'))
				.map((o) => s.names[o.key.replace(/@.*$/, '')] ?? '')
		)
	);
	const where =
		places.length === 0
			? `Trecho ${range}`
			: places.length === 1
				? places[0]
				: `${places[0]} → ${places[places.length - 1]}`;
	const title =
		quests.length === 0
			? where
			: quests.length === 1 && places.length <= 1
				? `${where}, ${quests[0]}`
				: `${where}, ${quests.length} ${quests.length === 1 ? 'quest' : 'quests'}`;
	const more = (xs: string[]) =>
		xs.length > SHOWN
			? `${xs.slice(0, SHOWN).join(', ')} e mais ${xs.length - SHOWN}`
			: xs.join(', ');
	const full = [
		quests.length
			? `${quests.length} ${quests.length === 1 ? 'quest' : 'quests'}: ${quests.join(', ')}`
			: '',
		places.length > 1
			? `${places.length} locais, de ${places[0]} a ${places[places.length - 1]}`
			: (places[0] ?? '')
	]
		.filter(Boolean)
		.join(' · ');
	return {
		title,
		quests: more(quests),
		places: places.length > 1 ? `por ${places.slice(0, SHOWN).join(', ')}` : '',
		full
	};
}

/** Realce do termo buscado: partes do texto com o trecho que bateu marcado. */
export function highlight(text: string, term: string): { t: string; m: boolean }[] {
	const q = term.trim().toLowerCase();
	if (!q) return [{ t: text, m: false }];
	const out: { t: string; m: boolean }[] = [];
	let from = 0;
	const lower = text.toLowerCase();
	for (let i = lower.indexOf(q); i >= 0; i = lower.indexOf(q, i + q.length)) {
		if (i > from) out.push({ t: text.slice(from, i), m: false });
		out.push({ t: text.slice(i, i + q.length), m: true });
		from = i + q.length;
	}
	if (from < text.length) out.push({ t: text.slice(from), m: false });
	return out;
}
