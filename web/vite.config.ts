import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, type Plugin } from 'vite';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const API_TARGET = process.env.API_TARGET ?? 'http://localhost:8080';

/**
 * Mock da API apenas em dev (`pnpm dev:mock` / MOCK_API=1).
 * Serve o contrato do Go a partir de ../data/guide/normalized/guide.json,
 * sem incluir o JSON no bundle. Em `pnpm dev` normal, /api é proxy para o Go.
 */
function mockApi(): Plugin {
	return {
		name: 'ironman-mock-api',
		apply: 'serve',
		configureServer(server) {
			const file = resolve(import.meta.dirname, '../data/guide/normalized/guide.json');
			if (!existsSync(file)) {
				server.config.logger.warn(`[mock-api] ${file} não encontrado; mock desligado`);
				return;
			}
			type RawStep = Record<string, unknown> & { id: string };
			type RawSection = {
				id: string;
				slug: string;
				title: string;
				description?: string;
				track: boolean;
				steps: RawStep[];
			};
			const guide = JSON.parse(readFileSync(file, 'utf8')) as { sections: RawSection[] };
			const sections = guide.sections;
			const steps = sections.flatMap((s) => s.steps);
			const json = (res: import('node:http').ServerResponse, status: number, body: unknown) => {
				res.statusCode = status;
				res.setHeader('content-type', 'application/json; charset=utf-8');
				res.end(JSON.stringify(body));
			};
			const DONE = 24; // progresso fictício só para a tela ter algo a mostrar
			server.middlewares.use((req, res, next) => {
				const url = new URL(req.url ?? '/', 'http://localhost');
				const p = url.pathname;
				if (!p.startsWith('/api/')) return next();
				if (p === '/api/health') {
					return json(res, 200, {
						status: 'ok',
						version: 'dev-mock',
						content: { guides: 1, steps: steps.length },
						db: 'absent'
					});
				}
				if (p === '/api/guide') {
					return json(
						res,
						200,
						sections.map((s) => ({
							id: s.id,
							slug: s.slug,
							title: s.title,
							steps: s.steps.length,
							track: s.track
						}))
					);
				}
				let m = p.match(/^\/api\/guide\/([^/]+)$/);
				if (m) {
					const key = decodeURIComponent(m[1]);
					const s = sections.find((x) => x.slug === key || x.id === key);
					if (!s) return json(res, 404, { error: 'section not found' });
					return json(res, 200, {
						id: s.id,
						slug: s.slug,
						title: s.title,
						description: s.description,
						track: s.track,
						steps: s.steps
					});
				}
				m = p.match(/^\/api\/steps\/([^/]+)$/);
				if (m) {
					const id = decodeURIComponent(m[1]);
					const st = steps.find((x) => x.id === id);
					return st ? json(res, 200, st) : json(res, 404, { error: 'step not found' });
				}
				if (p === '/api/now') {
					return json(res, 200, {
						current: steps[DONE],
						next: steps.slice(DONE + 1, DONE + 7),
						progress: { done: DONE, total: steps.length }
					});
				}
				return json(res, 404, { error: 'not found' });
			});
		}
	};
}

export default defineConfig({
	plugins: [
		...(process.env.MOCK_API ? [mockApi()] : []),
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
