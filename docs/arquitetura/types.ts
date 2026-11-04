/**
 * Ironman Planner — tipos compartilhados (esboço).
 * Vive em src/lib/core/ (código puro, sem imports de servidor nem de browser).
 * Zod é a fonte da verdade em runtime; guide.schema.json é gerado a partir dele
 * (zod-to-json-schema) para editores e CI. Este arquivo é um sketch, não o código final.
 */
import { z } from 'zod';

// ---------------------------------------------------------------------------
// Enums de domínio
// ---------------------------------------------------------------------------
export const SKILLS = [
  'attack','strength','defence','ranged','prayer','magic','runecraft','construction',
  'hitpoints','agility','herblore','thieving','crafting','fletching','slayer','hunter',
  'mining','smithing','fishing','cooking','firemaking','woodcutting','farming','sailing',
] as const;
export const Skill = z.enum(SKILLS);
export type Skill = z.infer<typeof Skill>;

export const QuestState = z.enum(['NOT_STARTED', 'IN_PROGRESS', 'FINISHED']);
export type QuestState = z.infer<typeof QuestState>;
export const QUEST_STATE_ORDER: Record<QuestState, number> = { NOT_STARTED: 0, IN_PROGRESS: 1, FINISHED: 2 };

export const DiaryTier = z.enum(['easy', 'medium', 'hard', 'elite']);
export const Mode = z.enum(['ironman', 'hcim', 'uim', 'gim']);
export const Layer = z.enum(['classic', 'enhanced']);
export const Phase = z.enum(['early', 'mid', 'late', 'endgame']);
export type Mode = z.infer<typeof Mode>;
export type Layer = z.infer<typeof Layer>;

export const StableId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/);
export const Ref = z.string().regex(/^(step|quest|skill|diary|diary_task|item|boss|ca|goal):.+$/);
export type Ref = z.infer<typeof Ref>;

// ---------------------------------------------------------------------------
// Regras (complete_when)
// ---------------------------------------------------------------------------
const base = <T extends string>(t: T) => z.object({ type: z.literal(t) });

export const SkillLevelRule = base('skill_level').extend({ skill: Skill, level: z.number().int().min(1).max(126), boosted: z.boolean().default(false) });
export const SkillXpRule = base('skill_xp').extend({ skill: Skill, xp: z.number().int().min(0) });
export const QuestStateRule = base('quest_state').extend({ quest: z.string(), state: QuestState, op: z.enum(['eq', 'gte']).default('gte') });
export const DiaryTaskRule = base('diary_task').extend({ area: z.string(), tier: DiaryTier, task: z.union([z.number().int(), z.string()]) });
export const DiaryTierRule = base('diary_tier').extend({ area: z.string(), tier: DiaryTier });
export const CombatAchievementRule = base('combat_achievement').extend({
  name: z.string().optional(),
  tier: z.enum(['easy', 'medium', 'hard', 'elite', 'master', 'grandmaster']).optional(),
  min_count: z.number().int().min(1).optional(),
}).refine(r => r.name || r.tier, 'name ou tier');
export const BossKcRule = base('boss_kc').extend({ boss: z.string(), kc: z.number().int().min(1) });
export const ActivityScoreRule = base('activity_score').extend({ activity: z.string(), score: z.number().int().min(1) });
export const CollectionLogItemRule = base('collection_log_item').extend({ item_id: z.number().int().optional(), item_name: z.string().optional(), qty: z.number().int().min(1).default(1) });
const varCmp = { eq: z.number().int().optional(), min: z.number().int().optional(), bit: z.number().int().min(0).max(31).optional(), label: z.string().optional() };
export const VarbitRule = base('varbit').extend({ id: z.number().int(), ...varCmp });
export const VarpRule = base('varp').extend({ id: z.number().int(), ...varCmp });
export const ItemOwnedRule = base('item_owned').extend({
  item_id: z.number().int().optional(), item_name: z.string().optional(),
  qty: z.number().int().min(1).default(1),
  containers: z.array(z.enum(['bank', 'inventory', 'equipment', 'poh'])).default(['bank', 'inventory', 'equipment']),
});
export const ManualRule = base('manual').extend({ label: z.string().optional() });

export type Rule =
  | z.infer<typeof SkillLevelRule> | z.infer<typeof SkillXpRule> | z.infer<typeof QuestStateRule>
  | z.infer<typeof DiaryTaskRule> | z.infer<typeof DiaryTierRule> | z.infer<typeof CombatAchievementRule>
  | z.infer<typeof BossKcRule> | z.infer<typeof ActivityScoreRule> | z.infer<typeof CollectionLogItemRule>
  | z.infer<typeof VarbitRule> | z.infer<typeof VarpRule> | z.infer<typeof ItemOwnedRule> | z.infer<typeof ManualRule>
  | { all: Rule[] } | { any: Rule[] } | { not: Rule };

