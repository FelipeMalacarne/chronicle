/**
 * Ironman Planner — espelho em Zod/TS de quatro contratos do servidor Go, só como referência para
 * clientes TypeScript. A fonte da verdade é o código Go:
 *   - contrato de ingest v1: internal/core/ingest/contract.go e validate.go (doc 04, seção 6.4);
 *   - readiness do trecho: internal/core/planning/readiness.go (doc 04, seção 6.4.1);
 *   - eventos de tempo real (SSE): internal/events/hub.go (doc 04, seção 6.6);
 *   - janela da rota do painel do plugin: internal/core/panel/panel.go (doc 04, seção 2.5).
 * O schema do conteúdo é content/schema/guide.schema.json (não há mais tipos Zod para ele); o
 * modelo de progresso está no doc 09. Este arquivo não é compilado nem consumido pela web.
 */
import { z } from 'zod';

export const QuestState = z.enum(['NOT_STARTED', 'IN_PROGRESS', 'FINISHED']);
export type QuestState = z.infer<typeof QuestState>;

/** Veredito de três valores do motor (internal/core/rules). */
export type Verdict = 'met' | 'unmet' | 'unknown';

// ---------------------------------------------------------------------------
// Contrato v1 do plugin Microbot
// ---------------------------------------------------------------------------
// marks:write: escrita das marcas pelo painel do plugin (instalação de dev sem token tem todos).
export const IngestTokenScope = z.enum(['state:write', 'items:bank', 'items:inventory', 'items:equipment', 'marks:write']);
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

/**
 * Faceta de quests (doc 09, item 7): nome RuneLite da quest (Quest.getName()) -> estado. Opcional
 * em todo push; em full, presente = lista completa que substitui as quests guardadas; em delta,
 * só as que mudaram. O cliente só envia se manifest.quests for true.
 */
const QuestStateMap = z.record(
  z.string().max(120).refine(name => name.trim() === name && name.length > 0, 'nome não vazio e sem espaços nas pontas'),
  QuestState,
);

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
  varbits: uniqueNonnegativeIDs,                        // varbits.json + proofs de unlocks.json + regras do conteúdo
  varps: uniqueNonnegativeIDs,                          // idem (varp 101 = quest points)
  collections: uniquePositiveIDs,                       // ordem dos bits do collectionLogSlots
  skills: z.boolean().default(true),
  quests: z.boolean().default(false),                   // o servidor aceita data.quest (servidor anterior: ausente)
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
    quest: QuestStateMap.optional(),                       // todas em full; só as que mudaram em delta
    collectionLogSlots: z.string().optional(),             // base64 bitset, índice = manifest.collections
    collectionLogItemCount: z.number().int().nonnegative().safe().optional(),
    items: IngestItemContainers.optional(),
  }).strict(),
}).strict();

