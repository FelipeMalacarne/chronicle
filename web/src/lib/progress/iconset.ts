/**
 * Os ids de item com ícone baixado vêm no manifesto como faixas ("1-3,7,9-12"), para o bundle
 * não carregar um mapa com milhares de entradas (tools/fetch_wiki_icons.py).
 */
export function parseRanges(ranges: string): Set<number> {
	const out = new Set<number>();
	for (const part of ranges.split(',')) {
		const p = part.trim();
		if (!p) continue;
		const dash = p.indexOf('-');
		if (dash < 0) {
			const n = Number(p);
			if (Number.isInteger(n)) out.add(n);
			continue;
		}
		const a = Number(p.slice(0, dash));
		const b = Number(p.slice(dash + 1));
		if (!Number.isInteger(a) || !Number.isInteger(b) || b < a) continue;
		for (let n = a; n <= b; n++) out.add(n);
	}
	return out;
}