export const Rule: z.ZodType<Rule> = z.lazy(() => z.union([
  SkillLevelRule, SkillXpRule, QuestStateRule, DiaryTaskRule, DiaryTierRule, CombatAchievementRule,
  BossKcRule, ActivityScoreRule, CollectionLogItemRule, VarbitRule, VarpRule, ItemOwnedRule, ManualRule,
  z.object({ all: z.array(Rule).min(1) }),
  z.object({ any: z.array(Rule).min(1) }),
  z.object({ not: Rule }),
]));

// ---------------------------------------------------------------------------
// Conteúdo: guide -> section -> step
// ---------------------------------------------------------------------------
export const Source = z.object({
  origin: z.enum(['ironman.guide', 'osrs-wiki', 'own', 'other']).optional(),
  original_id: z.string().optional(), url: z.string().url().optional(),
  author: z.string().optional(), license: z.string().optional(), imported_at: z.string().optional(),
});
export const Link = z.object({ type: z.enum(['image', 'video', 'guide', 'wiki', 'map', 'other']), url: z.string().url(), text: z.string().optional() });
export const Note = z.object({ kind: z.enum(['tip', 'hcim', 'update', 'warning']), text: z.string().min(1) });
export const Item = z.union([z.string().min(1), z.object({ name: z.string(), item_id: z.number().int().optional(), qty: z.number().int().min(1).optional(), equip: z.boolean().optional(), note: z.string().optional() })]);

export const Step = z.object({
  id: StableId,
  text: z.string().min(1),
  location: z.string().optional(),
  notes: z.array(Note).default([]),
  items: z.array(Item).default([]),
  links: z.array(Link).default([]),
  complete_when: z.array(Rule).default([]),      // vazio => manual
  manual_ok: z.boolean().default(true),
  requires: z.array(Ref).default([]),
  recommended_after: z.array(Ref).default([]),
  unlocks: z.array(Ref).default([]),
  tags: z.array(z.string().regex(/^[a-z0-9][a-z0-9:_-]*$/)).default([]),
  modes: z.array(Mode).min(1).default(['ironman', 'hcim', 'uim', 'gim']),
  layers: z.array(Layer).min(1).default(['classic', 'enhanced']),
  phase: Phase.optional(),
  optional: z.boolean().default(false),
  estimate_minutes: z.number().int().min(0).optional(),
  replaces: z.array(StableId).default([]),
  source: Source.optional(),
}).strict();
export type Step = z.infer<typeof Step>;

export const Section = z.object({
  id: StableId, slug: z.string().regex(/^[a-z0-9-]+$/), title: z.string(),
  description: z.string().optional(),
  kind: z.enum(['main', 'track']).default('main'),
  related: z.array(z.object({ href: z.string().optional(), label: z.string() })).default([]),
  tags: z.array(z.string()).default([]),
  steps: z.array(Step),
}).strict();

export const Guide = z.object({
  schema_version: z.literal(1), id: StableId, slug: z.string().optional(), title: z.string(),
  description: z.string().optional(), version: z.string().optional(),
  default_modes: z.array(Mode).optional(), default_layers: z.array(Layer).optional(),
  source: Source.optional(), sections: z.array(Section).min(1),
}).strict();
export type Guide = z.infer<typeof Guide>;

// ---------------------------------------------------------------------------
// Estado do jogador (entrada do rule engine) — já mesclado de todas as fontes
// ---------------------------------------------------------------------------
export type Observed<T> = T & { source: SourceName; observed_at: string; snapshot_id: string };
export type SourceName = 'hiscores' | 'wom' | 'wikisync' | 'plugin' | 'import';

export interface PlayerState {
  account_id: string;
  mode: Mode;
  skills: Partial<Record<Skill, Observed<{ level: number; xp: number; boosted?: number }>>>;
  activities: Record<string, Observed<{ score: number }>>;                 // bosses, clues, minigames
  quests: Record<string, Observed<{ state: QuestState }>>;
  diary_tasks: Record<string, Observed<{ done: boolean }>>;              // 'Ardougne:easy:3'
  combat_achievements: Record<number, Observed<{ done: boolean }>>;
  collection_log: Record<number, Observed<{ qty: number }>>;             // item_id
  varbits: Record<number, Observed<{ value: number }>>;
  varps: Record<number, Observed<{ value: number }>>;
  items: Record<string, Observed<{ qty: number }>>;                      // 'bank:4151'
}

