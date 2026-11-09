import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const API_TARGET = process.env.API_TARGET ?? 'http://localhost:8080';

export default defineConfig({
	plugins: [
		// SvelteKit 3: a configuração vive aqui (svelte.config.js não é mais lido).
		// SPA: o binário Go embute `web/dist` e serve `index.html` para qualquer rota.
		sveltekit({
			adapter: adapter({
				pages: 'dist',
				assets: 'dist',
				fallback: 'index.html',
				precompress: false,
				strict: true
			})
		})
	],
	server: {
		port: 5173,
		proxy: {
			// Tempo real (SSE): sem timeout no proxy para o stream durar; o servidor manda
			// X-Accel-Buffering: no e flush por evento, e o http-proxy repassa cada pedaço.
			'/api/events': { target: API_TARGET, changeOrigin: false, timeout: 0, proxyTimeout: 0 },
			// VNC ao vivo do Microbot (doc 08): websocket; o Go confere Origin contra Host.
			'/api/microbot/vnc': { target: API_TARGET, changeOrigin: false, ws: true },
			'/api': { target: API_TARGET, changeOrigin: false },
			// Login OIDC (AUTH_MODE=oidc): /auth/login, /auth/callback e /auth/logout são do Go.
			'^/auth/': { target: API_TARGET, changeOrigin: false }
		}
	}
});
