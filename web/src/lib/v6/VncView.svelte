<script lang="ts">
	import type RFB from '@novnc/novnc';

	/*
	 * Tela ao vivo do cliente (doc 08 fase 2): noVNC sobre o websocket /api/microbot/vnc, que o planner
	 * emenda pelo agent até o x11vnc do headless. viewOnly vem ligado; "Intervir com mouse e teclado"
	 * (V6Microbot1440) o desliga. O noVNC carrega só quando a tela abre: usa top-level await e pesa.
	 */
	export type VncPhase = 'connecting' | 'connected' | 'closed';

	let {
		url,
		viewOnly = true,
		onphase
	}: {
		url: string;
		viewOnly?: boolean;
		onphase?: (p: VncPhase, reason?: string) => void;
	} = $props();

	let host = $state<HTMLDivElement>();
	let rfb = $state<RFB | null>(null);

	$effect(() => {
		const el = host;
		if (!el) return;
		let gone = false;
		let r: RFB | null = null;
		onphase?.('connecting');
		void import('@novnc/novnc').then(({ default: RFBClass }) => {
			if (gone) return;
			r = new RFBClass(el, url, { shared: true });
			r.scaleViewport = true;
			r.background = 'transparent';
			r.qualityLevel = 6;
			r.addEventListener('connect', () => onphase?.('connected'));
			r.addEventListener('disconnect', (e) => {
				const clean = (e as CustomEvent<{ clean: boolean }>).detail?.clean;
				onphase?.('closed', clean ? undefined : 'a conexão caiu');
			});
			rfb = r;
		});
		return () => {
			gone = true;
			r?.disconnect();
			rfb = null;
		};
	});
	$effect(() => {
		if (!rfb) return;
		rfb.viewOnly = viewOnly;
		if (!viewOnly) rfb.focus();
	});
</script>

<div class="vnc" class:live={!viewOnly} bind:this={host}></div>

<style>
	.vnc {
		position: absolute;
		inset: 0;
	}
	.vnc.live {
		outline: 2px solid var(--gold);
		outline-offset: -2px;
	}
</style>