// ---------------------------------------------------------------------------
// Rule engine — função pura
// ---------------------------------------------------------------------------
export type Verdict = 'met' | 'unmet' | 'unknown';   // unknown = faceta nunca observada para esta conta

export interface Evidence {
  rule: Rule;
  verdict: Verdict;
  observed?: unknown;            // valor que sustentou o veredito (ex.: {level: 57})
  reason?: string;
  stale?: boolean;               // item_owned met só por um container restored
  source?: SourceName;
  observed_at?: string;
  snapshot_id?: string;
}

export interface Evaluation {
  verdict: Verdict;
  evidence: Evidence[];
  oldest_observed_at?: string;   // base para 'stale'
  monotonic: boolean;            // true se nenhuma regra pode regredir (skill, quest, clog, kc)
}

export interface EvalOptions { now: string; staleAfterHours: number; }

/** Avalia uma lista de regras (AND implícito) contra o estado. Determinística, sem IO. */
export declare function evaluateRules(rules: Rule[], state: PlayerState, opts: EvalOptions): Evaluation;

export type StepStatus = 'pending' | 'auto_done' | 'manual_done' | 'skipped';
export type Freshness = 'fresh' | 'stale' | 'unknown';

export interface StepProgressRow {
  step_id: string; status: StepStatus; completed_at?: string;
  evidence_snapshot_id?: string; evidence?: Evidence[]; rule_hash?: string;
  manual_locked: boolean; regressed_at?: string;
}

export interface StepView extends StepProgressRow { freshness: Freshness; evaluation?: Evaluation }

/**
 * Combina a avaliação com o progresso existente e devolve a nova linha + se mudou.
 * Regras: manual_locked nunca é alterado pelo engine; auto_done é "pegajoso"
 * para regras monotônicas (marca regressed_at em vez de voltar a pending);
 * para regras não monotônicas (item_owned, varbit sem min) volta a pending.
 */
export declare function reconcileStep(
  step: Pick<Step, 'id' | 'complete_when'>,
  previous: StepProgressRow | undefined,
  evaluation: Evaluation,
  ctx: { snapshot_id: string; rule_hash: string; now: string },
): { row: StepProgressRow; changed: boolean };

// ---------------------------------------------------------------------------
// Adapters de fonte (shapes externos podem mudar: só o adapter conhece o shape)
// ---------------------------------------------------------------------------
export interface StatePatch {
  skills?: Partial<Record<Skill, { level: number; xp: number; boosted?: number }>>;
  activities?: Record<string, { score: number }>;
  quests?: Record<string, { state: QuestState }>;
  diary_tasks?: Record<string, { done: boolean }>;
  combat_achievements?: Record<number, { done: boolean }>;
  collection_log?: Record<number, { qty: number }>;
  varbits?: Record<number, { value: number }>;
  varps?: Record<number, { value: number }>;
  items?: Record<string, { qty: number }>;
}

export interface SourceAdapter<Raw = unknown> {
  readonly name: SourceName;
  readonly version: number;                     // bump => snapshots antigos podem ser re-normalizados
  fetch(account: { rsn: string; mode: Mode; wom_player_id?: number }, signal: AbortSignal): Promise<{ raw: Raw; http_status: number }>;
  normalize(raw: Raw): StatePatch;              // pura; testada com fixtures gravadas
  rateLimit: { minIntervalSec: number; maxPerHour: number };
}

// ---------------------------------------------------------------------------
// Contrato v1 do plugin RuneLite/Microbot
// ---------------------------------------------------------------------------
export const IngestTokenScope = z.enum(['state:write', 'items:bank', 'items:inventory', 'items:equipment']);
export type IngestTokenScope = z.infer<typeof IngestTokenScope>;

const uniquePositiveIDs = z.array(z.number().int().positive().safe()).refine(
  ids => new Set(ids).size === ids.length,
  'ids devem ser únicos',
);
const uniqueNonnegativeIDs = z.array(z.number().int().nonnegative().safe()).refine(
  ids => new Set(ids).size === ids.length,
  'ids devem ser únicos',
);
const IntegerMap = z.record(z.string(), z.number().int().safe());
const ItemQuantityMap = z.record(
  z.string().regex(/^[1-9]\d*$/),
  z.number().int().nonnegative().safe(),
); // objeto JSON item_id decimal -> qty; Gson: Map<String, Long>

