/**
 * Automação › Cockpit (V6Microbot1440 e V6Microbot640): o que a tela diz do cliente do Microbot a
 * partir do estado que o agent devolve. Puro, testado em node.
 */
import type { AuditEntry, MicrobotStatus } from '../api/microbot.ts';

export type Tone = 'ok' | 'warn' | 'off';

export interface Card {
	k: string;
	v: string;
	tone: Tone;
	title?: string;
}

/** Os quatro fatos da faixa do cockpit: cliente, conta, agent e input humano. */
export function cards(s: MicrobotStatus | null): Card[] {
	if (!s) return [];
	const c = s.client;
	const agent: Card = s.agent.connected
		? {
				k: 'Agent',
				v: `conectado${s.agent.host ? ` · ${s.agent.host}` : ''}`,
				tone: 'ok',
				title:
					[s.agent.version, s.agent.identity && `identidade ${s.agent.identity}`]
						.filter(Boolean)
						.join(' · ') || undefined
			}
		: { k: 'Agent', v: 'desconectado · rode `ironman agent` no zaros', tone: 'off' };
	if (!s.agent.connected || !c) {
		return [
			{ k: 'Cliente', v: s.error ? `sem resposta · ${s.error}` : 'sem agent', tone: 'off' },
			{ k: 'Conta', v: '—', tone: 'off' },
			agent
		];
	}
	const mode = c.headless ? 'headless Xvfb' : 'em janela';
	const client: Card = c.running
		? {
				k: 'Cliente',
				v: `rodando · ${mode}`,
				tone: 'ok',
				title: c.state?.instance?.microbotVersion
			}
		: { k: 'Cliente', v: c.error ? `erro · ${c.error}` : 'parado', tone: c.error ? 'warn' : 'off' };
	return [client, accountCard(s), agent];
}

function accountCard(s: MicrobotStatus): Card {
	const st = s.client?.state;
	const login = s.client?.login;
	if (!s.client?.running) return { k: 'Conta', v: '—', tone: 'off' };
	const world = login?.currentWorld ? ` · mundo ${login.currentWorld}` : '';
	if (st?.loggedIn) {
		return { k: 'Conta', v: `logado · ${st.player?.name ?? '?'}${world}`, tone: 'ok' };
	}
	if (login?.loginError) return { k: 'Conta', v: `não logado · ${login.loginError}`, tone: 'warn' };
	return { k: 'Conta', v: `não logado${world}`, tone: 'off' };
}

/** As linhas do painel "Estado · GET /state". */
export function stateRows(s: MicrobotStatus | null): { k: string; v: string }[] {
	const st = s?.client?.state;
	if (!st) return [];
	const p = st.player;
	const rows: { k: string; v: string }[] = [];
	if (p?.position)
		rows.push({ k: 'Posição', v: `${p.position.x}, ${p.position.y}, ${p.position.plane}` });
	if (p?.combatLevel) rows.push({ k: 'Combat', v: String(p.combatLevel) });
	const doing = [
		p?.moving && 'andando',
		p?.animating && 'animando',
		p?.interacting && 'interagindo'
	]
		.filter(Boolean)
		.join(' · ');
	if (p) rows.push({ k: 'Agora', v: doing || 'parado' });
	rows.push({ k: 'Scripts', v: st.scriptsPaused ? 'pausados' : 'livres' });
	if (st.gameState) rows.push({ k: 'Estado do jogo', v: st.gameState });
	if (st.instance?.uptimeMs) rows.push({ k: 'Cliente no ar', v: duration(st.instance.uptimeMs) });
	return rows;
}

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
