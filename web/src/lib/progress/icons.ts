import manifest from '#lib/icons/manifest.json';

type Kind = 'item' | 'skill' | 'misc';
const files = manifest as unknown as Record<Kind, Record<string, string>> & { attribution: string };

/** Atribuição dos ícones (decisão 8 do v4); toda tela com ícone mostra no rodapé. */
export const ICON_ATTRIBUTION = files.attribution;

function url(kind: Kind, id: string): string | undefined {
	return files[kind]?.[id] ? `/icons/${kind}/${id}.png` : undefined;
}

/** Ícone do jogo para um ref ("item:314", "skill:agility", "quest:…" usa o quest point). */
export function iconFor(ref: string): string | undefined {
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
