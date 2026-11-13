/*
 * A tela Plugins da v7 (fatia V8, artboard V7Plugins1440): o que as contas sabem fazer. O padrão
 * da frota é o perfil modelo `frota`; cada conta tem o seu perfil sobre ele, onde fica o ajuste
 * próprio (decisão do dono de 2026-11-13). Os grupos saem do uso: o que liga no boot é "Sempre
 * ligados", o que um gatilho roda é "Rotinas usam", o resto instalado é "Jogam". Lógica pura,
 * testada com node --test.
 */

export const FLEET_MODEL = 'frota';

export type PluginSource = 'hub' | 'own' | 'core';
export type PluginGroup = 'plays' | 'routines' | 'always';

export interface ProfilePlugin {
	source: PluginSource;
	name: string;
	version?: string;
	class_name?: string;
	start: boolean;
}

export interface ProfileSetting {
	group: string;
	key: string;
	value: string;
}

/** Uma camada de perfil como `PUT /api/client-profiles/{nome}` recebe. */
export interface ProfileLayer {
	parent?: string;
	plugins: ProfilePlugin[];
	settings: ProfileSetting[];
}

export interface StoredProfile extends ProfileLayer {
	name: string;
	accounts: string[];
	updated_at: string;
}

export interface FleetPluginAccount {
	account: string;
	rsn: string;
	profile?: string;
	parent?: string;
	/** As configurações e os plugins da camada própria do perfil da conta. */
	own: ProfileSetting[];
	own_plugins: ProfilePlugin[];
	connected: boolean;
}

export interface FleetPluginState {
	account: string;
	in_profile: boolean;
	installed?: string;
	active?: boolean;
}

export interface FleetPlugin {
	source: PluginSource;
	name: string;
	class_name?: string;
	title?: string;
	group: PluginGroup;
	in_model: boolean;
	model?: ProfilePlugin;
	latest?: string;
	accounts: FleetPluginState[];
}

export interface FleetPlugins {
	model: StoredProfile | null;
	accounts: FleetPluginAccount[];
	plugins: FleetPlugin[];
}

export const GROUPS: { key: PluginGroup; title: string; hint: string }[] = [
	{ key: 'plays', title: 'Jogam', hint: 'viram a atividade principal de uma conta' },
	{ key: 'routines', title: 'Rotinas usam', hint: 'rodam até terminar e devolvem a conta' },
	{ key: 'always', title: 'Sempre ligados', hint: 'não jogam' }
];

export function pluginId(p: { source: string; name: string }): string {
	return `${p.source}:${p.name}`;
}

/** A classe que o cliente liga: a do plugin do Hub, ou o próprio nome dos outros. */
export function pluginClass(p: { source: string; name: string; class_name?: string }): string {
	return p.source === 'hub' ? (p.class_name ?? '') : p.name;
}

/** O nome que a tela mostra: o do cliente ou do Hub, senão o fim da classe sem "Plugin". */
export function pluginTitle(p: FleetPlugin): string {
	if (p.title) return p.title;
	if (p.source === 'hub') return p.name.replace(/Plugin$/, '');
	const last = p.name.split('.').pop() ?? p.name;
	return last.replace(/Plugin$/, '').replace(/([a-z])([A-Z])/g, '$1 $2');
}

/** As contas onde o plugin está: no perfil efetivo, instalado ou listado no cliente. */
export function present(p: FleetPlugin): FleetPluginState[] {
	return p.accounts.filter(
		(s) => s.in_profile || s.installed !== undefined || s.active !== undefined
	);
}

/** A linha da direita da lista: onde roda, onde está, e de quem é. */
export function statusText(p: FleetPlugin, accounts: number): string {
	const here = present(p);
	const running = here.filter((s) => s.active === true).length;
	let text: string;
	if (p.group === 'always') text = `em ${running} de ${accounts}`;
	else if (running > 0) text = `rodando em ${running} de ${here.length}`;
	else if (here.length > 0) text = `instalado em ${here.length}`;
	else if (p.in_model) text = 'no padrão da frota';
	else text = 'em nenhuma conta';
	return p.source === 'own' ? `seu · ${text}` : text;
}

/** As versões instaladas, sem repetir. */
export function installedVersions(p: FleetPlugin): string[] {
	const out: string[] = [];
	for (const s of p.accounts) if (s.installed && !out.includes(s.installed)) out.push(s.installed);
	return out;
}

/** A versão nova do Hub, quando alguma conta tem outra. */
export function update(p: FleetPlugin): string | null {
	if (!p.latest) return null;
	return installedVersions(p).some((v) => v !== p.latest) ? p.latest : null;
}

/** As contas que ganham a versão nova num "Atualizar". */
export function outdated(p: FleetPlugin): string[] {
	if (!p.latest) return [];
	return p.accounts.filter((s) => s.installed && s.installed !== p.latest).map((s) => s.account);
}

/** O nome do perfil próprio de uma conta (o servidor aceita minúsculas, dígitos, - e _). */
export function accountProfileName(account: string): string {
	return (
		account
			.toLowerCase()
			.replace(/[^a-z0-9_-]/g, '-')
			.replace(/^[^a-z0-9]+/, '')
			.slice(0, 48) || 'conta'
	);
}

/** A camada com o plugin (trocando o que já houver com a mesma origem e nome). */
export function withPlugin(layer: ProfileLayer, p: ProfilePlugin): ProfileLayer {
	const i = layer.plugins.findIndex((x) => pluginId(x) === pluginId(p));
	const plugins = [...layer.plugins];
	if (i >= 0) plugins[i] = p;
	else plugins.push(p);
	return { ...layer, plugins };
}

export function withoutPlugin(layer: ProfileLayer, id: string): ProfileLayer {
	return { ...layer, plugins: layer.plugins.filter((x) => pluginId(x) !== id) };
}

/** A camada com a configuração; null tira a chave (volta ao que vier por baixo). */
export function withSetting(
	layer: ProfileLayer,
	group: string,
	key: string,
	value: string | null
): ProfileLayer {
	const settings = layer.settings.filter((s) => !(s.group === group && s.key === key));
	if (value !== null) settings.push({ group, key, value });
	return { ...layer, settings };
}

/** Os valores de um grupo numa camada, para o formulário. */
export function settingsOf(
	settings: ProfileSetting[],
	group: string,
	keys: string[]
): Record<string, string | null> {
	const out: Record<string, string | null> = Object.fromEntries(keys.map((k) => [k, null]));
	for (const s of settings) if (s.group === group && keys.includes(s.key)) out[s.key] = s.value;
	return out;
}

/** Se a conta ajusta este grupo de config no perfil dela. */
export function ownAdjust(a: FleetPluginAccount, group: string | null): boolean {
	return !!group && a.own.some((s) => s.group === group);
}
