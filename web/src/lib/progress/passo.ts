/**
 * Passo-parágrafo do BRUHsailer por dentro (Agora): o texto do autor cortado em ações (as frases),
 * cada objetivo na ação onde está a citação dele, e o que ainda falta no passo. Puro, testado em
 * node. Não muda o passo nem as marcas: o passo continua sendo a unidade da rota.
 */
import type { Verdict } from './types.ts';

/** Um objetivo do passo com o veredito da conta e onde a anotação o cita no texto. */
export interface PassoObjective {
	key: string;
	role: string;
	label: string;
	verdict: Verdict;
	/** [início, fim) em bytes UTF-8 do texto do passo (ObjectiveEvidence.span do servidor). */
	span?: [number, number];
}

/** Um trecho de uma ação: texto puro ou a citação de um objetivo. */
export interface Piece {
	text: string;
	objective?: PassoObjective;
}

export type ActionState = 'done' | 'open' | 'unknown' | 'plain';

export interface Action {
	text: string;
	/** [início, fim) em posições do texto (UTF-16, como o JavaScript conta). */
	start: number;
	end: number;
	pieces: Piece[];
	objectives: PassoObjective[];
	state: ActionState;
}

/** Converte um deslocamento em bytes UTF-8 na posição do texto em UTF-16. */
export function byteToIndex(text: string, byte: number): number {
	let bytes = 0;
	let i = 0;
	while (i < text.length && bytes < byte) {
		const cp = text.codePointAt(i) ?? 0;
		bytes += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
		i += cp > 0xffff ? 2 : 1;
	}
	return i;
}

/* Abreviações que terminam em ponto sem fechar a frase. */
const ABBREV = /(?:\b(?:e\.g|i\.e|approx|vs|etc|st|mr|mrs|dr|lvl|no|ca)\.)$/i;

/**
 * As frases do texto com a posição de cada uma. Uma linha ("- …") nunca junta com a outra; dentro
 * dela, corta depois de . ! ? seguido de espaço e de maiúscula, número ou parêntese.
 */
export function sentences(text: string): { text: string; start: number; end: number }[] {
	const out: { text: string; start: number; end: number }[] = [];
	const push = (from: number, to: number) => {
		let s = from;
		let e = to;
		while (s < e && /[\s\-•]/.test(text[s])) s++;
		while (e > s && /\s/.test(text[e - 1])) e--;
		if (e > s) out.push({ text: text.slice(s, e), start: s, end: e });
	};
	const lines = /[^\n]+/g;
	for (let m = lines.exec(text); m; m = lines.exec(text)) {
		const lineEnd = m.index + m[0].length;
		let from = m.index;
		const cut = /[.!?]\s+(?=[A-Z0-9(“"'])/g;
		cut.lastIndex = m.index;
		for (let c = cut.exec(text); c && c.index < lineEnd; c = cut.exec(text)) {
			const before = text.slice(from, c.index + 1);
			if (ABBREV.test(before)) continue;
			push(from, c.index + 1);
			from = c.index + c[0].length;
		}
		push(from, lineEnd);
	}
	return out;
}

/** Frase longa a partir da qual as vírgulas também separam ações. */
export const LONG_SENTENCE = 160;
/** Pedaço curto demais para ser uma ação sozinho: junta com o anterior. */
const MIN_CLAUSE = 20;

/**
 * As ações do texto: as frases e, numa frase longa, os trechos entre vírgulas fora de parênteses
 * (o BRUHsailer encadeia ações com vírgulas: "Talk to Aris to start Demon Slayer, Romeo to start
 * Romeo and Juliet, …"). Pedaços com menos de MIN_CLAUSE caracteres juntam com o anterior.
 */
export function clauses(text: string): { text: string; start: number; end: number }[] {
	const out: { text: string; start: number; end: number }[] = [];
	for (const s of sentences(text)) {
		if (s.end - s.start <= LONG_SENTENCE) {
			out.push(s);
			continue;
		}
		const parts: { start: number; end: number }[] = [];
		let depth = 0;
		let from = s.start;
		for (let i = s.start; i < s.end; i++) {
			const ch = text[i];
			if (ch === '(' || ch === '[') depth++;
			else if ((ch === ')' || ch === ']') && depth > 0) depth--;
			else if (ch === ',' && depth === 0 && /\s/.test(text[i + 1] ?? '')) {
				parts.push({ start: from, end: i + 1 });
				from = i + 1;
			}
		}
		parts.push({ start: from, end: s.end });
		const merged: { start: number; end: number }[] = [];
		for (const p of parts) {
			let st = p.start;
			while (st < p.end && /\s/.test(text[st])) st++;
			const piece = { start: st, end: p.end };
			const last = merged[merged.length - 1];
			if (last && piece.end - piece.start < MIN_CLAUSE) last.end = piece.end;
			else merged.push(piece);
		}
		for (const m of merged) {
			let e = m.end;
			while (e > m.start && /\s/.test(text[e - 1])) e--;
			out.push({ text: text.slice(m.start, e), start: m.start, end: e });
		}
	}
	return out;
}

function stateOf(objectives: PassoObjective[]): ActionState {
	if (!objectives.length) return 'plain';
	if (objectives.some((o) => o.verdict === 'unmet')) return 'open';
	if (objectives.some((o) => o.verdict === 'unknown')) return 'unknown';
	return 'done';
}

/**
 * As ações de `text` (só a parte de `from` em diante, para pular o TL;DR já mostrado) com os
 * objetivos de cada uma: o objetivo vai para a ação onde a citação começa, e a citação vira um
 * trecho realçado.
 */
export function actionsOf(text: string, objectives: PassoObjective[], from = 0): Action[] {
	const located = objectives
		.filter((o) => o.span)
		.map((o) => ({
			o,
			start: byteToIndex(text, o.span![0]),
			end: byteToIndex(text, o.span![1])
		}));
	return clauses(text)
		.filter((s) => s.end > from)
		.map((s) => {
			const inside = located
				.filter((l) => l.start >= s.start && l.start < s.end)
				.sort((a, b) => a.start - b.start);
			const pieces: Piece[] = [];
			let at = s.start;
			for (const l of inside) {
				if (l.start < at) continue; // citações sobrepostas: a primeira fica
				const end = Math.min(l.end, s.end);
				if (l.start > at) pieces.push({ text: text.slice(at, l.start) });
				pieces.push({ text: text.slice(l.start, end), objective: l.o });
				at = end;
			}
			if (at < s.end) pieces.push({ text: text.slice(at, s.end) });
			const objs = inside.map((l) => l.o);
			return {
				text: s.text,
				start: s.start,
				end: s.end,
				pieces,
				objectives: objs,
				state: stateOf(objs)
			};
		});
}

/**
 * O que falta no passo: o que ele conclui ou inicia e a conta ainda não tem ("começar Shield of
 * Arrav", "terminar Holy Grail", "Doogle leaves"). "Não sabemos" fica de fora: não é falta.
 */
export function missingOf(objectives: PassoObjective[]): string[] {
	const out: string[] = [];
	for (const o of objectives) {
		if (o.verdict !== 'unmet' || (o.role !== 'completes' && o.role !== 'starts')) continue;
		const quest = o.key.startsWith('quest:');
		const label =
			o.role === 'starts' ? `começar ${o.label}` : quest ? `terminar ${o.label}` : o.label;
		if (!out.includes(label)) out.push(label);
	}
	return out;
}
