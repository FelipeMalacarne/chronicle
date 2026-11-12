import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import type { WorkerRow, WorkersView } from '../api/microbot.ts';
import { imageTag, workerLine, workerLines } from './workers.ts';

const now = Date.parse('2026-11-12T18:00:00Z');

function row(p: Partial<WorkerRow>): WorkerRow {
	return {
		account: 'acc_a',
		rsn: 'Iron A',
		session: {
			account_id: 'acc_a',
			character: 'Iron A',
			key_id: 'abcd1234',
			pushed_at: '2026-11-12T17:00:00Z',
			pushed_by: 'u'
		},
		desired: 'stopped',
		pod: null,
		agent: null,
		outdated: false,
		...p
	};
}

const on: Pick<WorkersView, 'enabled'> = { enabled: true };

test('imageTag', () => {
	assert.equal(
		imageTag('ghcr.io/felipemalacarne/chronicle-worker:87a8e21b4ad51fddce6b848853e4630da5a41133'),
		'87a8e21'
	);
	assert.equal(imageTag('ghcr.io/x/chronicle-worker:abc'), 'abc');
	assert.equal(imageTag('ghcr.io/x/chronicle-worker@sha256:0123456789abcdef'), '0123456');
	assert.equal(imageTag(''), '');
});

test('stopped account with a session can start', () => {
	const l = workerLine(row({}), on, [], now);
	assert.deepEqual(l.actions, ['start']);
	assert.equal(l.blocked, undefined);
	assert.equal(l.state, 'parado');
	assert.match(l.session, /Iron A · há 60 min/);
});

test('start is closed without a session, with workers off, or while on zaros', () => {
	assert.match(workerLine(row({ session: null }), on, [], now).blocked ?? '', /accounts connect/);
	assert.match(workerLine(row({}), { enabled: false }, [], now).blocked ?? '', /WORKERS=off/);
	const zaros = workerLine(
		row({}),
		on,
		[{ connected: true, account: 'acc_a', kind: 'attached', host: 'zaros' }],
		now
	);
	assert.match(zaros.blocked ?? '', /zaros/);
	assert.equal(zaros.where, 'zaros · anexado');
	assert.equal(zaros.cockpit, true);
});

test('running worker follows its pod and agent', () => {
	const asked = workerLine(
		row({ desired: 'running', image: 'ghcr.io/x/chronicle-worker:abc' }),
		on,
		[],
		now
	);
	assert.equal(asked.tone, 'busy');
	assert.equal(asked.where, 'saradomin · pod abc');
	const quota = workerLine(
		row({
			desired: 'running',
			pod: { deployment: 'w', state: 'blocked', reason: 'exceeded quota', seen: '' }
		}),
		on,
		[],
		now
	);
	assert.equal(quota.tone, 'warn');
	assert.equal(quota.detail, 'exceeded quota');
	const up = workerLine(
		row({
			desired: 'running',
			pod: { deployment: 'w', state: 'running', image: 'i:abc', seen: '' },
			agent: { connected: true, kind: 'managed' }
		}),
		on,
		[],
		now
	);
	assert.equal(up.tone, 'ok');
	assert.equal(up.cockpit, true);
	assert.deepEqual(up.actions, ['stop', 'restart']);
	const failed = workerLine(
		row({
			desired: 'running',
			pod: { deployment: 'w', state: 'running', seen: '' },
			last_error: 'bootstrap: session'
		}),
		on,
		[],
		now
	);
	assert.equal(failed.detail, 'bootstrap: session');
	const linkedButFailed = workerLine(
		row({
			desired: 'running',
			pod: { deployment: 'w', state: 'running', seen: '' },
			agent: { connected: true, kind: 'managed' },
			last_error: 'bootstrap: worker_bootstrap: mb-client start --headless: exit status 1'
		}),
		on,
		[],
		now
	);
	assert.equal(linkedButFailed.state, 'agent ligado · o cliente não subiu');
	assert.equal(linkedButFailed.tone, 'warn');
});

test('an outdated worker offers the restart first', () => {
	const l = workerLine(
		row({
			desired: 'running',
			outdated: true,
			pod: { deployment: 'w', state: 'running', seen: '' }
		}),
		on,
		[],
		now
	);
	assert.deepEqual(l.actions, ['restart', 'stop']);
	assert.equal(l.outdated, true);
});

test('workerLines of nothing', () => {
	assert.deepEqual(workerLines(null), []);
});

test('with WORKERS=off a linked worker shows its agent and offers no lifecycle', () => {
	const l = workerLine(
		row({ desired: 'running', agent: { connected: true, kind: 'managed' } }),
		{ enabled: false },
		[],
		now
	);
	assert.equal(l.state, 'agent ligado');
	assert.equal(l.tone, 'ok');
	assert.deepEqual(l.actions, []);
	assert.equal(l.cockpit, true);
});
