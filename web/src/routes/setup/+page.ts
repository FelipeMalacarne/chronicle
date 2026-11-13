import { redirect } from '@sveltejs/kit';

// O primeiro acesso em passos (v6) virou Nova conta (página 8 do canvas).
export function load() {
	redirect(308, '/contas/nova');
}
