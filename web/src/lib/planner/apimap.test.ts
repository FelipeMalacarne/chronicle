import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
	contentOnlyLeg,
	milestonesOf,
	toBankLeg,
	toGuideSummary,
	toPlan,
	upcomingLeg,
	type ApiGuide,
	type ApiSection,
	type ApiSegmentReadiness
} from './apimap.ts';
import { buildBands, flattenSteps, sharedMilestones } from './compare.ts';

const guide = (id: string): ApiGuide => ({
	id,
	title: id === 'oziris-main' ? 'OSRS Ironman Efficiency Guide' : 'BRUHsailer Ironman Guide',
	source: { author: id === 'oziris-main' ? 'OzirisLoL' : 'So Iron BRUH and ParasailerOSRS' },
	steps: 4,
	sections: [
		{ id: 's1', slug: 's1', title: 'Início', kind: 'main', track: false, steps: 3 },
		{ id: 't1', slug: 't1', title: 'Track', kind: 'track', track: true, steps: 1 }
	]
});

// Mesmos ids locais nos dois guias; só as menções a quests diferem.
const sections = (questMethod: 'rule' | 'text'): ApiSection[] => {
	const steps = [
		{
			id: '1',
			text: 'Start Cook’s Assistant',
			entities: [
				{ ref: 'quest:cooks_assistant', source_text: 'Cook’s Assistant', method: 'text' as const }
			]
		},
		{ id: '2', text: 'Bank', location: 'Draynor' },
		{
			id: '3',
			text: 'Finish Cook’s Assistant, then Dragon Slayer I',
			entities: [
				{ ref: 'quest:cooks_assistant', source_text: 'Cook’s Assistant', method: questMethod },
				{ ref: 'quest:dragon_slayer_i', source_text: 'Dragon Slayer I', method: questMethod }
			]
		}
	];
	return [
		{
			id: 's1',
			title: 'Início',
			kind: 'main',
			steps,
			segments: [
				{ id: 'seg-a', kind: 'sequence', steps: steps.slice(0, 1) },
				{
					id: 'seg-b',
					kind: 'bank_leg',
					bank_actions: [
						{ type: 'deposit_all', position: 'start', text: 'deposit all your items' }
					],
					loadout: {
						status: 'partial',
						inventory: [{ name: 'Hammer', item_id: 2347 }, { name: '1k arrow shafts' }]
					},
					steps: steps.slice(1)
				}
			]
		},
		{ id: 't1', title: 'Track', kind: 'track', steps: [{ id: '9', text: 'Track step' }] }
	];
};

test('guide summary keeps attribution and a short label', () => {
	const s = toGuideSummary(guide('bruhsailer-main'));
	assert.equal(s.label, 'BRUHsailer');
	assert.equal(s.author, 'So Iron BRUH and ParasailerOSRS');
});

test('milestones prefer the completion rule, else the last mention', () => {
	const ruled = milestonesOf(sections('rule')[0].steps);
	assert.equal(ruled.get('quest:cooks_assistant')?.stepId, '3');
	const text = milestonesOf([...sections('text')[0].steps].reverse());
	assert.equal(text.get('quest:cooks_assistant')?.stepId, '1'); // última menção na ordem dada
});

test('plans of two guides never share step keys and align on shared quests', () => {
	const oz = toPlan(guide('oziris-main'), sections('rule'));
	const bs = toPlan(guide('bruhsailer-main'), sections('text'));
	const a = flattenSteps(oz.sections);
	const b = flattenSteps(bs.sections);
	assert.equal(a.length, 3, 'tracks are left out of the main route');
	const keys = new Set([...a, ...b].map((s) => s.key));
	assert.equal(keys.size, a.length + b.length);
	// O passo 3 conclui duas quests; o marco é a primeira citada, igual nos dois guias.
	assert.deepEqual(sharedMilestones(a, b), ['quest:cooks_assistant']);
	assert.ok(buildBands(a, b).length >= 2);
	assert.equal(a.find((s) => s.localId === '2')?.bank, true);
	assert.equal(oz.doneCount, 0, 'no progress API yet: nothing is done');
});

const readiness = (bank: 'restored' | 'live'): ApiSegmentReadiness => ({
	guide_id: 'bruhsailer-main',
	segment_id: 'seg-b',
	kind: 'bank_leg',
	first_step_id: '2',
	status: 'not_ready',
	loadout_status: 'partial',
	items: [
		{
			name: 'Hammer',
			slot: 'inventory',
			required: 1,
			qty_stated: false,
			verdict: 'unmet',
			owned: {
				verdict: 'met',
				stale: bank === 'restored',
				containers: { bank: { state: bank, observed: true, qty: 1 } }
			}
		}
	],
	bank_actions: [{ type: 'deposit_all', text: 'deposit all your items' }],
	freshness: {
		bank: {
			state: bank,
			observed: true,
			live: bank === 'live',
			observed_at: '2026-11-04T09:00:00Z'
		},
		inventory: { state: 'live', observed: true, live: true },
		equipment: { state: 'live', observed: true, live: true }
	}
});

test('restored bank stays visibly stale and readiness is the server verdict', () => {
	const plan = toPlan(guide('bruhsailer-main'), sections('text'));
	const ctx = { steps: flattenSteps(plan.sections), segments: sections('text')[0].segments ?? [] };
	const leg = toBankLeg(readiness('restored'), ctx);
	assert.equal(leg.bank.freshness, 'restored');
	assert.equal(leg.bank.capturedAt, Date.parse('2026-11-04T09:00:00Z'));
	assert.equal(leg.atStepKey, 'bruhsailer-main::2');
	assert.equal(leg.location, 'Draynor');
	assert.equal(leg.stepsUntilNextBank, 1);
	assert.equal(leg.readiness?.status, 'not_ready');
	assert.equal(leg.withdraw[0].inBank, 1);
	assert.equal(leg.withdraw[0].ownedStale, true);
	assert.equal(leg.withdraw[0].qtyStated, false);
	assert.deepEqual(leg.deposit, ['deposit all your items']);
});

test('upcoming leg is the current segment only when it is a bank leg', () => {
	const r = readiness('live');
	assert.equal(upcomingLeg({ guide: 'g', current: r, next: null }), r);
	const seq = { ...r, kind: 'sequence' as const, segment_id: 'seg-a' };
	assert.equal(upcomingLeg({ guide: 'g', current: seq, next: r }), r);
	assert.equal(upcomingLeg({ guide: 'g', current: seq, next: null }), null);
});

test('content-only leg verifies nothing', () => {
	const plan = toPlan(guide('oziris-main'), sections('rule'));
	const ctx = { steps: flattenSteps(plan.sections), segments: sections('rule')[0].segments ?? [] };
	const leg = contentOnlyLeg('oziris-main', ctx, 'sem conta');
	assert.ok(leg);
	assert.equal(leg.bank.freshness, 'unavailable');
	assert.equal(leg.readiness?.status, 'unknown');
	assert.ok(leg.withdraw.every((i) => i.inBank === null && i.carried === undefined));
	assert.equal(contentOnlyLeg('oziris-main', { steps: [], segments: [] }, 'x'), null);
});
