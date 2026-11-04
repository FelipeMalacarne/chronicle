/** Navegação ciente do guia: o guia escolhido viaja na query string (`?guide=`). */

export const DEFAULT_GUIDE = 'oziris-main';

export function planQuery(params: Record<string, string | null | undefined>): string {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
	const s = q.toString();
	return s ? `?${s}` : '';
}

/** Valida o guia vindo da URL contra os guias conhecidos; cai no padrão se inválido. */
export function pickGuide(raw: string | null, known: readonly string[]): string {
	if (raw && known.includes(raw)) return raw;
	return known.includes(DEFAULT_GUIDE) ? DEFAULT_GUIDE : (known[0] ?? DEFAULT_GUIDE);
}

/** No comparativo, as duas faixas precisam ser guias diferentes. */
export function pickPair(
	a: string | null,
	b: string | null,
	known: readonly string[]
): [string, string] {
	const left = pickGuide(a, known);
	const fallback = known.find((g) => g !== left) ?? left;
	const right = b && known.includes(b) && b !== left ? b : fallback;
	return [left, right];
}
