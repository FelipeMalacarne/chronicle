import { test } from 'node:test';
import assert from 'node:assert/strict';
import { auditLine, auditWho, cards, duration, stateRows } from './automacao.ts';
import type { MicrobotStatus } from '../api/microbot.ts';

const running: MicrobotStatus = {
	enabled: true,
	dev: true,
	agent: { connected: true, host: 'zaros', version: 'dev' },
	client: {
		running: true,
		headless: true,
		vnc: true,
		state: {
			loggedIn: true,
			gameState: 'LOGGED_IN',
			player: {
				name: 'EventSourced',
				combatLevel: 40,
				moving: true,
				position: { x: 1678, y: 2966, plane: 0 }
			},
			scriptsPaused: false,
			instance: { uptimeMs: 9_173_151 }
		},
		login: { currentWorld: 693 }
	}
};

test('cards say client, account and agent', () => {
	assert.deepEqual(
		cards(running).map((c) => `${c.k}: ${c.v} (${c.tone})`),
		[
			'Cliente: rodando · headless Xvfb (ok)',
			'Conta: logado · EventSourced · mundo 693 (ok)',
			'Agent: conectado · zaros (ok)'
		]
	);
	const noAgent: MicrobotStatus = { enabled: true, dev: true, agent: { connected: false } };
	assert.equal(cards(noAgent)[2].tone, 'off');
	assert.equal(cards(noAgent)[0].v, 'sem agent');
	const stopped: MicrobotStatus = {
		...running,
		client: { running: false, headless: false, vnc: false }
	};
	assert.equal(cards(stopped)[0].v, 'parado');
	assert.equal(cards(stopped)[1].v, '—');
	const loginError: MicrobotStatus = {
		...running,
		client: {
			...running.client!,
			state: { loggedIn: false },
			login: { loginError: 'Account is banned' }
		}
	};
	assert.equal(cards(loginError)[1].v, 'não logado · Account is banned');
	assert.equal(cards(null).length, 0);
});

test('stateRows reads /state', () => {
	assert.deepEqual(stateRows(running), [
		{ k: 'Posição', v: '1678, 2966, 0' },
		{ k: 'Combat', v: '40' },
		{ k: 'Agora', v: 'andando' },
		{ k: 'Scripts', v: 'livres' },
		{ k: 'Estado do jogo', v: 'LOGGED_IN' },
		{ k: 'Cliente no ar', v: '2 h 32 min' }
	]);
	assert.equal(duration(59_000), '0 min');
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