const IngestItemSelection = z.object({
  bank: z.boolean().default(false),
  inventory: z.boolean().default(false),
  equipment: z.boolean().default(false),
  only_item_ids: uniquePositiveIDs.optional(),
}).strict();

export const IngestManifest = z.object({
  contract_version: z.literal(1),
  manifest_version: z.number().int().positive().safe(),
  token_scopes: z.array(IngestTokenScope).refine(scopes => new Set(scopes).size === scopes.length, 'scopes devem ser únicos'),
  varbits: uniqueNonnegativeIDs,
  varps: uniqueNonnegativeIDs,
  collections: uniquePositiveIDs,                       // ordem dos bits do collectionLogSlots
  skills: z.boolean().default(true),
  items: IngestItemSelection,
  interval_seconds: z.number().int().min(5).default(10),
  resync: z.boolean().default(false),                    // próximo push deve ser full
}).strict().superRefine((manifest, ctx) => {
  const scopes = new Set<IngestTokenScope>(manifest.token_scopes);
  if (!scopes.has('state:write')) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['token_scopes'], message: 'state:write é obrigatório' });
  }
  const flags: Array<[keyof Pick<z.infer<typeof IngestItemSelection>, 'bank' | 'inventory' | 'equipment'>, IngestTokenScope]> = [
    ['bank', 'items:bank'], ['inventory', 'items:inventory'], ['equipment', 'items:equipment'],
  ];
  for (const [container, scope] of flags) {
    if (manifest.items[container] !== scopes.has(scope)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['items', container], message: `deve corresponder ao scope ${scope}` });
    }
  }
});
export type IngestManifest = z.infer<typeof IngestManifest>;

export const IngestItemUnavailable = z.object({
  state: z.literal('unavailable'),
  observed_at: z.string().datetime({ offset: true }),
}).strict();
export const IngestItemRestored = z.object({
  state: z.literal('restored'),
  observed_at: z.string().datetime({ offset: true }),
  revision: z.number().int().nonnegative().safe(),
  quantities: ItemQuantityMap,
}).strict();
export const IngestItemLive = z.object({
  state: z.literal('live'),
  observed_at: z.string().datetime({ offset: true }),
  revision: z.number().int().nonnegative().safe(),
  live_epoch: z.string().trim().min(1),
  quantities: ItemQuantityMap,
}).strict();
export const IngestItemObservation = z.discriminatedUnion('state', [
  IngestItemUnavailable, IngestItemRestored, IngestItemLive,
]);
export type IngestItemObservation = z.infer<typeof IngestItemObservation>;

const IngestItemContainers = z.object({
  bank: IngestItemObservation.optional(),
  inventory: IngestItemObservation.optional(),
  equipment: IngestItemObservation.optional(),
}).strict();

const IngestPayloadBase = z.object({
  contract_version: z.literal(1),
  username: z.string().refine(value => value.trim().length > 0, 'username é obrigatório'),
  profile: z.enum(['STANDARD', 'BETA', 'DEPRECATED', 'OTHER']),
  account_type: z.enum(['NORMAL', 'IRONMAN', 'HARDCORE_IRONMAN', 'ULTIMATE_IRONMAN', 'GROUP_IRONMAN', 'HARDCORE_GROUP_IRONMAN', 'UNRANKED_GROUP_IRONMAN']).optional(),
  manifest_version: z.number().int().positive().safe(),
  client_seq: z.number().int().positive().safe(),
  kind: z.enum(['full', 'delta']),
  sent_at: z.string().datetime({ offset: true }),
  data: z.object({
    varb: IntegerMap.optional(),
    varp: IntegerMap.optional(),
    level: IntegerMap.optional(),
    xp: IntegerMap.optional(),
    boosted: IntegerMap.optional(),
    collectionLogSlots: z.string().optional(),             // base64 bitset, índice = manifest.collections
    collectionLogItemCount: z.number().int().nonnegative().safe().optional(),
    items: IngestItemContainers.optional(),
  }).strict(),
}).strict();

