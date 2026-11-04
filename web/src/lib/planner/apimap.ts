/**
 * Tradução do contrato HTTP multi-guia (Go) para o view-model do planner. Puro: sem
 * fetch nem runes, para ser testado com `node --test`. Tipos espelham
 * docs/arquitetura/types.ts (seção de readiness) e os handlers de internal/httpapi.
 */
import { stepKey } from './compare.ts';
import type {
	BankLeg,
	BankStatus,
	GuidePlan,
	GuideSummary,
	LoadoutItem,
	SectionVM,
	StepVM,
	Verdict
} from './types.ts';

/* ------------------------------------------------------------------ contrato HTTP */

export interface ApiGuide {
	id: string;
	slug?: string;
	title: string;
	version?: string;
	source?: { author?: string; url?: string };
	steps: number;
	sections: {
		id: string;
		slug: string;
		title: string;
		kind: string;
		track: boolean;
		steps: number;
	}[];
}

export interface ApiEntityMention {
	ref: string;
	source_text: string;
	method: 'rule' | 'item' | 'link' | 'text';
}

export interface ApiStep {
	id: string;
	text: string;
	location?: string;
	entities?: ApiEntityMention[];
}

export interface ApiLoadoutItem {
	name: string;
	item_id?: number;
	qty?: number;
	optional?: boolean;
}

export interface ApiSegment {
	id: string;
	kind: 'bank_leg' | 'sequence';
	title?: string;
	bank_actions?: { type: string; position: string; text: string }[];
	loadout?: {
		status: 'known' | 'partial' | 'unknown';
		inventory?: ApiLoadoutItem[];
		equipment?: ApiLoadoutItem[];
	};
	steps: ApiStep[];
}

export interface ApiSection {
	id: string;
	title: string;
	kind?: string;
	segments?: ApiSegment[];
	steps: ApiStep[];
}

type ContainerState = 'unavailable' | 'restored' | 'live';

export interface ApiOwnership {
	verdict: Verdict;
	stale?: boolean;
	containers: Partial<Record<string, { state?: ContainerState; observed: boolean; qty: number }>>;
}

export interface ApiSegmentReadiness {
	guide_id: string;
	segment_id: string;
	kind: 'bank_leg' | 'sequence';
	title?: string;
	first_step_id?: string;
	status: 'ready' | 'listed_ready' | 'not_ready' | 'unknown' | 'no_loadout';
	loadout_status: 'known' | 'partial' | 'unknown';
	items: {
		name: string;
		slot: 'inventory' | 'equipment';
		required: number;
		qty_stated: boolean;
		optional?: boolean;
		verdict: Verdict;
		owned?: ApiOwnership;
	}[];
	bank_actions: { type: string; text: string }[];
	freshness: Record<
		string,
		{ state?: ContainerState; observed: boolean; live: boolean; observed_at?: string }
	>;
	reason?: string;
}

export interface ApiReadiness {
	guide: string;
	current: ApiSegmentReadiness | null;
	next: ApiSegmentReadiness | null;
}

/* ------------------------------------------------------------------ guias e plano */

const SHORT_LABELS: [prefix: string, label: string][] = [
	['oziris', 'Oziris'],
	['bruhsailer', 'BRUHsailer']
];

export function toGuideSummary(g: ApiGuide): GuideSummary {
	const label = SHORT_LABELS.find(([p]) => g.id.startsWith(p))?.[1] ?? g.title;
	return { id: g.id, label, title: g.title, author: g.source?.author ?? '', steps: g.steps };
}

/**
 * Marcos compartilháveis: uma quest concluída. Por guia, cada quest marca um único passo:
 * o da regra de conclusão (`method: rule`) quando existe; senão, a última menção à quest,
 * que costuma ser a conclusão. O alinhamento entre guias fica com `sharedMilestones` (LCS).
 */
export function milestonesOf(
	steps: readonly ApiStep[]
): Map<string, { label: string; stepId: string }> {
	const byRule = new Map<string, { label: string; stepId: string }>();
	const byLast = new Map<string, { label: string; stepId: string }>();
	for (const st of steps) {
		for (const m of st.entities ?? []) {
			if (!m.ref.startsWith('quest:')) continue;
			const hit = { label: m.source_text, stepId: st.id };
			if (m.method === 'rule' && !byRule.has(m.ref)) byRule.set(m.ref, hit);
			byLast.set(m.ref, hit);
		}
	}
	const out = new Map(byLast);
	for (const [ref, hit] of byRule) out.set(ref, hit);
	return out;
}

