import type { AgentInfo, WorkerRow, WorkersView } from '../api/microbot.ts';

/*
 * Automação › Workers (página 7 do canvas, docs/11): uma linha por conta, com onde roda, o estado
 * do pod e do agent, a sessão e as ações. Puro, testado com node --test.
 */

export type WorkerActionKind = 'start' | 'stop' | 'restart';

export interface WorkerLine {
	account: string;
	rsn: string;
	/** "saradomin · pod <tag>", "zaros · anexado" ou "nenhum". */
	where: string;
	/** Estado em palavras, e o tom: ok (rodando), busy (subindo), warn (falhou), idle (parado). */
	state: string;
	tone: 'ok' | 'busy' | 'warn' | 'idle';
	/** Detalhe do estado (motivo do pod, erro do último bootstrap). */
	detail?: string;
	session: string;
	sessionTone: 'ok' | 'warn';
	/** O que dá para fazer agora, na ordem dos botões. */
	actions: WorkerActionKind[];
	/** Por que Iniciar está fechado, quando está. */
	blocked?: string;
	/** Tem um agent para abrir o Cockpit desta conta. */
	cockpit: boolean;
	outdated: boolean;
}

/** A tag curta de uma imagem (o sha7 do commit). */
export function imageTag(image?: string): string {
	if (!image) return '';
	const at = image.indexOf('@sha256:');
	if (at >= 0) return image.slice(at + 8, at + 15);
	const colon = image.lastIndexOf(':');
	const tag = colon > image.lastIndexOf('/') ? image.slice(colon + 1) : image;
	return tag.length > 12 ? tag.slice(0, 7) : tag;
}

const POD_STATES: Record<string, { label: string; tone: WorkerLine['tone'] }> = {
	pending: { label: 'esperando espaço no nó', tone: 'warn' },
	blocked: { label: 'não cabe (quota)', tone: 'warn' },
	pulling: { label: 'subindo · baixando a imagem', tone: 'busy' },
	starting: { label: 'subindo · abrindo o cliente', tone: 'busy' },
	running: { label: 'pod de pé', tone: 'ok' },
	failing: { label: 'falhando', tone: 'warn' },
	terminating: { label: 'parando', tone: 'busy' }
};

function when(iso?: string, now = Date.now()): string {
	if (!iso) return '';
	const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
	if (s < 90) return 'agora há pouco';
	const m = Math.round(s / 60);
	if (m < 90) return `há ${m} min`;
	const h = Math.round(m / 60);
	if (h < 36) return `há ${h} h`;
	return `há ${Math.round(h / 24)} d`;
}

export function workerLine(
	row: WorkerRow,
	view: Pick<WorkersView, 'enabled'>,
	attached: AgentInfo[],
	now = Date.now()
): WorkerLine {
	const here = attached.find((a) => a.account === row.account);
	const line: WorkerLine = {
		account: row.account,
		rsn: row.rsn,
		where: 'nenhum',
		state: 'parado',
		tone: 'idle',
		session: row.session
			? `${row.session.character} · ${when(row.session.pushed_at, now)}`
			: 'sem sessão',
		sessionTone: row.session ? 'ok' : 'warn',
		actions: [],
		cockpit: !!row.agent || !!here,
		outdated: row.outdated
	};
	if (row.session?.last_login_result)
		line.session += ` · último login: ${row.session.last_login_result}`;
	if (here) line.where = `${here.host ?? 'zaros'} · anexado`;
	if (row.desired === 'running') {
		line.where = `saradomin · pod ${imageTag(row.pod?.image ?? row.image)}`.trim();
		const pod = row.pod ? POD_STATES[row.pod.state] : undefined;
		if (!row.pod && row.agent) {
			// Fora do cluster (um worker subido à mão contra este servidor) ou antes da primeira
			// passada do reconciliador: o agent ligado é o que importa.
			line.state = 'agent ligado';
			line.tone = 'ok';
		} else if (!row.pod) {
			line.state = view.enabled ? 'pedido · esperando o reconciliador' : 'sem pod (WORKERS=off)';
			line.tone = view.enabled ? 'busy' : 'idle';
		} else if (pod) {
			line.state = pod.label;
			line.tone = pod.tone;
			line.detail = row.pod.reason;
		}
		if (row.pod?.state === 'running') {
			line.state = row.agent
				? 'na tela de login ou no jogo · agent ligado'
				: 'pod de pé · esperando o agent';
			line.tone = row.agent ? 'ok' : 'busy';
		}
		if (row.last_error) {
			line.detail = row.last_error;
			if (line.tone !== 'ok') line.tone = 'warn';
		}
		line.actions = !view.enabled ? [] : row.outdated ? ['restart', 'stop'] : ['stop', 'restart'];
	} else {
		if (here) line.state = 'ativa no zaros';
		line.actions = ['start'];
		if (!view.enabled) line.blocked = 'workers desligados neste servidor (WORKERS=off)';
		else if (!row.session)
			line.blocked = 'conecte a conta: `chronicle accounts connect --character <nome>`';
		else if (here) line.blocked = 'a conta está no agent do zaros; pare lá antes';
	}
	return line;
}

export function workerLines(view: WorkersView | null, now = Date.now()): WorkerLine[] {
	if (!view) return [];
	return view.workers.map((w) => workerLine(w, view, view.attached, now));
}

export const ACTION_LABEL: Record<WorkerActionKind, string> = {
	start: 'Iniciar',
	stop: 'Parar',
	restart: 'Reiniciar na versão nova'
};

/** Parar e reiniciar saem do jogo: pedem o segundo clique. */
export const ACTION_CONFIRM: Record<WorkerActionKind, string> = {
	start: 'Iniciar: confirmar',
	stop: 'Parar e sair do jogo?',
	restart: 'Reiniciar e sair do jogo?'
};
