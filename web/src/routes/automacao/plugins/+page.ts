import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';

// The Plugins sub-tab left Conta › Cliente (canvas round 4, brief section 13): starting, stopping
// and configuring live in the Plugins card of the Cliente tab; Hub, versions and removal in /plugins.
export function load() {
	redirect(307, resolve('/plugins'));
}
