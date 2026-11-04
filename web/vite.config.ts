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
			'/api': { target: API_TARGET, changeOrigin: false }
		}
	}
});
