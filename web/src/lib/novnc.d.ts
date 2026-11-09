// The part of noVNC's RFB (@novnc/novnc, which ships no types) the cockpit uses.
declare module '@novnc/novnc' {
	export default class RFB extends EventTarget {
		constructor(
			target: HTMLElement,
			urlOrChannel: string | WebSocket,
			options?: { shared?: boolean; credentials?: { password?: string }; wsProtocols?: string[] }
		);
		viewOnly: boolean;
		scaleViewport: boolean;
		resizeSession: boolean;
		focusOnClick: boolean;
		background: string;
		qualityLevel: number;
		compressionLevel: number;
		disconnect(): void;
		focus(): void;
	}
}