export const IngestPayload = IngestPayloadBase.superRefine((payload, ctx) => {
  const containers = ['bank', 'inventory', 'equipment'] as const;
  if (payload.kind === 'full') {
    // data.quest fica fora: é opcional mesmo em full (plugin 1.0.0 não envia).
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
  // Rota ativa depois da reavaliação deste push (painel do plugin 1.2.0); clientes antigos ignoram.
  route: z.object({
    guide: z.string(),
    cursor: z.string().optional(),               // ausente: rota concluída
    cursor_cause: z.enum(['auto', 'pin']).optional(),
    changed: z.array(z.string()),                // passos da rota ativa que mudaram de status, em ordem
  }).strict().optional(),
}).strict();

// Pedido sem token (só AUTH_MODE=dev e peer de loopback; doc 04, seção 2.5): sem Authorization,
// com estes cabeçalhos. O token ipl_ continua valendo quando presente.
export interface TokenlessHeaders {
  'X-Ironman-Installation': string;  // UUID minúsculo gerado uma vez pelo plugin
  'X-Ironman-Account'?: string;      // config "Conta": id da conta, só com várias contas
}

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
  item_id?: number;               // variante exact; any_of/family chegam com a lista de ids (doc 09, 3.4)
  slot: 'inventory' | 'equipment';
  required: number;
  qty_stated: boolean;            // false = guia não diz quantidade; required = 1
  optional?: boolean;
  source_text?: string;
  verdict: Verdict;               // carregado agora (ids exatos dentro da variante: nota ≠ item, doses distintas)
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
  method: 'explicit' | 'derived' | 'llm' | 'manual';
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

// ---------------------------------------------------------------------------
// Tempo real: GET /api/events?account= (text/event-stream, só AUTH_MODE=dev).
// `event:` é o `type`; `data:` é o JSON abaixo. Sinal de invalidação: o cliente relê pela API.

export const LiveHello = z.object({
  type: z.literal('hello'),
  account: z.string(),
  at: z.string().datetime({ offset: true }),
});

export const LiveEvent = z.object({
  type: z.enum(['progress', 'sync']),
  account: z.string(),
  source: z.enum(['plugin', 'hiscores', 'owner']),
  guides: z.array(z.string()).optional(),       // progress: o guia da marca, ou todos
  steps_changed: z.number().int().nonnegative(), // só mudanças causadas pelo motor
  status: z.enum(['ok', 'unchanged', 'error', 'rate_limited']).optional(), // sync
  at: z.string().datetime({ offset: true }),
});
export type LiveEvent = z.infer<typeof LiveEvent>;

// ---------------------------------------------------------------------------
// Painel do plugin: GET /api/plugin/route?limit= (passos depois do cursor, 1..50, padrão 15);
// PATCH /api/plugin/guides/{guide}/steps/{id}/mark?limit= ({kind, note?}, como a web) e
// POST /api/plugin/guides/{guide}/marks?limit= ({restore: MarkRestore[]}) respondem com a mesma
// janela. Autenticação do plugin (token ou sem token em dev); escrita por token exige marks:write.
// ---------------------------------------------------------------------------
export type StepStatus = 'pending' | 'auto_done' | 'manual_done' | 'implied' | 'skipped' | 'pinned_open';

export interface StepMark {
  kind: 'manual_done' | 'skipped' | 'pinned_open' | 'implied';
  implied_by_step_id?: string;
  note?: string;
}

export interface RouteWindow {
  guide: string;
  title: string;
  attribution: string;                 // regra 6 do CLAUDE.md
  cursor: { step_id: string; cause: 'auto' | 'pin' } | null;
  counts: { total: number; done: number; auto_done: number; manual_done: number; implied: number;
            skipped: number; pinned_open: number; remaining: number; blocked: number };
  steps: RouteWindowStep[];            // o passo do cursor (current) e os próximos visíveis da rota principal
  carry: RouteCarry | null;            // trecho do cursor
  section?: { id: string; title: string; description?: string };  // seção do cursor
}

export interface RouteWindowStep {
  id: string;
  ref: string;                         // referência do autor
  section_id: string;
  section_title?: string;
  title: string;
  text: string;
  location?: string;
  status: StepStatus;
  mark?: StepMark;
  current: boolean;
  blocked_by: { key: string; label: string }[];  // "falta …"
  objectives: { key: string; role: 'completes' | 'starts' | 'requires' | 'uses'; label: string;
                verdict: Verdict; regressed?: boolean }[];
  notes?: { kind: 'tip' | 'update' | 'hcim'; text: string }[];  // notas do autor, sem metadados
}

export interface RouteCarry {
  segment_id: string;
  title?: string;
  status: ReadinessStatus;
  loadout_status: 'known' | 'partial' | 'unknown';
  derived: boolean;                    // sem setup no guia: prévia dos próximos passos, nunca loadout
  items: { name: string; qty: number; qty_stated: boolean; slot: 'inventory' | 'equipment';
           optional?: boolean; verdict: Verdict; carried: number; owned?: Verdict; held?: number }[];
  bank_actions: string[];
}
