/*
 * Capacidade dos workers no resumo da Frota (página 8 do canvas, V7Frota1440; docs/11, F7): quantos
 * workers ocupam a quota, quantos ainda cabem e o que limita, e o uso medido dos pods contra o teto
 * da quota. O uso do nó ("CPU 3,1 / 4" do artboard) não existe: pediria ler os nós do cluster.
 * Puro, testado com node --test.
 */
import type { WorkerCapacity, WorkersView, WorkerUsage } from '../api/microbot.ts';
import type { LaneTone } from './frota.ts';

const ONE = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 });
const TWO = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 2 });
const GIB = 2 ** 30;
const MIB = 2 ** 20;

/** Núcleos em PT-BR, com uma casa ("0,4", "2,2", "4"); abaixo de 0,05, duas ("0,04"). */
export function cores(n: number): string {
	return (n > 0 && n < 0.05 ? TWO : ONE).format(n);
}

/** "0,7 núcleo", "2,2 núcleos". */
export function coresLabel(n: number): string {
	return `${cores(n)} ${n >= 2 ? 'núcleos' : 'núcleo'}`;
}

/** Bytes em GiB com uma casa, sem a unidade ("0,8", "3,3"): o par do medidor. */
export function gib(bytes: number): string {
	return ONE.format(bytes / GIB);
}

/** "837 MiB" abaixo de 1 GiB; "1 GiB", "3,3 GiB" a partir dele. */
export function bytesLabel(bytes: number): string {
	return bytes < GIB ? `${Math.round(bytes / MIB)} MiB` : `${gib(bytes)} GiB`;
}

const LIMITS: Record<string, string> = {
	'requests.cpu': 'a CPU pedida',
	cpu: 'a CPU pedida',
	'requests.memory': 'a memória pedida',
	memory: 'a memória pedida',
	'limits.cpu': 'o limite de CPU',
	'limits.memory': 'o limite de memória',
	pods: 'o número de pods',
	'count/pods': 'o número de pods',
	'count/deployments.apps': 'o número de Deployments',
	'count/replicasets.apps': 'o número de ReplicaSets'
};

/** O recurso da quota que dá o "cabe mais N", em palavras. */
export function limitLabel(by: string): string {
	return LIMITS[by] ?? by;
}

/** "cabe mais 1", "cabem mais 2", "não cabe mais nenhum". */
export function fitsShort(fits: number): string {
	if (fits <= 0) return 'não cabe mais nenhum';
	return fits === 1 ? 'cabe mais 1' : `cabem mais ${fits}`;
}

/** A frase da raia de Nova conta: "Cabe mais 1 worker no saradomin". */
export function fitsSentence(fits: number, host = 'saradomin'): string {
	if (fits <= 0) return `Não cabe mais nenhum worker no ${host}`;
	return fits === 1 ? `Cabe mais 1 worker no ${host}` : `Cabem mais ${fits} workers no ${host}`;
}

/** A etiqueta da raia de Nova conta. */
export function freeSlots(fits: number): string {
	if (fits <= 0) return 'sem lugar livre';
	return fits === 1 ? '1 lugar livre' : `${fits} lugares livres`;
}

/** O uso de um worker, para o título da etiqueta de onde ele roda: "CPU 0,4 · RAM 837 MiB". */
export function usageLine(u?: WorkerUsage): string {
	return u ? `CPU ${cores(u.cpu_cores)} · RAM ${bytesLabel(u.memory_bytes)}` : '';
}

/** Por que um worker não sobe (estado `blocked` do pod), com o número quando a quota o dá. */
export function roomHint(cap?: WorkerCapacity): string {
	const tail = 'Pare outra conta ou deixe esta na espera.';
	if (!cap) return tail;
	const by = cap.limited_by;
	if ((by === 'requests.cpu' || by === 'cpu') && cap.cpu_free_cores !== undefined)
		return `O worker pede ${coresLabel(cap.worker_cpu_cores)} e sobram ${cores(Math.max(0, cap.cpu_free_cores))}. ${tail}`;
	if ((by === 'requests.memory' || by === 'memory') && cap.memory_free_bytes !== undefined)
		return `O worker pede ${bytesLabel(cap.worker_memory_bytes)} e sobram ${bytesLabel(Math.max(0, cap.memory_free_bytes))}. ${tail}`;
	return `O limite é ${limitLabel(by)}. ${tail}`;
}

