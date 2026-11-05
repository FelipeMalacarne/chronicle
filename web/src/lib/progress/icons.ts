import manifest from '#lib/icons/manifest.json';
import { parseRanges } from './iconset.ts';
import { iconRef } from './view.ts';

/**
 * Ícones do jogo baixados da OSRS Wiki por tools/fetch_wiki_icons.py: todo item obtível do
 * catálogo (os itens vêm como faixas de id), os skills e o quest point.
 */
const files = manifest as unknown as {
	attribution: string;
	item_ids?: string;
	/** Formato anterior do manifesto: id -> arquivo da wiki. */
	item?: Record<string, string>;
	skill: Record<string, string>;
	misc: Record<string, string>;
};
const items = files.item_ids
	? parseRanges(files.item_ids)
	: new Set(Object.keys(files.item ?? {}).map(Number));

/** Atribuição dos ícones (decisão 8 do v4); toda tela com ícone mostra no rodapé. */
export const ICON_ATTRIBUTION = files.attribution;

function url(kind: 'item' | 'skill' | 'misc', id: string): string | undefined {
	const has = kind === 'item' ? items.has(Number(id)) : !!files[kind]?.[id];
	return has ? `/icons/${kind}/${id}.png` : undefined;
}

/**
 * Ícone do jogo para um ref ("item:314", "skill:agility", "quest:…" usa o quest point); alternativas
 * (`item:any(...)`) usam a primeira, família fica sem ícone.
 */
export function iconFor(subject: string): string | undefined {
	const ref = iconRef(subject);
	if (!ref) return undefined;
	const colon = ref.indexOf(':');
	if (colon < 0) return undefined;
	const kind = ref.slice(0, colon);
	const id = ref.slice(colon + 1);
	if (kind === 'item') return url('item', id);
	if (kind === 'skill') return url('skill', id);
	if (kind === 'quest') return url('misc', 'quest_point');
	return undefined;
}

export function itemIcon(id: number | undefined): string | undefined {
	return id === undefined ? undefined : url('item', String(id));
}
