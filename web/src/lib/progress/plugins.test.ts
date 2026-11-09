import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	cleanName,
	detailFacts,
	fieldError,
	formSections,
	pluginRows,
	rowLine,
	rowVersion
} from './plugins.ts';
import { auditLine } from './automacao.ts';
import type { PluginSchema, PluginsView } from '../api/microbot.ts';

const view: PluginsView = {
	scripts: {
		count: 5,
		scripts: [
			{
				name: '<html>[<font color=#b8f704M>M</font>] GuardiansOfTheRift',
				className: 'net.runelite.client.plugins.microbot.gotr.GotrPlugin',
				active: false,
				enabled: false
			},
			{
				name: '<html>[<font color=#ffff1a>🦆</font>]  Kitten Tracker',
				className: 'net.runelite.client.plugins.microbot.kittentracker.KittenPlugin',
				active: true,
				enabled: true
			},
			{
				name: '<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] Ironman Planner',
				className: 'net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin',
				active: true,
				enabled: true
			},
			{
				name: '<html>[<b><font color=#FF8C00>F</font><font color=#1E90FF>M</font></b>] Tick Fisher',
				className: 'net.runelite.client.plugins.microbot.fm.tickfisher.TickFisherPlugin',
				active: false,
				enabled: false
			},
			{
				name: '<html>[<font color=#b8f704M>M</font>] Agent Server',
				className: 'net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin',
				active: true,
				enabled: true
			}
		]
	},
	installed: {
		count: 2,
		total: 2,
		plugins: [
			{
				internalName: 'GotrPlugin',
				name: 'GuardiansOfTheRift',
				latestVersion: '1.5.15',
				installedVersion: '1.5.14',
				loaded: true
			},
			{
				internalName: 'KittenPlugin',
				name: ' Kitten Tracker',
				latestVersion: '1.3.1',
				installedVersion: '1.3.1',
				loaded: true
			}
		]
	}
};

test('cleanName drops the HTML tag prefix', () => {
	assert.equal(cleanName(view.scripts.scripts[3].name), 'Tick Fisher');
	assert.equal(cleanName("<html>[<font color=#9900ff>B</font>] Bank's Shopper"), "Bank's Shopper");
	assert.equal(cleanName('Quest Helper'), 'Quest Helper');
});

test('pluginRows joins the Hub by class name and orders hub, mine, client', () => {
	const rows = pluginRows(view);
	assert.deepEqual(
		rows.map((r) => [r.name, r.source, r.tone, r.protected]),
		[
			['Kitten Tracker', 'hub', 'ok', false],
			['GuardiansOfTheRift', 'hub', 'warn', false],
			['Ironman Planner', 'meus', 'ok', true],
			['Tick Fisher', 'meus', 'off', false],
			['Agent Server', 'cliente', 'ok', true]
		]
	);
	const gotr = rows[1];
	assert.equal(rowVersion(gotr), '1.5.14 → 1.5.15');
	assert.equal(rowLine(gotr), 'parado · atualização disponível no Hub');
	assert.equal(rowLine(rows[2]), 'tracking · somente leitura · não é controlado daqui');
	assert.equal(rowVersion(rows[3]), 'seu · sideload');
	assert.deepEqual(pluginRows(null), []);
});

test('detailFacts reads status, health and the last result', () => {
	assert.deepEqual(
		detailFacts({
			status: { status: 'RUNNING', active: true, runtimeMs: 185000 },
			health: { status: 'OK', loopCount: 42, lastError: { message: 'NPE', recentCount: 2 } },
			results: { count: 1, results: [{ passed: true, houses: 4, timestamp: 'x' }] }
		}),
		[
			{ k: 'Status', v: 'rodando · há 3 min' },
			{ k: 'Saúde', v: 'ok · 42 loops · erro: NPE (2×)' },
			{ k: 'Último resultado', v: 'passed=true · houses=4' }
		]
	);
	assert.deepEqual(
		detailFacts({ errors: { health: 'HTTP 404' } }).map((f) => f.v),
		['sem script de loop', 'nenhum']
	);
});

test('auditLine names the plugin actions', () => {
	const base = { id: 1, at: '', actor: 'dev', origin: 'web', ok: true };
	assert.equal(
		auditLine({ ...base, action: 'script_start', params: { className: 'a.b.GotrPlugin' } }),
		'iniciar plugin · GotrPlugin · ok'
	);
	assert.equal(
		auditLine({
			...base,
			action: 'config_set',
			params: { group: 'qol', key: 'speed', value: null }
		}),
		'configuração · qol.speed = (padrão) · ok'
	);
	assert.equal(
		auditLine({
			...base,
			action: 'plugin_install',
			params: { name: 'GotrPlugin' },
			ok: false,
			error: '409'
		}),
		'instalar plugin · GotrPlugin · falhou: 409'
	);
});

test('formSections groups visible options by section, loose ones first', () => {
	const schema: PluginSchema = {
		className: 'a.B',
		name: 'B',
		group: 'b',
		sections: [
			{ key: 'adv', name: 'Avançado', position: 1, closedByDefault: true },
			{ key: 'empty', name: 'Vazia', position: 2 }
		],
		items: [
			{ key: 'speed', name: 'Speed', position: 0, type: 'int', min: 1, max: 10 },
			{ key: 'debug', name: 'Debug', position: 1, type: 'boolean', section: 'adv' },
			{ key: 'secretish', name: 'x', position: 2, type: 'string', hidden: true }
		]
	};
	assert.deepEqual(
		formSections(schema).map((s) => [s.name, s.closed, s.items.map((i) => i.key)]),
		[
			['Geral', false, ['speed']],
			['Avançado', true, ['debug']]
		]
	);
	const speed = schema.items[0];
	assert.equal(fieldError(speed, '5'), null);
	assert.equal(fieldError(speed, '11'), 'máximo 10');
	assert.equal(fieldError(speed, '2.5'), 'inteiro');
	assert.equal(
		fieldError({ ...speed, type: 'enum', values: [{ value: 'A', label: 'a' }] }, 'B'),
		'escolha um valor'
	);
});
