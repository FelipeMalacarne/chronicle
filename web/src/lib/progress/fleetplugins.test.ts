import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
	accountProfileName,
	outdated,
	pluginTitle,
	settingsOf,
	statusText,
	update,
	withPlugin,
	withSetting,
	withoutPlugin,
	type FleetPlugin
} from './fleetplugins.ts';

const plugin = (over: Partial<FleetPlugin>): FleetPlugin => ({
	source: 'hub',
	name: 'TickFisherPlugin',
	group: 'plays',
	in_model: false,
	accounts: [],
	...over
});

test('the right column says where it runs and where it is', () => {
	const p = plugin({
		accounts: [
			{ account: 'a', in_profile: true, installed: '1.4.0', active: true },
			{ account: 'b', in_profile: true, installed: '1.4.0', active: false },
			{ account: 'c', in_profile: false, installed: '1.4.0' },
			{ account: 'd', in_profile: false }
		]
	});
	assert.equal(statusText(p, 4), 'rodando em 1 de 3');
	const stopped = p.accounts.map((s) => (s.active === undefined ? s : { ...s, active: false }));
	assert.equal(statusText({ ...p, accounts: stopped }, 4), 'instalado em 3');
	assert.equal(statusText({ ...p, group: 'always' }, 5), 'em 1 de 5');
	assert.equal(
		statusText(
			plugin({
				source: 'own',
				name: 'a.CatSwapPlugin',
				accounts: [{ account: 'a', in_profile: true }]
			}),
			1
		),
		'seu · instalado em 1'
	);
	assert.equal(statusText(plugin({ in_model: true }), 2), 'no padrão da frota');
	assert.equal(statusText(plugin({}), 2), 'em nenhuma conta');
});

test('a newer Hub version is offered to the accounts that lack it', () => {
	const p = plugin({
		latest: '1.4.2',
		accounts: [
			{ account: 'a', in_profile: true, installed: '1.4.0' },
			{ account: 'b', in_profile: true, installed: '1.4.2' },
			{ account: 'c', in_profile: false }
		]
	});
	assert.equal(update(p), '1.4.2');
	assert.deepEqual(outdated(p), ['a']);
	assert.equal(
		update({ ...p, accounts: [{ account: 'b', in_profile: true, installed: '1.4.2' }] }),
		null
	);
	assert.equal(update({ ...p, latest: undefined }), null);
});

test('titles', () => {
	assert.equal(pluginTitle(plugin({ title: 'Tick Fisher' })), 'Tick Fisher');
	assert.equal(pluginTitle(plugin({ name: 'GotrPlugin' })), 'Gotr');
	assert.equal(
		pluginTitle(
			plugin({
				source: 'own',
				name: 'net.runelite.client.plugins.microbot.fm.catswap.CatSwapPlugin'
			})
		),
		'Cat Swap'
	);
});

test('profile layer edits', () => {
	const layer = { parent: 'frota', plugins: [], settings: [{ group: 'g', key: 'k', value: '1' }] };
	const one = withPlugin(layer, { source: 'hub', name: 'X', start: false });
	assert.equal(one.plugins.length, 1);
	const pinned = withPlugin(one, { source: 'hub', name: 'X', version: '2', start: false });
	assert.deepEqual(pinned.plugins, [{ source: 'hub', name: 'X', version: '2', start: false }]);
	assert.equal(withoutPlugin(pinned, 'hub:X').plugins.length, 0);
	assert.deepEqual(withSetting(layer, 'g', 'k', '2').settings, [
		{ group: 'g', key: 'k', value: '2' }
	]);
	assert.deepEqual(withSetting(layer, 'g', 'k', null).settings, []);
	assert.equal(layer.settings.length, 1, 'the input is not changed');
	assert.deepEqual(settingsOf(layer.settings, 'g', ['k', 'other']), { k: '1', other: null });
});

test('account profile names', () => {
	assert.equal(accountProfileName('acc_EventSourced'), 'acc_eventsourced');
	assert.equal(accountProfileName('acc felipe.pesca'), 'acc-felipe-pesca');
	assert.equal(accountProfileName('__'), 'conta');
});
