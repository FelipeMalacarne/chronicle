import { redirect } from '@sveltejs/kit';

// A Frota é a casa do app (página 8 do canvas).
export function load() {
	redirect(308, '/');
}
