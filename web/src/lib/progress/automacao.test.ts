import { test } from 'node:test';
import assert from 'node:assert/strict';
import { auditLine, auditWho, duration } from './automacao.ts';

test('duration', () => {
	assert.equal(duration(59_000), '0 min');
	assert.equal(duration(9_173_151), '2 h 32 min');
});

test('audit lines', () => {
	const e = {
		id: 1,
		at: '',
		actor: 'dev',
		origin: 'web',
		action: 'login',
		params: { world: 303 },
		ok: true
	};
	assert.equal(auditLine(e), 'login · mundo 303 · ok');
	assert.equal(auditWho(e), 'você');
	assert.equal(
		auditLine({ ...e, action: 'client_stop', params: {}, ok: false, error: 'no agent connected' }),
		'parar cliente · falhou: no agent connected'
	);
});
