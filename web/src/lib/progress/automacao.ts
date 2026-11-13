/**
 * O que o cliente do Microbot fez: a duração e a auditoria (quem, o quê), lidas na Visão geral da
 * conta. Puro, testado em node.
 */
import type { AuditEntry } from '../api/microbot.ts';

export function duration(ms: number): string {
	const m = Math.floor(ms / 60000);
	if (m < 60) return `${m} min`;
	const h = Math.floor(m / 60);
	return `${h} h ${m % 60} min`;
}

const ACTION_LABEL: Record<string, string> = {
	client_start: 'iniciar cliente (headless)',
	client_stop: 'parar cliente',
	login: 'login',
	logout: 'logout',
	screenshot: 'screenshot',
	status: 'estado',
	vnc_open: 'abrir VNC ao vivo',
	plugin_install: 'instalar plugin',
	plugin_remove: 'remover plugin',
	script_start: 'iniciar plugin',
	script_stop: 'parar plugin',
	scripts_pause: 'pausar scripts',
	scripts_resume: 'retomar scripts',
	config_set: 'configuração',
	walk: 'andar',
	inventory_use: 'usar item',
	inventory_drop: 'dropar',
	bank_open: 'abrir banco',
	bank_close: 'fechar banco',
	bank_deposit: 'depositar',
	bank_withdraw: 'sacar',
	npc_interact: 'NPC',
	object_interact: 'objeto',
	ground_pickup: 'pegar do chão',
	dialogue_continue: 'continuar diálogo',
	dialogue_select: 'opção de diálogo',
	world_hop: 'trocar de mundo',
	account_use: 'trocar personagem',
	run_start: 'fazer agora',
	run_cancel: 'cancelar o fazer agora',
	recurring_set: 'gravar rotina',
	recurring_delete: 'apagar rotina',
	profile_save: 'salvar no perfil',
	profile_apply: 'aplicar o perfil',
	trigger_set: 'gatilho'
};

/** Uma linha da auditoria: "login · mundo 303 · ok". */
export function auditLine(e: AuditEntry): string {
	const parts = [ACTION_LABEL[e.action] ?? e.action];
	const p = e.params ?? {};
	if (typeof p.world === 'number') parts.push(`mundo ${p.world}`);
	if (typeof p.x === 'number' && typeof p.y === 'number')
		parts.push(`${p.x}, ${p.y}${typeof p.plane === 'number' && p.plane ? `, ${p.plane}` : ''}`);
	if (typeof p.action === 'string') parts.push(p.action);
	if (typeof p.id === 'number' && typeof p.name !== 'string') parts.push(`id ${p.id}`);
	if (typeof p.quantity === 'number') parts.push(`×${p.quantity}`);
	if (p.all === true) parts.push('tudo');
	if (typeof p.option === 'string') parts.push(p.option);
	if (typeof p.index === 'number') parts.push(`opção ${p.index}`);
	if (typeof p.name === 'string') parts.push(p.version ? `${p.name} ${String(p.version)}` : p.name);
	if (typeof p.className === 'string') parts.push(p.className.split('.').pop() ?? p.className);
	if (e.action === 'trigger_set')
		parts.push(
			`${p.event === 'kitten_grown' ? 'quando o filhote crescer' : String(p.event)}: ${p.enabled ? 'ligado' : 'desligado'}`
		);
	if (typeof p.group === 'string' && typeof p.key === 'string')
		parts.push(`${p.group}.${p.key} = ${p.value === null ? '(padrão)' : String(p.value)}`);
	parts.push(e.ok ? 'ok' : `falhou: ${e.error ?? ''}`.trim());
	return parts.join(' · ');
}

/** O ator da auditoria como a tela mostra: "você" para o usuário da web. */
export function auditWho(e: AuditEntry): string {
	return e.origin === 'web' ? 'você' : e.origin;
}