/** Monta o plano a partir das seções completas do guia (ordem do guia, tracks fora). */
export function toPlan(guide: ApiGuide, sections: readonly ApiSection[]): GuidePlan {
	const main = sections.filter((s) => s.kind !== 'track');
	const all = main.flatMap((s) => s.steps);
	const milestoneAt = new Map<string, { ref: string; label: string }>();
	// Um passo carrega um marco só: quando conclui várias quests, vale a primeira citada.
	for (const [ref, hit] of milestonesOf(all)) {
		if (!milestoneAt.has(hit.stepId)) milestoneAt.set(hit.stepId, { ref, label: hit.label });
	}
	const bankAt = new Set(
		main.flatMap((s) =>
			(s.segments ?? []).filter((g) => g.kind === 'bank_leg').map((g) => g.steps[0]?.id)
		)
	);
	const vm: SectionVM[] = main.map((s) => ({
		key: `${guide.id}::sec:${s.id}`,
		guideId: guide.id,
		id: s.id,
		title: s.title,
		steps: s.steps.map((st): StepVM => {
			const ms = milestoneAt.get(st.id);
			return {
				key: stepKey(guide.id, st.id),
				guideId: guide.id,
				localId: st.id,
				text: st.text,
				location: st.location,
				bank: bankAt.has(st.id) || undefined,
				milestone: ms?.ref,
				milestoneLabel: ms?.label
			};
		})
	}));
	const summary = toGuideSummary(guide);
	// Sem API de progresso ainda: nada é dado como feito.
	return {
		guide: { ...summary, steps: vm.reduce((n, s) => n + s.steps.length, 0) },
		sections: vm,
		doneCount: 0
	};
}

/* ------------------------------------------------------------------ perna de banco */

/** Segmento de banco que o jogador encontra a partir do início da rota. */
export function upcomingLeg(r: ApiReadiness): ApiSegmentReadiness | null {
	if (r.current?.kind === 'bank_leg') return r.current;
	return r.next;
}

function bankStatus(f: ApiSegmentReadiness['freshness'][string] | undefined): BankStatus {
	if (!f?.observed || !f.state || f.state === 'unavailable')
		return { freshness: 'unavailable', capturedAt: null };
	return { freshness: f.state, capturedAt: f.observed_at ? Date.parse(f.observed_at) : null };
}

const READINESS_DETAIL: Record<ApiSegmentReadiness['status'], string> = {
	ready: 'Inventário e equipamento ao vivo batem com o setup do guia.',
	listed_ready: 'Os itens listados estão com você; o guia lista o setup só em parte.',
	not_ready: 'Falta algum item no inventário ou equipamento ao vivo.',
	unknown: 'Não dá para verificar: inventário/equipamento sem leitura ao vivo.',
	no_loadout: 'O guia não lista o setup deste trecho.'
};

interface LegContext {
	steps: readonly StepVM[];
	segments: readonly ApiSegment[];
}

function legShape(
	guideId: string,
	seg: ApiSegment | undefined,
	firstStepId: string,
	ctx: LegContext
) {
	const at = ctx.steps.find((s) => s.localId === firstStepId);
	return {
		guideId,
		atStepKey: at?.key ?? stepKey(guideId, firstStepId),
		atStepId: firstStepId,
		// O título do segmento é rótulo interno da derivação; não serve como local.
		location: at?.location ?? 'Banco',
		stepsUntilNextBank: Math.max(0, (seg?.steps.length ?? 1) - 1)
	};
}

/** Perna vinda da readiness do servidor: posse no banco, item carregado e frescor. */
export function toBankLeg(seg: ApiSegmentReadiness, ctx: LegContext): BankLeg {
	const content = ctx.segments.find((s) => s.id === seg.segment_id);
	const bank = bankStatus(seg.freshness.bank);
	const withdraw = seg.items.map((it): LoadoutItem => {
		const inBank = it.owned?.containers.bank;
		const readable = inBank?.observed && (inBank.state === 'live' || inBank.state === 'restored');
		return {
			name: it.name,
			qty: it.required,
			inBank: readable ? inBank.qty : null,
			qtyStated: it.qty_stated,
			optional: it.optional,
			carried: it.verdict,
			ownedStale: it.owned?.stale
		};
	});
	return {
		...legShape(seg.guide_id, content, seg.first_step_id ?? content?.steps[0]?.id ?? '', ctx),
		withdraw,
		deposit: depositsOf(seg.bank_actions),
		bank,
		readiness: {
			status: seg.status,
			loadout: seg.loadout_status,
			detail: READINESS_DETAIL[seg.status]
		}
	};
}

/**
 * Perna só com o conteúdo do guia, quando não há conta vinculada ou a rota de readiness
 * não existe neste servidor: nada é verificado e o banco aparece como indisponível.
 */
export function contentOnlyLeg(guideId: string, ctx: LegContext, detail: string): BankLeg | null {
	const seg = ctx.segments.find((s) => s.kind === 'bank_leg');
	if (!seg || !seg.steps.length) return null;
	const lo = seg.loadout;
	const items = [...(lo?.equipment ?? []), ...(lo?.inventory ?? [])];
	return {
		...legShape(guideId, seg, seg.steps[0].id, ctx),
		withdraw: items.map((it) => ({
			name: it.name,
			qty: it.qty ?? 1,
			inBank: null,
			qtyStated: it.qty !== undefined,
			optional: it.optional
		})),
		deposit: depositsOf(seg.bank_actions ?? []),
		bank: { freshness: 'unavailable', capturedAt: null },
		readiness: {
			status: lo && items.length ? 'unknown' : 'no_loadout',
			loadout: lo?.status ?? 'unknown',
			detail
		}
	};
}

function depositsOf(actions: readonly { type: string; text: string }[]): string[] {
	return actions.filter((a) => a.type === 'deposit' || a.type === 'deposit_all').map((a) => a.text);
}
