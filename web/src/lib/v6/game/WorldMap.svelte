<script lang="ts">
	import 'leaflet/dist/leaflet.css';
	import type * as Leaflet from 'leaflet';
	import type { Pos } from '#lib/api/microbot';
	import {
		DEFAULT_CENTER,
		gameTile,
		MAP_CREDIT,
		MAX_NATIVE_ZOOM,
		MAX_ZOOM,
		MIN_ZOOM,
		parsePos,
		posLabel,
		tileCenter,
		tileUrl
	} from '#lib/progress/mapa';
	import type { MenuOption } from '#lib/progress/jogo';
	import { untrack } from 'svelte';
	import Glyph from '#lib/v6/Glyph.svelte';
	import { gameMap, gameMenu, openMenu, runAction } from './menu.svelte';

	/*
	 * O mapa do mundo do cockpit (doc 08), como o orb do mapa no cliente: os tiles do mejrs num
	 * Leaflet, o jogador em amarelo e o destino em vermelho. Clicar abre o mesmo "Choose Option" com
	 * Walk here, que anda pelo webwalker do Microbot (teleports e atalhos que ele conhece).
	 */
	let { me }: { me?: Pos } = $props();

	let el = $state<HTMLDivElement>();
	/** O plano mostrado: abre no do jogador e depois é do dono. */
	let plane = $state(untrack(() => me?.plane ?? 0));
	let hover = $state<Pos | null>(null);
	let typed = $state('');
	let L: typeof Leaflet | undefined;
	let map: Leaflet.Map | undefined;
	let tiles: Leaflet.TileLayer | undefined;
	let meMark: Leaflet.CircleMarker | undefined;
	let destMark: Leaflet.CircleMarker | undefined;
	let shownPlane = 0;

	/** Um PNG transparente para os tiles que não existem (fora do mapa). */
	const EMPTY =
		'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';

	function walkMenu(p: Pos): MenuOption[] {
		return [
			{ label: 'Walk here', target: posLabel(p), tone: 'object', command: 'walk', body: { ...p } }
		];
	}

	$effect(() => {
		if (!el) return;
		const box = el;
		let dead = false;
		void import('leaflet').then((mod) => {
			if (dead) return;
			const lf = (mod as unknown as { default?: typeof Leaflet }).default ?? mod;
			L = lf;
			const Tiles = lf.TileLayer.extend({
				getTileUrl(c: Leaflet.Coords) {
					return tileUrl(c.z, c.x, c.y, shownPlane);
				}
			}) as new (url: string, options: Leaflet.TileLayerOptions) => Leaflet.TileLayer;
			const m = lf.map(box, {
				crs: lf.CRS.Simple,
				minZoom: MIN_ZOOM,
				maxZoom: MAX_ZOOM,
				zoomSnap: 1,
				attributionControl: true
			});
			m.attributionControl.setPrefix(false);
			tiles = new Tiles('', {
				tileSize: 256,
				minZoom: MIN_ZOOM,
				maxNativeZoom: MAX_NATIVE_ZOOM,
				maxZoom: MAX_ZOOM,
				noWrap: true,
				errorTileUrl: EMPTY,
				attribution: MAP_CREDIT
			}).addTo(m);
			m.setView(tileCenter(me ?? DEFAULT_CENTER), 2);
			const at = (e: Leaflet.LeafletMouseEvent) => gameTile(e.latlng.lat, e.latlng.lng, plane);
			m.on('mousemove', (e: Leaflet.LeafletMouseEvent) => (hover = at(e)));
			m.on('mouseout', () => (hover = null));
			const choose = (e: Leaflet.LeafletMouseEvent) => {
				const p = at(e);
				gameMap.dest = p;
				openMenu(e.originalEvent, walkMenu(p));
			};
			m.on('click', choose);
			m.on('contextmenu', choose);
			map = m;
			marks();
		});
		return () => {
			dead = true;
			map?.remove();
			map = tiles = meMark = destMark = undefined;
		};
	});

	/** O jogador e o destino, no plano mostrado. */
	function marks() {
		if (!map || !L) return;
		meMark?.remove();
		destMark?.remove();
		meMark = destMark = undefined;
		if (me && me.plane === plane)
			meMark = L.circleMarker(tileCenter(me), {
				radius: 6,
				color: '#000',
				weight: 1,
				fillColor: '#ffff00',
				fillOpacity: 1
			})
				.bindTooltip(`Você · ${posLabel(me)}`)
				.addTo(map);
		const d = gameMap.dest;
		if (d && d.plane === plane)
			destMark = L.circleMarker(tileCenter(d), {
				radius: 5,
				color: '#000',
				weight: 1,
				fillColor: '#ff3030',
				fillOpacity: 1
			}).addTo(map);
	}

	$effect(() => {
		// Relê quando o jogador anda, o destino muda ou o plano troca.
		void [me?.x, me?.y, me?.plane, gameMap.dest, plane];
		if (shownPlane !== plane) {
			shownPlane = plane;
			tiles?.redraw();
		}
		marks();
	});

	function center(p: Pos) {
		plane = p.plane;
		map?.setView(tileCenter(p), Math.max(map.getZoom(), 3));
	}

	function walkTyped(e: SubmitEvent) {
		e.preventDefault();
		const p = parsePos(typed, plane);
		if (!p) return;
		gameMap.dest = p;
		center(p);
		void runAction(walkMenu(p)[0]);
	}

	function close() {
		gameMap.open = false;
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'Escape' && !gameMenu.open && !gameMenu.selected) close();
	}