export interface Meter {
	/** De 0 a 100, para a largura da barra. */
	pct: number;
	/** "0,4 / 2,2". */
	label: string;
}

/** Um lugar na quota: o tom da raia da conta que o ocupa, ou livre. */
export type Slot = LaneTone | 'free';

export interface FleetCapacity {
	/** Workers que ocupam a quota (os pedidos para rodar). */
	used: number;
	/** `used` + o que ainda cabe; ausente sem a capacidade (sem quota, WORKERS=off). */
	total?: number;
	fits?: number;
	/** Um por lugar, ocupados primeiro (no jogo, subindo, com problema, parados) e os livres. */
	slots: Slot[];
	/** "cabe mais 1 · zaros com 1 cliente". */
	line: string;
	/** Quanto um worker pede e o que limita. */
	title?: string;
	/** Uso medido dos workers contra o teto da quota; ausentes sem medição ou sem teto. */
	cpu?: Meter;
	ram?: Meter;
	usageTitle?: string;
}

const SLOT_ORDER: Record<Slot, number> = { ok: 0, up: 1, bad: 2, off: 3, free: 4 };
const MAX_SLOTS = 16;

function meter(used: number, hard: number, label: string): Meter {
	return { pct: Math.min(100, Math.max(0, Math.round((100 * used) / hard))), label };
}

export function fleetCapacity(view: WorkersView, tones: Record<string, LaneTone>): FleetCapacity {
	const running = view.workers.filter((w) => w.desired === 'running');
	const cap = view.capacity;
	const out: FleetCapacity = { used: running.length, slots: [], line: '' };
	const slots: Slot[] = running.map((w) => tones[w.account] ?? 'off');
	if (cap) {
		out.fits = Math.max(0, cap.fits);
		out.total = running.length + out.fits;
		for (let i = 0; i < out.fits; i++) slots.push('free');
		out.title = `Workers no saradomin: ${coresLabel(cap.worker_cpu_cores)} de CPU e ${bytesLabel(cap.worker_memory_bytes)} por worker; o limite hoje é ${limitLabel(cap.limited_by)}`;
	}
	out.slots = slots.sort((a, b) => SLOT_ORDER[a] - SLOT_ORDER[b]).slice(0, MAX_SLOTS);

	const n = view.attached.length;
	const host = view.attached[0]?.host ?? 'zaros';
	out.line = [
		out.fits !== undefined ? fitsShort(out.fits) : '',
		n ? `${host} com ${n} ${n === 1 ? 'cliente' : 'clientes'}` : cap ? '' : 'nenhum cliente anexado'
	]
		.filter(Boolean)
		.join(' · ');

	const measured = view.workers.filter((w) => w.usage);
	if (cap && measured.length) {
		const cpu = measured.reduce((s, w) => s + (w.usage?.cpu_cores ?? 0), 0);
		const mem = measured.reduce((s, w) => s + (w.usage?.memory_bytes ?? 0), 0);
		const parts: string[] = [];
		if (cap.cpu_hard_cores) {
			out.cpu = meter(cpu, cap.cpu_hard_cores, `${cores(cpu)} / ${cores(cap.cpu_hard_cores)}`);
			parts.push(`CPU ${cores(cpu)} de ${coresLabel(cap.cpu_hard_cores)}`);
		}
		if (cap.memory_hard_bytes) {
			out.ram = meter(mem, cap.memory_hard_bytes, `${gib(mem)} / ${gib(cap.memory_hard_bytes)}`);
			parts.push(`memória ${gib(mem)} de ${gib(cap.memory_hard_bytes)} GiB`);
		}
		if (parts.length)
			out.usageTitle = `Uso medido de ${measured.length} ${measured.length === 1 ? 'worker' : 'workers'} contra o teto da quota (o que os pods podem pedir), não o uso do nó: ${parts.join(', ')}`;
	}
	return out;
}
