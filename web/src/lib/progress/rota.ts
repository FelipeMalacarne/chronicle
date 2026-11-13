import type { ContentStep } from './types.ts';
import { loadoutItems, metaOf, type SegmentView } from './view.ts';

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

/** A busca da Rota: o termo no ref, no texto, no local ou num nome resolvido do passo. */
export function stepMatches(
	s: Pick<ContentStep, 'ref' | 'text' | 'location' | 'names'>,
	term: string
): boolean {
	const q = term.trim().toLowerCase();
	if (!q) return true;
	return [s.ref, s.text, s.location ?? '', ...Object.values(s.names ?? {})]
		.join(' ')
		.toLowerCase()
		.includes(q);
}

/**
 * Recorte de `text` em volta da primeira ocorrência de `term`, com reticências nas pontas
 * cortadas; sem ocorrência no texto (o termo bateu num nome ou no local), o começo.
 */
export function snippet(text: string, term: string, width = 140): string {
	const flat = text.replace(/\s+/g, ' ').trim();
	if (flat.length <= width) return flat;
	const q = term.trim().toLowerCase();
	const at = q ? flat.toLowerCase().indexOf(q) : -1;
	const from = at < 0 ? 0 : Math.max(0, Math.min(at - Math.floor(width / 3), flat.length - width));
	const to = Math.min(flat.length, from + width);
	return `${from > 0 ? '…' : ''}${flat.slice(from, to).trim()}${to < flat.length ? '…' : ''}`;
}

/* ------------------------------------------------------------------ v7: marca de banco e fatos da linha */

export interface BankMark {
	/** "banco · 10 itens", ou só "banco" quando o guia não lista o que levar. */
	label: string;
	title: string;
}

/**
 * A marca de banco na linha do passo (V7Rota1440, V7RotaLista1440; decisoes-canvas.md, 7.1.10): o
 * primeiro passo de cada trecho de banco depois do primeiro da seção, no lugar do separador
 * "banco · entre X e Y" da v6. Banco é instrução do guia e nunca vira passo feito (regra 8).
 */
export function bankMarks(
	segments: Pick<SegmentView, 'kind' | 'steps' | 'loadout' | 'bankActions'>[]
): Map<string, BankMark> {
	const out = new Map<string, BankMark>();
	segments.forEach((seg, i) => {
		const first = seg.steps[0];
		if (i === 0 || seg.kind !== 'bank_leg' || !first) return;
		const n = loadoutItems(seg.loadout).length;
		const said = seg.bankActions.find((a) => a.position === 'start') ?? seg.bankActions[0];
		out.set(first.id, {
			label: n ? `banco · ${n} ${n === 1 ? 'item' : 'itens'}` : 'banco',
			title: [
				'O guia manda passar no banco antes deste passo',
				said ? `“${said.text}”` : '',
				seg.loadout?.status === 'partial' ? 'lista parcial do que levar' : ''
			]
				.filter(Boolean)
				.join(' · ')
		});
	});
	return out;
}

/** O GP depois do passo e o tempo do guia, do BRUHsailer, cada um ausente sem a nota dele. */
export function stepFactParts(step: Pick<ContentStep, 'notes'>): { gp?: string; time?: string } {
	const meta = metaOf(step);
	const gp = meta.find((m) => m.label === 'GP após')?.value;
	const time = meta.find((m) => m.label === 'tempo acumulado')?.value;
	return { gp: gp ? gpLabel(gp) : undefined, time: time ? timeLabel(time) : undefined };
}

/** "777.349 gp · 5 d 0 h": o GP depois do passo e o tempo do guia, do BRUHsailer; "" sem os dois. */
export function stepFacts(step: Pick<ContentStep, 'notes'>): string {
	const { gp, time } = stepFactParts(step);
	return [gp, time].filter(Boolean).join(' · ');
}

/**
 * O cabeçalho do passo atual (V7Rota1440): "GP depois 777.349 · tempo do guia 5 d 0 h"; a 640
 * (V7Rota640), só o GP. "" sem as notas do BRUHsailer.
 */
export function headerFacts(step: Pick<ContentStep, 'notes'>, narrow = false): string {
	const { gp, time } = stepFactParts(step);
	return [
		gp ? `GP depois ${gp.replace(/\s*gp$/i, '')}` : '',
		time && !narrow ? `tempo do guia ${time}` : ''
	]
		.filter(Boolean)
		.join(' · ');
}

/** "777349 gp (after buying…)" → "777.349 gp". */
export function gpLabel(raw: string): string {
	return raw
		.replace(/\s*\(.*$/, '')
		.replace(/\d{4,}/g, (n) => Number(n).toLocaleString('pt-BR'))
		.trim();
}

/** "5d0h" → "5 d 0 h"; outro formato passa como veio. */
export function timeLabel(raw: string): string {
	return raw
		.replace(/(\d+)\s*([dhm])(?![a-z])/gi, '$1 $2 ')
		.replace(/\s+/g, ' ')
		.trim();
}