</script>

<svelte:window onkeydown={key} />

<button class="wscrim" type="button" onclick={close} aria-label="Fechar o mapa"></button>
<div class="panel wmap" role="dialog" aria-label="Mapa do mundo">
	<div class="plaque">
		Mapa do mundo<span class="r"
			>{hover ? posLabel(hover) : me ? `você: ${posLabel(me)}` : 'clique para andar'}</span
		>
	</div>
	<div class="wbar">
		<div class="planes" role="group" aria-label="Plano">
			{#each [0, 1, 2, 3] as p (p)}
				<button
					type="button"
					class="btn btn-s"
					class:btn-g={plane !== p}
					aria-pressed={plane === p}
					title={p ? `Andar ${p}` : 'Térreo'}
					onclick={() => (plane = p)}>{p}</button
				>
			{/each}
		</div>
		<button class="btn btn-s btn-g" type="button" disabled={!me} onclick={() => me && center(me)}
			>Você</button
		>
		<form class="go" onsubmit={walkTyped}>
			<input
				class="wnum"
				placeholder="x, y"
				aria-label="Coordenadas"
				bind:value={typed}
				spellcheck="false"
			/>
			<button
				class="btn btn-s"
				type="submit"
				disabled={gameMenu.busy || !parsePos(typed, plane)}
				title="Walk here pelo webwalker">Andar</button
			>
		</form>
		<button class="x" type="button" onclick={close} aria-label="Fechar o mapa"
			><Glyph name="x" size={18} /></button
		>
	</div>
	<div class="wview" bind:this={el}></div>
</div>

<style>
	.wscrim {
		position: fixed;
		inset: 0;
		z-index: 50;
		border: 0;
		background: rgba(0, 0, 0, 0.45);
		cursor: default;
	}
	.wmap {
		position: fixed;
		z-index: 51;
		inset: 5vh 5vw;
		display: flex;
		flex-direction: column;
		max-width: 1280px;
		margin: 0 auto;
	}
	.wbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
	}
	.planes {
		display: flex;
		gap: 2px;
	}
	.go {
		display: flex;
		gap: 6px;
		margin-left: auto;
	}
	.wnum {
		width: 120px;
		height: 30px;
		box-sizing: border-box;
		padding: 0 8px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--ground);
		color: var(--ink);
		font:
			400 12px 'JetBrains Mono',
			monospace;
	}
	.x {
		border: 0;
		background: transparent;
		color: var(--ink-muted);
		cursor: pointer;
		padding: 4px;
	}
	/* Dentro, o mapa do jogo: o fundo preto de fora do mundo e o cursor de mira. */
	.wview {
		flex: 1;
		min-height: 0;
		margin: 0 12px 12px;
		border-radius: 3px;
		background: #000;
		cursor: crosshair;
	}
	.wview :global(.leaflet-tile) {
		image-rendering: pixelated;
	}
	.wview :global(.leaflet-control-attribution) {
		font:
			400 10px 'JetBrains Mono',
			monospace;
		background: rgba(0, 0, 0, 0.6);
		color: #c8c0a8;
	}
	@media (max-width: 640px) {
		.wmap {
			inset: 8px;
		}
		.go {
			margin-left: 0;
		}
	}
</style>
