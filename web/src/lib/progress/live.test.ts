import { test } from 'node:test';
import assert from 'node:assert/strict';
import { detectionToast, parseLiveEvent, tagsFor, type LiveEvent } from './live.ts';

const progress = (over: Partial<LiveEvent> = {}): LiveEvent => ({
	type: 'progress',
	account: 'acc_x',
	source: 'plugin',
	guides: ['oziris-main'],
	steps_changed: 0,
	at: '2026-11-05T20:14:00Z',
	...over
});

test('parseLiveEvent aceita progress e sync e rejeita o resto', () => {
	assert.deepEqual(
		parseLiveEvent(
			'{"type":"progress","account":"acc_x","source":"hiscores","guides":["oziris-main","bruhsailer-main"],"steps_changed":3,"at":"2026-11-05T20:14:00Z"}'
		),
		{
			type: 'progress',
			account: 'acc_x',
			source: 'hiscores',
			guides: ['oziris-main', 'bruhsailer-main'],
			steps_changed: 3,
			status: undefined,
			at: '2026-11-05T20:14:00Z'
		}
	);
	const sync = parseLiveEvent(
		'{"type":"sync","account":"acc_x","source":"hiscores","steps_changed":0,"status":"rate_limited","at":"x"}'
	);
	assert.equal(sync?.status, 'rate_limited');
	assert.deepEqual(sync?.guides, []);
	assert.equal(parseLiveEvent('{"type":"hello","account":"acc_x","at":"x"}'), null);
	assert.equal(parseLiveEvent('{"type":"progress","account":"acc_x","source":"wom"}'), null);
	assert.equal(parseLiveEvent('não é json'), null);
	assert.equal(parseLiveEvent('null'), null);
	// steps_changed negativo ou ausente vira 0
	assert.equal(
		parseLiveEvent('{"type":"progress","account":"a","source":"plugin","steps_changed":-2}')
			?.steps_changed,
		0
	);
});

test('tagsFor separa progresso de estado da fonte', () => {
	assert.deepEqual(tagsFor(progress()), ['progress']);
	assert.deepEqual(tagsFor(progress({ type: 'sync', status: 'ok' })), ['sync']);
});

test('detectionToast só para plugin ou hiscores com passos mudados', () => {
	const at = () => '20:14';
	assert.deepEqual(detectionToast(progress({ steps_changed: 1 }), at), {
		title: '1 passo detectado',
		text: 'via plugin · 20:14'
	});
	assert.deepEqual(detectionToast(progress({ source: 'hiscores', steps_changed: 4 }), at), {
		title: '4 passos detectados',
		text: 'via hiscores · 20:14'
	});
	assert.equal(detectionToast(progress({ steps_changed: 0 }), at), null);
	assert.equal(detectionToast(progress({ source: 'owner', steps_changed: 2 }), at), null);
	assert.equal(
		detectionToast(progress({ type: 'sync', source: 'hiscores', steps_changed: 2 }), at),
		null
	);
	assert.deepEqual(detectionToast(progress({ steps_changed: 2, at: '' })), {
		title: '2 passos detectados',
		text: 'via plugin'
	});
});
