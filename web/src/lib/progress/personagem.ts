/*
 * Conta › Personagem (página 8 do canvas, V7Personagem1440): o resumo, as quests, os diaries, o que
 * ela leva agora, as conquistas e o banco, a partir do estado da conta (GET /api/account/state).
 * Sem leitura, a célula ou a linha some; nunca vira ausência (regra 8). Puro, testado em node.
 */
import { CA_TIER_LABEL, caMismatch, caTotals, combatLevel } from './conta.ts';
import { PLAYING_MS } from './frota.ts';
import type {
	AccountState,
	CATierView,
	CollectionLogView,
	ContainerView,
	DiaryView,
	FactSource,
	QuestView
} from './types.ts';
import { ago, STALE_SOURCE_MS } from './view.ts';

const n = (v: number) => v.toLocaleString('pt-BR');

export interface SumCell {
	k: string;
	v: string;
	/** "de 217 · 132 QP": o total, em letra menor. */
	of?: string;
}

/** O resumo do topo: total, combat, quests, diaries, combat achievements e collection log. */
export function summaryCells(st: AccountState): SumCell[] {
	const out: SumCell[] = [];
	if (st.total_level) out.push({ k: 'Total', v: n(st.total_level) });
	const combat = combatLevel(st.skills);
	if (combat !== null) out.push({ k: 'Combat', v: n(combat) });
	const qp = st.quest_points !== undefined ? `${n(st.quest_points)} QP` : '';
	if (st.quests.length) {
		const done = st.quests.filter((q) => q.state === 'FINISHED').length;
		out.push({
			k: 'Quests',
			v: n(done),
			of: [`de ${n(st.quests.length)}`, qp].filter(Boolean).join(' · ')
		});
	} else if (qp) out.push({ k: 'Quest points', v: n(st.quest_points!) });
	if (st.diaries.length) {
		const done = st.diaries.filter((d) => d.complete).length;
		out.push({ k: 'Diaries', v: n(done), of: `de ${n(st.diaries.length)}` });
	}
	if (st.combat_achievements.length) {
		const t = caTotals(st.combat_achievements);
		out.push({ k: 'Combat achievements', v: n(t.done), of: `de ${n(t.total)}` });
	}
	const clog = st.collection_log;
	if (clog)
		out.push({
			k: 'Collection log',
			v: n(clog.obtained),
			of: clog.total ? `de ${n(clog.total)}` : undefined
		});
	return out;
}

export interface Freshness {
	/** ok: o plugin acabou de mandar; rest: a última leitura; off: nenhuma. */
	lamp: 'ok' | 'rest' | 'off';
	text: string;
}

function newest<T extends { observed_at: string }>(xs: T[]): T | undefined {
	return xs.reduce<T | undefined>(
		(m, x) => (!m || x.observed_at > m.observed_at ? x : m),
		undefined
	);
}

/** "plugin · há 12 s": a fonte da leitura mais nova, com a lâmpada acesa se ela é de agora. */
export function factSource(xs: FactSource[], now: number): Freshness {
	const top = newest(xs);
	if (!top) return { lamp: 'off', text: 'sem leitura' };
	const at = Date.parse(top.observed_at);
	return {
		lamp: now - at < PLAYING_MS ? 'ok' : 'rest',
		text: `${top.source} · ${ago(at, now)}`
	};
}

export type QuestFilter = 'active' | 'done' | 'missing';

const IN: Record<QuestFilter, QuestView['state']> = {
	active: 'IN_PROGRESS',
	done: 'FINISHED',
	missing: 'NOT_STARTED'
};

export function questCounts(quests: QuestView[]): Record<QuestFilter, number> {
	const c = (f: QuestFilter) => quests.filter((q) => q.state === IN[f]).length;
	return { active: c('active'), done: c('done'), missing: c('missing') };
}

/** As quests do filtro, a mais recente primeiro; com busca, só as que têm o texto no nome. */
export function questRows(quests: QuestView[], filter: QuestFilter, query = ''): QuestView[] {
	const q = query.trim().toLowerCase();
	return quests
		.filter((x) => x.state === IN[filter] && (!q || x.name.toLowerCase().includes(q)))
		.sort((a, b) =>
			a.observed_at === b.observed_at
				? a.name.localeCompare(b.name)
				: a.observed_at < b.observed_at
					? 1
					: -1
		);
}

/** "mais 3 em andamento": o rótulo do link que abre o resto do filtro. */
export const QUEST_MORE: Record<QuestFilter, string> = {
	active: 'em andamento',
	done: 'feitas',
	missing: 'que faltam'
};

