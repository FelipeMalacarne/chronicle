import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import type { WorkerCapacity, WorkerRow, WorkersView } from '../api/microbot.ts';
import {
	bytesLabel,
	cores,
	coresLabel,
	fitsSentence,
	fitsShort,
	fleetCapacity,
	freeSlots,
	gib,
	limitLabel,
	roomHint,
	usageLine
} from './capacidade.ts';

// The quota read on saradomin (PR #25): 2200m / 3328Mi, 720m and 1088Mi used; a worker asks 700m, 1Gi.
const cap: WorkerCapacity = {
	fits: 2,
	limited_by: 'requests.cpu',
	cpu_hard_cores: 2.2,
	cpu_free_cores: 1.48,
	memory_hard_bytes: 3489660928,
	memory_free_bytes: 2348810240,
	worker_cpu_cores: 0.7,
	worker_memory_bytes: 1073741824,
	seen: '2026-11-13T04:13:10Z'
};

function row(account: string, p: Partial<WorkerRow> = {}): WorkerRow {
	return {
		account,
		rsn: account,
		session: null,
		desired: 'running',
		pod: null,
		agent: null,
		outdated: false,
		...p
	};
}

function view(p: Partial<WorkersView> = {}): WorkersView {
	return { enabled: true, workers: [], attached: [], ...p };
}

test('numbers in PT-BR: cores with one decimal, memory in GiB or MiB', () => {
	assert.equal(cores(3.1), '3,1');
	assert.equal(cores(4), '4');
	assert.equal(cores(0.404), '0,4');
	assert.equal(cores(0.04), '0,04');
	assert.equal(cores(0), '0');
	assert.equal(coresLabel(0.7), '0,7 núcleo');
	assert.equal(coresLabel(2.2), '2,2 núcleos');
	assert.equal(gib(3489660928), '3,3');
	assert.equal(gib(877350912), '0,8');
	assert.equal(bytesLabel(877350912), '837 MiB');
	assert.equal(bytesLabel(1073741824), '1 GiB');
	assert.equal(bytesLabel(3489660928), '3,3 GiB');
});

test('cabe mais N, with the verb agreeing, and the free places', () => {
	assert.equal(fitsShort(1), 'cabe mais 1');
	assert.equal(fitsShort(2), 'cabem mais 2');
	assert.equal(fitsShort(0), 'não cabe mais nenhum');
	assert.equal(fitsSentence(1), 'Cabe mais 1 worker no saradomin');
	assert.equal(fitsSentence(3), 'Cabem mais 3 workers no saradomin');
	assert.equal(fitsSentence(0), 'Não cabe mais nenhum worker no saradomin');
	assert.equal(freeSlots(1), '1 lugar livre');
	assert.equal(freeSlots(2), '2 lugares livres');
	assert.equal(freeSlots(0), 'sem lugar livre');
});

test('the limiting resource in words; an unknown one as it came', () => {
	assert.equal(limitLabel('requests.cpu'), 'a CPU pedida');
	assert.equal(limitLabel('memory'), 'a memória pedida');
	assert.equal(limitLabel('count/deployments.apps'), 'o número de Deployments');
	assert.equal(limitLabel('requests.nvidia.com/gpu'), 'requests.nvidia.com/gpu');
});

test('roomHint gives what a worker asks and what is left, when the quota says', () => {
	assert.equal(
		roomHint({ ...cap, fits: 0, cpu_free_cores: 0.3 }),
		'O worker pede 0,7 núcleo e sobram 0,3. Pare outra conta ou deixe esta na espera.'
	);
	assert.equal(
		roomHint({ ...cap, fits: 0, limited_by: 'requests.memory', memory_free_bytes: 536870912 }),
		'O worker pede 1 GiB e sobram 512 MiB. Pare outra conta ou deixe esta na espera.'
	);
	// limits.cpu: the free numbers are of the requests, so they say nothing here.
	assert.equal(
		roomHint({ ...cap, fits: 0, limited_by: 'limits.cpu' }),
		'O limite é o limite de CPU. Pare outra conta ou deixe esta na espera.'
	);
	assert.equal(roomHint(undefined), 'Pare outra conta ou deixe esta na espera.');
});

test('usageLine of a pod, empty without a measurement', () => {
	assert.equal(
		usageLine({ cpu_cores: 0.404, memory_bytes: 877350912, measured_at: '' }),
		'CPU 0,4 · RAM 837 MiB'
	);
	assert.equal(usageLine(undefined), '');
});

test('fleetCapacity: used and free places, the line and the meters against the quota', () => {
	const c = fleetCapacity(
		view({
			capacity: cap,
			workers: [
				row('a', { usage: { cpu_cores: 0.404, memory_bytes: 877350912, measured_at: '' } }),
				row('b'),
				row('c', { desired: 'stopped' })
			],
			attached: [{ connected: true, account: 'd', kind: 'attached', host: 'zaros' }]
		}),
		{ a: 'ok', b: 'up', c: 'off' }
	);
	assert.equal(c.used, 2);
	assert.equal(c.fits, 2);
	assert.equal(c.total, 4);
	assert.deepEqual(c.slots, ['ok', 'up', 'free', 'free']);
	assert.equal(c.line, 'cabem mais 2 · zaros com 1 cliente');
	assert.equal(
		c.title,
		'Workers no saradomin: 0,7 núcleo de CPU e 1 GiB por worker; o limite hoje é a CPU pedida'
	);
	assert.deepEqual(c.cpu, { pct: 18, label: '0,4 / 2,2' });
	assert.deepEqual(c.ram, { pct: 25, label: '0,8 / 3,3' });
	assert.match(c.usageTitle ?? '', /não o uso do nó/);
});

test('fleetCapacity without a quota: no total, no free places and no meters', () => {
	const c = fleetCapacity(
		view({
			workers: [
				row('a', { usage: { cpu_cores: 0.4, memory_bytes: 877350912, measured_at: '' } }),
				row('b')
			]
		}),
		{ a: 'bad' }
	);
	assert.equal(c.used, 2);
	assert.equal(c.total, undefined);
	assert.deepEqual(c.slots, ['bad', 'off']);
	assert.equal(c.line, 'nenhum cliente anexado');
	assert.equal(c.cpu, undefined);
	assert.equal(c.ram, undefined);
});

test('fleetCapacity: a full quota, and no meters while nothing was measured', () => {
	const c = fleetCapacity(view({ capacity: { ...cap, fits: 0 }, workers: [row('a')] }), {
		a: 'ok'
	});
	assert.equal(c.total, 1);
	assert.equal(c.line, 'não cabe mais nenhum');
	assert.equal(c.cpu, undefined);
	// A quota without a CPU ceiling (only counts): the CPU meter goes, the RAM one stays.
	const counts = fleetCapacity(
		view({
			capacity: { ...cap, cpu_hard_cores: undefined, cpu_free_cores: undefined },
			workers: [row('a', { usage: { cpu_cores: 0.4, memory_bytes: 2 ** 30, measured_at: '' } })]
		}),
		{}
	);
	assert.equal(counts.cpu, undefined);
	assert.deepEqual(counts.ram, { pct: 31, label: '1 / 3,3' });
});
