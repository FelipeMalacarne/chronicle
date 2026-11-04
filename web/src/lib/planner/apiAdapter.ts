/**
 * Adapter do planner sobre a API multi-guia real. Guias, plano e perna de banco vêm do
 * servidor; grafo, metas, conta e Microbot ainda não têm rota e continuam nas fixtures
 * (nenhuma tela os consome por enquanto).
 */
import { api, ApiError } from '#lib/api/client';
import { contentOnlyLeg, toBankLeg, toGuideSummary, toPlan, upcomingLeg } from './apimap.ts';
import type { ApiSection, ApiSegment } from './apimap.ts';
import { createFixtureAdapter, type PlannerAdapter } from './adapter.ts';
import type { GuideId, GuidePlan } from './types.ts';

export interface ApiAdapterOptions {
	/** Conta cuja readiness é consultada; sem ela a perna mostra só o conteúdo do guia. */
	accountId?: string;
}

interface Loaded {
	plan: GuidePlan;
	segments: ApiSegment[];
}

// Cache por guia durante a vida da página: o conteúdo é estático por versão do servidor.
const cache = new Map<GuideId, Promise<Loaded>>();

function load(guideId: GuideId): Promise<Loaded> {
	let p = cache.get(guideId);
	if (!p) {
		p = (async () => {
			const guide = await api.guideById(guideId);
			const main = guide.sections.filter((s) => !s.track);
			const sections: ApiSection[] = await Promise.all(
				main.map((s) => api.guideSection(guideId, s.id))
			);
			return {
				plan: toPlan(guide, sections),
				segments: sections.flatMap((s) => s.segments ?? [])
			};
		})();
		p.catch(() => cache.delete(guideId));
		cache.set(guideId, p);
	}
	return p;
}

export function createApiAdapter(opts: ApiAdapterOptions = {}): PlannerAdapter {
	const fixtures = createFixtureAdapter();
	return {
		...fixtures,
		async listGuides() {
			const { guides } = await api.guides();
			return guides.map(toGuideSummary);
		},
		async loadPlan(guideId) {
			return (await load(guideId)).plan;
		},
		async nextBankLeg(guideId) {
			const { plan, segments } = await load(guideId);
			const ctx = { steps: plan.sections.flatMap((s) => s.steps), segments };
			if (!opts.accountId) {
				return contentOnlyLeg(
					guideId,
					ctx,
					'Sem conta vinculada: nada foi verificado. Abra com ?account=<id> (modo dev).'
				);
			}
			try {
				const seg = upcomingLeg(await api.readiness(guideId, opts.accountId));
				return seg ? toBankLeg(seg, ctx) : null;
			} catch (e) {
				// 404 de rota: servidor sem modo dev ou sem banco de dados.
				if (e instanceof ApiError && e.status === 404 && /endpoint/.test(e.message)) {
					return contentOnlyLeg(guideId, ctx, 'Este servidor não expõe a readiness da conta.');
				}
				throw e;
			}
		}
	};
}
