import { test } from 'node:test';
import assert from 'node:assert/strict';
import { gameTile, parsePos, posLabel, tileCenter, tileUrl, TILE_BASE } from './mapa.ts';

test('tileUrl maps Leaflet tiles to the mejrs mapsquares', () => {
	// Lumbridge (3200..3263, 3200..3263) is mapsquare 50_50 at zoom 2; Leaflet's y grows south.
	assert.equal(tileUrl(2, 50, -51, 0), `${TILE_BASE}/2/0_50_50.png`);
	assert.equal(tileUrl(3, 100, -101, 1), `${TILE_BASE}/3/1_100_100.png`);
});

test('gameTile and tileCenter round-trip', () => {
	assert.deepEqual(gameTile(3218.7, 3222.2, 0), { x: 3222, y: 3218, plane: 0 });
	const [lat, lng] = tileCenter({ x: 3222, y: 3218, plane: 0 });
	assert.deepEqual(gameTile(lat, lng, 0), { x: 3222, y: 3218, plane: 0 });
});

test('parsePos reads typed coordinates', () => {
	assert.deepEqual(parsePos('3222 3218'), { x: 3222, y: 3218, plane: 0 });
	assert.deepEqual(parsePos('3222, 3218, 1'), { x: 3222, y: 3218, plane: 1 });
	assert.deepEqual(parsePos(' 3222;3218 ', 2), { x: 3222, y: 3218, plane: 2 });
	assert.equal(parsePos('3222'), null);
	assert.equal(parsePos('3222 3218 7'), null);
	assert.equal(parsePos('x y'), null);
	assert.equal(posLabel({ x: 1, y: 2, plane: 0 }), '1, 2');
	assert.equal(posLabel({ x: 1, y: 2, plane: 1 }), '1, 2, 1');
});
