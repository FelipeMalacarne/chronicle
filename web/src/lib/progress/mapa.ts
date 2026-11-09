/**
 * O mapa do mundo do cockpit (doc 08): os tiles renderizados de mejrs/layers_osrs (a base do mapa
 * da OSRS Wiki) num Leaflet com CRS.Simple, em que lat é o y e lng é o x do jogo. Puro, testado em
 * node.
 *
 * No zoom z do Leaflet um tile do jogo tem 2^z pixels; os zooms do mejrs são os mesmos (no 2, cada
 * imagem de 256 px é um mapsquare de 64 tiles). O y do Leaflet cresce para o sul e o do mejrs para
 * o norte, daí o -y - 1.
 */
import type { Pos } from '../api/microbot.ts';

export const TILE_BASE = 'https://raw.githubusercontent.com/mejrs/layers_osrs/master/mapsquares/-1';

/** Zooms que o mejrs renderizou. Acima do 4 o Leaflet amplia o tile do 4. */
export const MIN_ZOOM = -4;
export const MAX_NATIVE_ZOOM = 4;
export const MAX_ZOOM = 6;

/** O crédito que fica no canto do mapa. */
export const MAP_CREDIT = 'Mapa: mejrs/layers_osrs · imagens © Jagex';

/** A URL do tile (x, y) do Leaflet no zoom z, no plano do jogo. */
export function tileUrl(z: number, x: number, y: number, plane: number): string {
	return `${TILE_BASE}/${z}/${plane}_${x}_${-y - 1}.png`;
}

/** O tile do jogo sob um ponto do mapa (lat, lng do Leaflet), no plano mostrado. */
export function gameTile(lat: number, lng: number, plane: number): Pos {
	return { x: Math.floor(lng), y: Math.floor(lat), plane };
}

/** O centro de um tile do jogo no mapa, como [lat, lng]. */
export function tileCenter(p: Pos): [number, number] {
	return [p.y + 0.5, p.x + 0.5];
}

/** Lumbridge: onde o mapa abre sem a posição do jogador. */
export const DEFAULT_CENTER: Pos = { x: 3222, y: 3218, plane: 0 };

/** "3222, 3218" ou "3222, 3218, 1" fora do térreo. */
export function posLabel(p: Pos): string {
	return p.plane ? `${p.x}, ${p.y}, ${p.plane}` : `${p.x}, ${p.y}`;
}

/** Coordenadas digitadas ("3222 3218", "3222,3218,1"): um tile do jogo, ou null. */
export function parsePos(text: string, plane = 0): Pos | null {
	const n = text
		.trim()
		.split(/[\s,;]+/)
		.filter(Boolean)
		.map(Number);
	if (n.length < 2 || n.length > 3 || n.some((v) => !Number.isInteger(v) || v < 0)) return null;
	const [x, y, p = plane] = n;
	if (x > 16000 || y > 16000 || p > 3) return null;
	return { x, y, plane: p };
}