export const DIARY_TIERS = ['easy', 'medium', 'hard', 'elite'] as const;
export type DiaryTier = (typeof DIARY_TIERS)[number];

export interface DiaryRow {
	area: string;
	name: string;
	/** Uma bolinha por tier: on (completo), off (falta) e unk (sem leitura). */
	tiers: { tier: DiaryTier; dot: 'on' | 'off' | 'unk' }[];
	done: number;
	/** O primeiro tier que falta (ou o elite): o que o painel da entidade abre. */
	next: DiaryTier;
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** "lumbridge_draynor" vira "Lumbridge Draynor"; "kourend_and_kebos", "Kourend & Kebos". */
export function areaName(area: string): string {
	return area.split('_').map(cap).join(' ').replace(/ And /g, ' & ');
}

/** Uma linha por área, as começadas primeiro, depois pelo nome. */
export function diaryRows(diaries: DiaryView[]): DiaryRow[] {
	const by = new Map<string, DiaryView[]>();
	for (const d of diaries) by.set(d.area, [...(by.get(d.area) ?? []), d]);
	return [...by]
		.map(([area, ds]) => {
			const tiers = DIARY_TIERS.map((tier) => {
				const d = ds.find((x) => x.tier === tier);
				return {
					tier,
					dot: !d ? ('unk' as const) : d.complete ? ('on' as const) : ('off' as const)
				};
			});
			return {
				area,
				name: areaName(area),
				tiers,
				done: tiers.filter((t) => t.dot === 'on').length,
				next: tiers.find((t) => t.dot !== 'on')?.tier ?? 'elite'
			};
		})
		.sort((a, b) => Number(b.done > 0) - Number(a.done > 0) || a.name.localeCompare(b.name));
}

export interface ConquestRow {
	tier: CATierView['tier'];
	label: string;
	done: number;
	total: number;
	pct: number;
	/** O jogo conta outro número que o catálogo: vai no title. */
	warn: string | null;
}

/**
 * Conquistas (V7Personagem1440): uma barra por tier de combat achievements, de Easy a Elite, e
 * os mais altos só quando já têm algum; a nota diz o collection log e os tiers ainda vazios.
 */
export function conquests(
	ca: CATierView[],
	clog: CollectionLogView | undefined
): { rows: ConquestRow[]; note: string } {
	const low = new Set<string>(DIARY_TIERS);
	const rows = ca
		.filter((t) => low.has(t.tier) || t.done > 0)
		.map((t) => ({
			tier: t.tier,
			label: CA_TIER_LABEL[t.tier],
			done: t.done,
			total: t.total,
			pct: t.total ? Math.round((100 * t.done) / t.total) : 0,
			warn: caMismatch(t)
		}));
	const empty = ca
		.filter((t) => !low.has(t.tier) && t.done === 0)
		.map((t) => CA_TIER_LABEL[t.tier]);
	const note = [
		clog ? `Collection log: ${n(clog.obtained)}${clog.total ? ` de ${n(clog.total)}` : ''}.` : '',
		empty.length ? `${empty.join(' e ')} ainda sem nada.` : ''
	]
		.filter(Boolean)
		.join(' ');
	return { rows, note };
}

/** Com ela agora: o inventário e o equipamento ao vivo, ou a última leitura deles. */
export function carriedSource(containers: (ContainerView | undefined)[], now: number): Freshness {
	const read = containers.filter((c): c is ContainerView => !!c && c.state !== 'unavailable');
	const top = newest(read);
	if (!top) return { lamp: 'off', text: 'sem leitura' };
	const at = Date.parse(top.observed_at);
	if (top.state === 'restored') return { lamp: 'rest', text: `espelho · ${ago(at, now)}` };
	return now - at < PLAYING_MS
		? { lamp: 'ok', text: `ao vivo · ${ago(at, now)}` }
		: { lamp: 'rest', text: `lido ${ago(at, now)}` };
}

/**
 * O banco só é lido quando a conta o abre no jogo: a lâmpada fica em pausa e o texto diz quando foi.
 * `stale`: a leitura passou de um dia (V7Estados1440, "Dado velho").
 */
export function bankSource(
	bank: ContainerView | undefined,
	now: number
): Freshness & { stale: boolean } {
	if (!bank || bank.state === 'unavailable')
		return { lamp: 'off', text: 'sem leitura', stale: false };
	const at = Date.parse(bank.observed_at);
	return {
		lamp: 'rest',
		text: `lido ${ago(at, now)}, na última vez que ela abriu o banco`,
		stale: now - at > STALE_SOURCE_MS
	};
}
