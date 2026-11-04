import { redirect } from '@sveltejs/kit';

// Caminho anterior às telas v4 (canvas, página v4).
export function load() {
	redirect(308, '/mundo');
}
