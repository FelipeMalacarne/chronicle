import type { AgentInfo, WorkerRow, WorkersView, WorkerUsage } from '../api/microbot.ts';

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
	/** As duas linhas de "Onde" na tabela: a máquina e, embaixo, como roda lá. */
	host: string;
	runtime?: string;
	/** Estado em palavras, e o tom: ok (rodando), busy (subindo), warn (falhou), idle (parado). */
	state: string;
	tone: 'ok' | 'busy' | 'warn' | 'idle';
	/** Detalhe do estado (motivo do pod, erro do último bootstrap). */
	detail?: string;
	session: string;
	sessionTone: 'ok' | 'warn';
	/** As duas linhas de "Sessão" na tabela: quando chegou e o último login. */
	sessionWhen?: string;
	sessionNote?: string;
	/** O que dá para fazer agora, na ordem dos botões. */
	actions: WorkerActionKind[];
	/** Por que Iniciar está fechado, quando está. */
	blocked?: string;
	/** Tem um agent para abrir o Cockpit desta conta. */
	cockpit: boolean;
	outdated: boolean;
	/**
	 * O último login com a sessão guardada não entrou: o motivo do jogo, quando e o personagem da
	 * sessão (para o comando que manda uma nova). Um `accounts connect` novo limpa.
	 */
	refused?: { reason: string; when: string; character: string };
	/** O pod não sobe porque a quota do namespace não tem lugar. */
	noRoom?: boolean;
	/** O que o metrics.k8s.io mediu do pod; ausente sem pod ou sem metrics-server. */
	usage?: WorkerUsage;
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
		host: 'nenhum',
		state: 'parado',
		tone: 'idle',
		session: row.session
			? `${row.session.character} · ${when(row.session.pushed_at, now)}`
			: 'sem sessão',
		sessionTone: row.session && !row.session.login_failed ? 'ok' : 'warn',
		actions: [],
		cockpit: !!row.agent || !!here,
		outdated: row.outdated,
		usage: row.usage
	};
	if (row.session?.login_failed)
		line.refused = {
			reason: row.session.last_login_result ?? '',
			when: when(row.session.last_login_at, now),
			character: row.session.character
		};
	if (row.session) {
		line.sessionWhen = `push ${when(row.session.pushed_at, now)}`;
		line.sessionNote = row.session.last_login_result
			? `último login ${row.session.last_login_result}`
			: 'ainda não usada';
	}
	if (row.session?.last_login_result)
		line.session += ` · último login: ${row.session.last_login_result}`;
	if (here) {
		line.where = `${here.host ?? 'zaros'} · anexado`;
		line.host = here.host ?? 'zaros';
		line.runtime = 'anexado';
	}
	if (row.desired === 'running') {
		line.where = `saradomin · pod ${imageTag(row.pod?.image ?? row.image)}`.trim();
		line.host = 'saradomin';
		line.runtime = `pod · ${imageTag(row.pod?.image ?? row.image)}`;
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
			line.noRoom = row.pod.state === 'blocked';
		}
		if (row.pod?.state === 'running') {
			line.state = row.agent
				? 'na tela de login ou no jogo · agent ligado'
				: 'pod de pé · esperando o agent';
			line.tone = row.agent ? 'ok' : 'busy';
		}
		if (row.last_error) {
			// An error after the pod came up is the client's (the bootstrap or a start failed): the
			// agent being linked says nothing about the game, so the line must not read as fine.
			line.detail = row.last_error;
			if (row.agent) line.state = 'agent ligado · o cliente não subiu';
			line.tone = 'warn';
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

/** O rótulo do botão: reiniciar só fala em versão nova quando o worker está numa antiga. */
export function actionLabel(line: Pick<WorkerLine, 'outdated'>, action: WorkerActionKind): string {
	return action === 'restart' && !line.outdated ? 'Reiniciar' : ACTION_LABEL[action];
}

/** Parar e reiniciar saem do jogo: pedem o segundo clique. */
export const ACTION_CONFIRM: Record<WorkerActionKind, string> = {
	start: 'Iniciar: confirmar',
	stop: 'Parar e sair do jogo?',
	restart: 'Reiniciar e sair do jogo?'
};

export interface WorkerFact {
	k: string;
	v: string;
	/** A tag da imagem, em mono antes do texto. */
	mono?: string;
	bad?: boolean;
}

const cores = (c: number) => c.toLocaleString('pt-BR', { maximumFractionDigits: 2 });
const mib = (b: number) => Math.round(b / (1024 * 1024)).toLocaleString('pt-BR');

/**
 * O cartão Worker da Visão geral (V7Conta1440): o pod, a versão, o uso, a sessão e o plugin. Sem
 * dado, a linha some; o FPS do artboard não é medido.
 */
export function workerFacts(
	row: WorkerRow | undefined,
	line: WorkerLine | undefined,
	plugin: { at?: string; bridge?: string },
	now = Date.now()
): WorkerFact[] {
	const out: WorkerFact[] = [];
	if (row?.pod) {
		const st = POD_STATES[row.pod.state];
		const restarts = row.pod.restarts
			? `${row.pod.restarts} ${row.pod.restarts === 1 ? 'reinício' : 'reinícios'}`
			: 'sem reinícios';
		out.push({
			k: 'Pod',
			v: [
				st?.label ?? row.pod.state,
				row.pod.reason ?? '',
				row.started_at ? `pedido ${when(row.started_at, now)}` : '',
				restarts
			]
				.filter(Boolean)
				.join(' · '),
			bad: st?.tone === 'warn'
		});
	} else if (line?.runtime === 'anexado') {
		out.push({ k: 'Onde', v: `${line.host} · anexado` });
	} else if (line) {
		out.push({ k: 'Estado', v: line.state, bad: line.tone === 'warn' });
	}
	if (row?.last_error) out.push({ k: 'Erro', v: row.last_error, bad: true });
	const tag = row?.pod || row?.desired === 'running' ? imageTag(row.pod?.image ?? row.image) : '';
	if (tag) out.push({ k: 'Versão', mono: tag, v: row?.outdated ? 'versão antiga' : 'atual' });
	if (row?.usage)
		out.push({
			k: 'Uso',
			v: `${cores(row.usage.cpu_cores)} núcleo · ${mib(row.usage.memory_bytes)} MB`
		});
	if (row) {
		const s = row.session;
		if (!s) out.push({ k: 'Sessão', v: 'sem sessão do jogo', bad: true });
		else {
			const login = s.login_failed
				? `último login recusado${s.last_login_result ? `: ${s.last_login_result}` : ''}`
				: s.last_login_result
					? `último login ${s.last_login_result}`
					: 'ainda não usada';
			out.push({
				k: 'Sessão',
				v: `enviada ${when(s.pushed_at, now)} · ${login}`,
				bad: !!s.login_failed
			});
		}
	}
	if (plugin.at)
		out.push({
			k: 'Plugin',
			v: `Chronicle Link${plugin.bridge ? ` ${plugin.bridge}` : ''} · envio ${when(plugin.at, now)}`
		});
	return out;
}