export const IngestPayload = IngestPayloadBase.superRefine((payload, ctx) => {
  const containers = ['bank', 'inventory', 'equipment'] as const;
  if (payload.kind === 'full') {
    for (const field of ['varb', 'varp', 'level', 'xp', 'boosted'] as const) {
      if (payload.data[field] === undefined) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', field], message: 'obrigatório em full' });
      }
    }
    if (payload.data.items === undefined) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', 'items'], message: 'obrigatório em full' });
    }
    for (const container of containers) {
      const observation = payload.data.items?.[container];
      if (observation === undefined) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', 'items', container], message: 'obrigatório em full' });
      } else if (observation.state !== 'unavailable') {
        for (const [itemID, qty] of Object.entries(observation.quantities)) {
          if (qty === 0) {
            ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', 'items', container, 'quantities', itemID], message: 'qty deve ser positiva em full' });
          }
        }
      }
    }
  } else {
    for (const container of containers) {
      if (payload.data.items?.[container]?.state === 'restored') {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', 'items', container, 'state'], message: 'restored só é válido em full' });
      }
    }
  }

  const sentAt = Date.parse(payload.sent_at);
  for (const container of containers) {
    const observation = payload.data.items?.[container];
    if (observation && Date.parse(observation.observed_at) > sentAt) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['data', 'items', container, 'observed_at'], message: 'não pode ser posterior a sent_at' });
    }
  }
});
export type IngestPayload = z.infer<typeof IngestPayload>;

export const IngestResponse = z.object({
  contract_version: z.literal(1),
  ok: z.boolean(),
  accepted_seq: z.number().int().nonnegative().safe(),
  resync: z.boolean().default(false),          // true => cliente deve mandar 'full' no próximo ciclo
  manifest_version: z.number().int().positive().safe(), // se diferente, cliente rebaixa manifest
  steps_changed: z.number().int().nonnegative().safe().optional(),
}).strict();

// ---------------------------------------------------------------------------
// Readiness do trecho — GET /api/guides/{guide}/readiness?account=&step=
// e GET /api/guides/{guide}/segments/{segment}/readiness?account=
// (internal/core/planning; rotas só em AUTH_MODE=dev até existir sessão)
// ---------------------------------------------------------------------------
// Três semânticas separadas: posse (o item existe em algum container),
// readiness (inventário/equipamento live batem com o loadout do segmento) e
// ações de banco (instruções do guia, devolvidas como texto e nunca concluídas).
export type ItemContainer = 'bank' | 'inventory' | 'equipment';
export type ItemContainerState = 'unavailable' | 'restored' | 'live';

export interface ContainerEvidence {
  state?: ItemContainerState;     // ausente = nunca observado (sem escopo/opt-in)
  observed: boolean;
  complete: boolean;              // existe full no mesmo estado/epoch: ausência é prova
  qty: number;
  observed_at?: string;
}

export interface Ownership {
  verdict: Verdict;
  stale?: boolean;                // met só graças a um banco restored
  held: number;
  required: number;
  containers: Partial<Record<ItemContainer | 'poh', ContainerEvidence>>;
  reason?: string;
}

export type ReadinessStatus = 'ready' | 'listed_ready' | 'not_ready' | 'unknown' | 'no_loadout';

export interface ReadinessItem {
  name: string;
  item_id?: number;
  slot: 'inventory' | 'equipment';
  required: number;
  qty_stated: boolean;            // false = guia não diz quantidade; required = 1
  optional?: boolean;
  source_text?: string;
  verdict: Verdict;               // carregado agora (ids exatos: nota ≠ item, doses distintas)
  carried: number;
  owned?: Ownership;              // posse em qualquer container (nota = item)
  reason?: string;
}

export interface ReadinessSlots {
  required: number;
  free_min?: number;              // intervalo: empilhabilidade fora do catálogo é desconhecida
  free_max?: number;
  verdict: Verdict;
  reason?: string;
}

export interface ContainerFreshness {
  state?: ItemContainerState;
  observed: boolean;
  complete: boolean;
  live: boolean;                  // só live prova loadout atual
  observed_at?: string;
}

export interface BankAction {
  type: 'open_bank' | 'deposit_all' | 'deposit' | 'withdraw' | 'bank_setup' | 'close_bank';
  position: 'start' | 'within' | 'end';
  text: string;
  source: Provenance;
}

export interface Provenance {
  method: 'explicit' | 'derived';
  confidence: 'high' | 'medium' | 'low';
  step_ids: string[];
  evidence?: string;
}

export interface SegmentReadiness {
  guide_id: string;
  segment_id: string;
  section_id: string;
  kind: 'bank_leg' | 'sequence';
  title?: string;
  first_step_id?: string;
  status: ReadinessStatus;
  loadout_status: 'known' | 'partial' | 'unknown';
  items: ReadinessItem[];
  empty_slots?: ReadinessSlots;
  bank_actions: BankAction[];
  freshness: Record<ItemContainer, ContainerFreshness>;
  loadout_source?: Provenance;
  reason?: string;
}

export interface ReadinessResponse {
  guide: string;
  step?: string;
  current: SegmentReadiness | null;   // segmento que contém o step (rota principal)
  next: SegmentReadiness | null;      // próximo bank_leg da mesma variante
}
