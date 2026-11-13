/**
 * Rotinas da frota (v7, fatia V6; artboard V7Rotinas1440): as rotinas recorrentes e os gatilhos de
 * cada conta na ordem em que entram na fila dela, as rotinas da frota (as cópias de cada conta
 * juntas) e o formulário da rotina aberta. Puro, testado em node.
 */
import type {
	BreakState,
	Recurring,
	RecurringClock,
	RecurringReason,
	RecurringState,
	RecurringStatus,
	Run,
	RunParams,
	Trigger,
	TriggerEvent,
	Triggers
} from '../api/microbot.ts';
import { RUN_LABEL, runDuration, runOutcome } from './fazer.ts';

/**
 * Os relógios na ordem do painel Time Tracking do RuneLite, com o nome que ele mostra (a aba
 * `Tab` do RuneLite) e o sprite do item da aba quando o catálogo tem um.
 */
export const CLOCKS: { key: RecurringClock; label: string; item?: number }[] = [
	{ key: 'BIRD_HOUSE', label: 'Bird Houses', item: 21512 },
	{ key: 'HERB', label: 'Herb Patches', item: 207 },
	{ key: 'TREE', label: 'Tree Patches', item: 1515 },
	{ key: 'FRUIT_TREE', label: 'Fruit Tree Patches', item: 2114 },
	{ key: 'SPECIAL', label: 'Special Patches', item: 6004 },
	{ key: 'FLOWER', label: 'Flower Patches', item: 2462 },
	{ key: 'ALLOTMENT', label: 'Allotment Patches', item: 1965 },
	{ key: 'BUSH', label: 'Bush Patches', item: 6018 },
	{ key: 'GRAPE', label: 'Grape Patches', item: 1987 },
	{ key: 'HOPS', label: 'Hops Patches', item: 6006 },
	{ key: 'FARMING_CONTRACT', label: 'Farming contract', item: 22993 }
];

export const clockLabel = (key: string | undefined) =>
	CLOCKS.find((c) => c.key === key)?.label ?? key ?? '';

/** Os eventos que um gatilho espera, como a tela diz ("quando o filhote crescer"). */
export const EVENTS: { key: TriggerEvent; label: string; name: string }[] = [
	{ key: 'kitten_grown', label: 'o filhote crescer', name: 'Trocar o gato' }
];

export const eventOf = (key: string) => EVENTS.find((e) => e.key === key);

/** Limites do servidor (agent.ValidateRecurring e validateRun). */
export const LIMITS = {
	nameMax: 80,
	intervalMin: 10,
	intervalMax: 7 * 24 * 60,
	priorityMax: 100,
	timeoutMin: 1,
	timeoutMax: 120,
	timeoutUntilMax: 12 * 60,
	stopMax: 8
};

/** "45 min", "1 h", "2 h 30", "1 dia", "1 dia 6 h", "7 dias". */
export function intervalText(min: number): string {
	if (min < 60) return `${min} min`;
	const d = Math.floor(min / 1440);
	const h = Math.floor((min % 1440) / 60);
	const m = min % 60;
	if (d === 0) return m ? `${h} h ${String(m).padStart(2, '0')}` : `${h} h`;
	const days = `${d} ${d === 1 ? 'dia' : 'dias'}`;
	if (!h && !m) return days;
	return m ? `${days} ${h} h ${String(m).padStart(2, '0')}` : `${days} ${h} h`;
}

const hm = (d: Date) => d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
const dayOf = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

/**
 * Quando algo da fila acontece, pelo relógio do navegador: "agora", "em 12 min" (dentro da hora),
 * "15:00" (hoje), "amanhã 08:10", "16/11 09:00".
 */
export function whenText(iso: string, now: Date): string {
	const at = new Date(iso);
	const diff = at.getTime() - now.getTime();
	if (diff <= 0) return 'agora';
	if (diff < 3_600_000) return `em ${Math.max(1, Math.ceil(diff / 60_000))} min`;
	const days = Math.round((dayOf(at) - dayOf(now)) / 86_400_000);
	if (days === 0) return hm(at);
	if (days === 1) return `amanhã ${hm(at)}`;
	return `${at.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' })} ${hm(at)}`;
}

/** "hoje 13:58" vira "13:58"; "ontem 23:10" e "08/11 09:15" ficam. */
export function shortWhen(iso: string, now: Date): string {
	const at = new Date(iso);
	const days = Math.round((dayOf(now) - dayOf(at)) / 86_400_000);
	if (days === 0) return hm(at);
	if (days === 1) return 'ontem';
	return at.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' });
}

/** "net...fm.birdhouse.BirdhouseRunnerPlugin" vira "Birdhouse Runner" quando o cliente não diz o nome. */
export function pluginLabel(className: string, names?: Map<string, string>): string {
	const known = names?.get(className);
	if (known) return known;
	const simple = (className.split('.').pop() ?? className).replace(/Plugin$/, '');
	return simple.replace(/([a-z0-9])([A-Z])/g, '$1 $2') || className;
}

/** O "quando" de uma rotina numa linha: "quando Herb Patches fica pronto", "a cada 2 h". */
export function whenRule(rc: Pick<Recurring, 'clock' | 'intervalMin'>): string {
	const every = rc.intervalMin ? intervalText(rc.intervalMin) : '';
	if (rc.clock) {
		const ready = `quando ${clockLabel(rc.clock)} fica pronto`;
		return every ? `${ready}; sem leitura, a cada ${every}` : ready;
	}
	return every ? `a cada ${every} depois do último run` : '';
}

export type RowTone = 'due' | 'wait' | 'held' | 'off' | 'idle';

/** Uma linha da lista de rotinas: uma recorrente ou um gatilho. */
export interface RoutineRow {
	/** recurring:<id> ou trigger:<evento>; único na conta. */
	key: string;
	kind: 'recurring' | 'trigger';
	id?: string;
	event?: TriggerEvent;
	name: string;
	/** "quando Bird Houses fica pronto · Birdhouse Runner". */
	line: string;
	enabled: boolean;
	priority: number;
	/** O que a coluna da fila diz: "agora", "em 12 min", "15:00", "segura", "desligada". */
	when: string;
	/** Por que: vai para o title da coluna. */
	why: string;
	tone: RowTone;
	/** Clock (sprite) ou evento. */
	clock?: RecurringClock;
	held?: 'held' | 'held_agent';
	nextAt?: string;
	run: RunParams;
	recurring?: Recurring;
	trigger?: Trigger;
}

/**
 * Por que uma rotina está segura sem o vigia do agent, pelo que o banco tem: o último run dela
 * falhou ou foi cancelado depois de o dono salvá-la (held), ou o último run da conta falhou depois
 * do último salvamento de qualquer rotina (held_agent). As mesmas regras do agent (assess).
 */
export function offlineHold(
	rc: Recurring,
	all: Recurring[],
	runs: Run[]
): 'held' | 'held_agent' | null {
	if (!rc.enabled) return null;
	const saved = rc.updatedAt ? Date.parse(rc.updatedAt) : 0;
	const last = rc.lastRun;
	if (last) {
		const end = Date.parse(last.finishedAt ?? last.startedAt);
		if ((!last.finishedAt || last.state !== 'done') && end > saved) return 'held';
	}
	const latestSave = Math.max(0, ...all.map((x) => (x.updatedAt ? Date.parse(x.updatedAt) : 0)));
	const latest = runs[0];
	if (
		latest &&
		latest.finishedAt &&
		(latest.state === 'failed' || latest.state === 'canceled') &&
		Date.parse(latest.finishedAt) > latestSave
	)
		return 'held_agent';
	return null;
}

const HELD_WHY = {
	held: 'O último run dela falhou ou foi cancelado depois de você salvá-la: fica parada até você salvá-la de novo.',
	held_agent:
		'O último run da conta falhou depois do último salvamento: as rotinas esperam até você salvar uma delas.'
};

/** O que a coluna da fila diz de uma recorrente, e por quê (title). */
export function recurringWhen(
	st: RecurringState,
	now: Date,
	hold: 'held' | 'held_agent' | null = null
): { when: string; why: string; tone: RowTone } {
	const clock = clockLabel(st.clock);
	const at = st.nextAt ? whenText(st.nextAt, now) : '';
	const reason: RecurringReason | 'offline' | undefined = st.reason ?? hold ?? 'offline';
	switch (reason) {
		case 'off':
			return { when: 'desligada', why: 'Desligada: não começa sozinha.', tone: 'off' };
		case 'clock_ready':
			return {
				when: 'agora',
				why: `${clock} está pronto: entra no próximo ponto seguro.`,
				tone: 'due'
			};
		case 'first':
			return { when: 'agora', why: 'Nunca rodou: entra no próximo ponto seguro.', tone: 'due' };
		case 'interval':
			return {
				when: 'agora',
				why: 'O intervalo desde o último run passou: entra no próximo ponto seguro.',
				tone: 'due'
			};
		case 'clock_waiting':
			return at
				? { when: at, why: `${clock} fica pronto ${at === 'agora' ? 'agora' : at}.`, tone: 'wait' }
				: {
						when: 'relógio parado',
						why: `${clock} não está crescendo: plantado sem prazo ou vazio.`,
						tone: 'idle'
					};
		case 'interval_waiting':
			return {
				when: at || 'esperando',
				why: 'Esperando o intervalo desde o último run.',
				tone: 'wait'
			};
		case 'clock_stale':
			return {
				when: 'esperando o relógio',
				why: `Rodou com ${clock} pronto; espera o relógio mudar para rodar de novo.`,
				tone: 'idle'
			};
		case 'no_clock':
			return {
				when: 'sem relógio',
				why: `O Time Tracking não conhece ${clock} e ela não tem intervalo.`,
				tone: 'idle'
			};
		case 'held':
		case 'held_agent':
			return { when: 'segura', why: HELD_WHY[reason], tone: 'held' };
		default:
			return {
				when: st.enabled ? '' : 'desligada',
				why: st.enabled ? 'Sem a leitura do agent.' : 'Desligada: não começa sozinha.',
				tone: st.enabled ? 'idle' : 'off'
			};
	}
}

const RANK: Record<RowTone, number> = { due: 0, wait: 1, idle: 2, held: 3, off: 4 };

/**
 * A ordem da fila de uma conta: as prontas pela prioridade (a ordem do vigia), depois as que têm
 * hora pela hora, os gatilhos e as que esperam sem hora, as seguras e por fim as desligadas.
 */
export function queueOrder(rows: RoutineRow[]): RoutineRow[] {
	return [...rows].sort(
		(a, b) =>
			RANK[a.tone] - RANK[b.tone] ||
			(a.tone === 'wait' ? Date.parse(a.nextAt ?? '') - Date.parse(b.nextAt ?? '') : 0) ||
			b.priority - a.priority ||
			a.name.localeCompare(b.name) ||
			a.key.localeCompare(b.key)
	);
}

/** As linhas de uma conta: as recorrentes como o vigia (ou o banco) diz e os gatilhos. */
export function accountRows(
	status: RecurringStatus | null,
	triggers: Triggers | null,
	runs: Run[],
	now: Date,
	names?: Map<string, string>
): RoutineRow[] {
	const list = status?.recurring ?? [];
	const out: RoutineRow[] = list.map((st) => {
		const hold = st.reason ? null : offlineHold(st, list, runs);
		const w = recurringWhen(st, now, hold);
		const held =
			st.reason === 'held' || st.reason === 'held_agent' ? st.reason : (hold ?? undefined);
		return {
			key: `recurring:${st.id}`,
			kind: 'recurring',
			id: st.id,
			name: st.name,
			line: [whenRule(st), pluginLabel(st.run.className, names)].filter(Boolean).join(' · '),
			enabled: st.enabled,
			priority: st.priority,
			when: w.when,
			why: w.why,
			tone: w.tone,
			clock: st.clock,
			held,
			nextAt: w.tone === 'wait' ? st.nextAt : undefined,
			run: st.run,
			recurring: stripState(st)
		};
	});
	for (const t of triggers?.triggers ?? []) {
		const ev = eventOf(t.event);
		const watching = triggers?.watching ?? false;
		out.push({
			key: `trigger:${t.event}`,
			kind: 'trigger',
			event: t.event,
			name: ev?.name ?? t.event,
			line: [`quando ${ev?.label ?? t.event}`, pluginLabel(t.run.className, names)].join(' · '),
			enabled: t.enabled,
			priority: 0,
			when: !t.enabled ? 'desligada' : watching ? 'no evento' : 'sem vigia',
			why: !t.enabled
				? 'Desligada: não começa sozinha.'
				: watching
					? `Entra quando ${ev?.label ?? t.event}.`
					: 'O agent não está lendo o jogo agora: o evento passa sem disparar.',
			tone: !t.enabled ? 'off' : 'idle',
			run: t.run,
			trigger: t
		});
	}
	return queueOrder(out);
}

/** A rotina como o servidor guarda, sem o que o vigia acrescenta. */
export function stripState(st: RecurringState): Recurring {
	const rc: Recurring = {
		id: st.id,
		name: st.name,
		enabled: st.enabled,
		priority: st.priority,
		run: st.run
	};
	if (st.clock) rc.clock = st.clock;
	if (st.intervalMin) rc.intervalMin = st.intervalMin;
	if (st.updatedAt) rc.updatedAt = st.updatedAt;
	if (st.lastRun) rc.lastRun = st.lastRun;
	return rc;
}

/** O corpo de um POST que só muda `enabled` (o servidor pede a rotina inteira e ignora o resto). */
export function withEnabled(rc: Recurring, enabled: boolean): Recurring {
	const body: Recurring = { id: rc.id, name: rc.name, enabled, priority: rc.priority, run: rc.run };
	if (rc.clock) body.clock = rc.clock;
	if (rc.intervalMin) body.intervalMin = rc.intervalMin;
	return body;
}

/** O que o servidor diz quando a leitura do vigia falta, em PT-BR; o original vai para o title. */
export function watchText(err: string | undefined): string | null {
	if (!err) return null;
	if (err.startsWith('no agent connected'))
		return 'sem agent ligado: recebe as rotinas quando ligar';
	if (err.startsWith('the agent does not run recurring'))
		return 'o agent desta conta é antigo: atualize o chronicle na máquina do jogo';
	if (err.startsWith('the agent did not answer')) return 'o agent não respondeu';
	if (err.startsWith('saved; the agent did not take it'))
		return 'salva; o agent recebe quando ligar de novo';
	return 'sem a leitura do agent';
}

/** A linha do vigia de uma conta: por que nada começou, os relógios sem leitura ou a última olhada. */
export function watcherLine(status: RecurringStatus | null, now: Date): string | null {
	if (!status) return null;
	const away = watchText(status.watchError);
	if (away) return away;
	if (status.waiting) return status.waiting;
	if (status.clockError)
		return status.clockError.includes('Time Tracking plugin is off')
			? 'o Time Tracking está desligado no cliente'
			: 'sem leitura dos relógios';
	if (status.last) {
		const at = shortWhen(status.last.at, now);
		return status.last.runId
			? `${at} · ${status.last.name} começou`
			: `${at} · ${status.last.name} não começou`;
	}
	return status.checkedAt ? `olhou às ${shortWhen(status.checkedAt, now)}` : null;
}

/** Os runs de uma rotina, o mais novo primeiro. */
export function runsOfRow(row: Pick<RoutineRow, 'kind' | 'id' | 'event'>, runs: Run[]): Run[] {
	return runs.filter((r) =>
		row.kind === 'recurring' ? r.routine === row.id : r.trigger === row.event
	);
}

/**
 * Uma rotina da frota (decisoes-canvas.md, 7.1, decisão 5: quando + o que faz + se falhar + em quais
 * contas vale). O servidor guarda uma cópia por conta; a tela junta as cópias pelo nome (único na
 * conta) e os gatilhos pelo evento.
 */
export interface FleetRoutine {
	/** recurring:<nome> ou trigger:<evento>. */
	key: string;
	kind: 'recurring' | 'trigger';
	name: string;
	/** A linha da primeira conta: "quando Bird Houses fica pronto · Birdhouse Runner". */
	line: string;
	clock?: RecurringClock;
	event?: TriggerEvent;
	/** As cópias, na ordem das contas da frota. */
	members: { account: string; row: RoutineRow }[];
	/** Quantas estão ligadas. */
	on: number;
	/** As cópias diferem no relógio, no intervalo, na prioridade ou no run. */
	differs: boolean;
}

export const fleetKey = (r: Pick<RoutineRow, 'kind' | 'name' | 'event'>) =>
	r.kind === 'trigger' ? `trigger:${r.event}` : `recurring:${r.name}`;

/** A configuração que vale para todas as cópias, para comparar. */
function shape(r: RoutineRow): string {
	const rc = r.recurring;
	return JSON.stringify([rc?.clock ?? '', rc?.intervalMin ?? 0, r.priority, r.run]);
}

/**
 * As rotinas da frota a partir das linhas de cada conta: as ligadas primeiro, depois pelo nome (a
 * ordem não pula a cada olhada do vigia; a fila de cada conta fica na seção Fila).
 */
export function fleetRoutines(accounts: { account: string; rows: RoutineRow[] }[]): FleetRoutine[] {
	const by = new Map<string, FleetRoutine>();
	for (const { account, rows } of accounts) {
		for (const row of rows) {
			const key = fleetKey(row);
			let fr = by.get(key);
			if (!fr) {
				fr = {
					key,
					kind: row.kind,
					name: row.name,
					line: row.line,
					clock: row.clock,
					event: row.event,
					members: [],
					on: 0,
					differs: false
				};
				by.set(key, fr);
			} else if (shape(fr.members[0].row) !== shape(row)) {
				fr.differs = true;
			}
			fr.members.push({ account, row });
			if (row.enabled) fr.on++;
		}
	}
	return [...by.values()].sort(
		(a, b) =>
			Number(b.on > 0) - Number(a.on > 0) ||
			Number(a.kind === 'trigger') - Number(b.kind === 'trigger') ||
			a.name.localeCompare(b.name, 'pt-BR')
	);
}

const PICK: Record<RowTone, number> = { held: 0, due: 1, wait: 2, idle: 3, off: 4 };

/**
 * A cópia que a linha da rotina destaca: uma segura (pede o dono), depois a que entra primeiro na
 * fila da conta dela.
 */
export function headline(fr: FleetRoutine): { account: string; row: RoutineRow } | null {
	let best: { account: string; row: RoutineRow } | null = null;
	for (const m of fr.members) {
		if (!best) {
			best = m;
			continue;
		}
		const d = PICK[m.row.tone] - PICK[best.row.tone];
		if (
			d < 0 ||
			(d === 0 &&
				m.row.tone === 'wait' &&
				Date.parse(m.row.nextAt ?? '') < Date.parse(best.row.nextAt ?? ''))
		)
			best = m;
	}
	return best;
}

/** O que a fila de uma conta tem pela frente: a primeira pronta ou com hora. */
export const queueHead = (rows: RoutineRow[]): RoutineRow | null =>
	rows.find((r) => r.tone === 'due' || r.tone === 'wait') ?? null;

/** O formulário da rotina aberta. */
export interface Draft {
	kind: 'clock' | 'interval' | 'event';
	/** As contas em que a rotina vale. */
	accounts: string[];
	/** O id da cópia de cada conta que já tem a recorrente. */
	ids: Record<string, string>;
	name: string;
	clock: RecurringClock | '';
	/** Minutos; vazio com relógio quer dizer só o relógio. */
	interval: number | null;
	priority: number;
	event: TriggerEvent;
	className: string;
	stop: string[];
	/** Volta ao lugar depois (o inverso de `stay`). */
	back: boolean;
	/** Limite do plugin em minutos; vazio é o padrão do agent (30 min). */
	timeout: number | null;
	/**
	 * A chave de objetivo que encerra o run (wait_until), quando a rotina tem uma: a tela ainda não
	 * a mostra nem edita (falta o canvas), só a mantém ao salvar de novo.
	 */
	until?: string;
}

export function newDraft(accounts: string[]): Draft {
	return {
		kind: 'clock',
		accounts,
		ids: {},
		name: '',
		clock: 'BIRD_HOUSE',
		interval: null,
		priority: 50,
		event: 'kitten_grown',
		className: '',
		stop: [],
		back: true,
		timeout: null
	};
}

function runFields(
	run: RunParams
): Pick<Draft, 'className' | 'stop' | 'back' | 'timeout' | 'until'> {
	return {
		className: run.className,
		stop: [...(run.stop ?? [])],
		back: !run.stay,
		timeout: run.timeoutS ? Math.round(run.timeoutS / 60) : null,
		...(run.until ? { until: run.until.objective } : {})
	};
}

/** Uma rotina da frota de volta no formulário, pela cópia da primeira conta. */
export function draftOf(fr: FleetRoutine): Draft {
	const base = newDraft(fr.members.map((m) => m.account));
	const lead = fr.members[0].row;
	if (fr.kind === 'trigger' && lead.trigger) {
		return {
			...base,
			kind: 'event',
			event: lead.trigger.event,
			name: fr.name,
			...runFields(lead.run)
		};
	}
	const rc = lead.recurring as Recurring;
	const ids: Record<string, string> = {};
	for (const m of fr.members) if (m.row.id) ids[m.account] = m.row.id;
	return {
		...base,
		kind: rc.clock ? 'clock' : 'interval',
		ids,
		name: rc.name,
		clock: rc.clock ?? '',
		interval: rc.intervalMin ?? null,
		priority: rc.priority,
		...runFields(rc.run)
	};
}

function runOf(d: Draft): RunParams {
	const run: RunParams = { className: d.className };
	const stop = d.stop.filter((c) => c !== d.className);
	if (stop.length) run.stop = stop;
	if (d.timeout) run.timeoutS = d.timeout * 60;
	if (!d.back) run.stay = true;
	// The account is the server's to write (from each account's request).
	if (d.until) run.until = { objective: d.until };
	return run;
}

/** O que falta para salvar, como a tela diz; null quando dá. */
export function draftProblem(d: Draft): string | null {
	const name = d.name.trim();
	if (!d.accounts.length) return 'Escolha ao menos uma conta.';
	if (d.kind !== 'event') {
		// eslint-disable-next-line no-control-regex -- the server refuses control characters
		if (!name || name.length > LIMITS.nameMax || /[\u0000-\u001f\u007f]/.test(name))
			return `Dê um nome de 1 a ${LIMITS.nameMax} letras.`;
		if (d.kind === 'clock' && !d.clock) return 'Escolha o relógio.';
		if (d.kind === 'interval' && !d.interval) return 'Diga de quanto em quanto tempo.';
		if (
			d.interval !== null &&
			(!Number.isInteger(d.interval) ||
				d.interval < LIMITS.intervalMin ||
				d.interval > LIMITS.intervalMax)
		)
			return `O intervalo vai de ${LIMITS.intervalMin} min a ${intervalText(LIMITS.intervalMax)}.`;
		if (!Number.isInteger(d.priority) || d.priority < 0 || d.priority > LIMITS.priorityMax)
			return `A prioridade vai de 0 a ${LIMITS.priorityMax}.`;
	}
	if (!d.className) return 'Escolha o plugin que a rotina roda.';
	if (d.stop.filter((c) => c !== d.className).length > LIMITS.stopMax)
		return `Pare no máximo ${LIMITS.stopMax} plugins.`;
	// A run that waits for an objective may take hours (the agent's 12 h).
	const timeoutMax = d.until ? LIMITS.timeoutUntilMax : LIMITS.timeoutMax;
	if (
		d.timeout !== null &&
		(!Number.isInteger(d.timeout) || d.timeout < LIMITS.timeoutMin || d.timeout > timeoutMax)
	)
		return `O limite vai de ${LIMITS.timeoutMin} a ${timeoutMax} min.`;
	return null;
}

/**
 * O corpo do POST /recurring de uma conta. `enabled` é o da cópia que já existe: salvar nunca liga
 * nada, e a cópia numa conta nova nasce desligada (ligar é o interruptor, com o segundo clique).
 */
export function recurringBody(d: Draft, account: string, enabled = false): Recurring {
	const rc: Recurring = {
		name: d.name.trim(),
		enabled,
		priority: d.priority,
		run: runOf(d)
	};
	const id = d.ids[account];
	if (id) rc.id = id;
	if (d.kind === 'clock' && d.clock) rc.clock = d.clock;
	if (d.interval) rc.intervalMin = d.interval;
	return rc;
}

/** O corpo do POST /triggers de uma conta, com o `enabled` da cópia que já existe. */
export function triggerBody(d: Draft, enabled = false): Trigger {
	return { event: d.event, enabled, run: runOf(d) };
}

/** O que salvar a rotina aberta faz em cada conta. */
export interface SavePlan {
	puts: { account: string; recurring?: Recurring; trigger?: Trigger }[];
	/** As cópias recorrentes que saem (a conta foi desmarcada). */
	removes: { account: string; id: string }[];
	/** As contas em que salvar solta uma trava com algo ligado: pode começar logo. */
	releases: string[];
}

/**
 * O plano de salvar: grava em cada conta marcada (com o id e o `enabled` da cópia dela), exclui a
 * cópia das desmarcadas e diz em quais contas uma trava se solta. `heldAgent` são as contas em que o
 * último run da conta segura as rotinas (qualquer salvamento ali solta).
 */
export function savePlan(d: Draft, fr: FleetRoutine | null, heldAgent: Set<string>): SavePlan {
	const member = (account: string) => fr?.members.find((m) => m.account === account)?.row;
	const plan: SavePlan = { puts: [], removes: [], releases: [] };
	for (const account of d.accounts) {
		const row = member(account);
		const enabled = row?.enabled ?? false;
		if (d.kind === 'event') plan.puts.push({ account, trigger: triggerBody(d, enabled) });
		else plan.puts.push({ account, recurring: recurringBody(d, account, enabled) });
		if ((row?.held === 'held' && enabled) || (d.kind !== 'event' && heldAgent.has(account)))
			plan.releases.push(account);
	}
	if (fr?.kind === 'recurring') {
		for (const m of fr.members) {
			if (!d.accounts.includes(m.account) && m.row.id)
				plan.removes.push({ account: m.account, id: m.row.id });
		}
	}
	return plan;
}

/** Os runs de uma rotina da frota em todas as contas dela, o mais novo primeiro. */
export function fleetRuns(
	fr: FleetRoutine,
	runsOf: (account: string) => Run[]
): { account: string; run: Run }[] {
	return fr.members
		.flatMap((m) => runsOfRow(m.row, runsOf(m.account)).map((run) => ({ account: m.account, run })))
		.sort((a, b) => Date.parse(b.run.startedAt) - Date.parse(a.run.startedAt));
}

/** Os passos do "Faz" do artboard: parar, rodar, voltar e religar. */
export function doSteps(d: Draft, names?: Map<string, string>): string[] {
	const stop = d.stop.filter((c) => c !== d.className).map((c) => pluginLabel(c, names));
	const steps: string[] = [];
	steps.push(stop.length ? `Para ${list(stop)} num ponto seguro` : 'Espera um ponto seguro');
	steps.push(`Roda ${d.className ? pluginLabel(d.className, names) : '…'} até terminar`);
	if (d.back) steps.push('Volta ao lugar de antes');
	if (stop.length) steps.push(`Religa ${list(stop)}`);
	return steps;
}

function list(xs: string[]): string {
	return xs.length <= 1 ? (xs[0] ?? '') : `${xs.slice(0, -1).join(', ')} e ${xs[xs.length - 1]}`;
}

/** Uma linha das "Últimas" da rotina aberta: quando, o que deu e o resultado. */
export function runLine(
	r: Run,
	now: Date
): { at: string; text: string; result: string; tone: 'ok' | 'bad' | 'go' } {
	const final = r.state === 'done' || r.state === 'failed' || r.state === 'canceled';
	const tone = !final ? 'go' : r.state === 'done' ? 'ok' : 'bad';
	const text = final
		? [runDuration(r), runOutcome(r), r.error].filter(Boolean).join(' · ')
		: (r.events[r.events.length - 1]?.text ?? RUN_LABEL[r.state]);
	return { at: shortWhen(r.startedAt, now), text, result: RUN_LABEL[r.state], tone };
}

/** Um item da Fila da conta (V7Conta1440): a hora, o que é e a linha de baixo. */
export interface QueueItem {
	key: string;
	/** "agora", "a seguir", "14:32", "amanhã", "no evento", "segura". */
	at: string;
	title: string;
	sub: string;
	clock?: RecurringClock;
	glyph: 'clock' | 'eye' | 'pause' | 'play' | 'alert';
	/** O que acontece agora: o anel de ouro. */
	cur: boolean;
	/** A rotina da frota que o item abre na tela Rotinas. */
	routine?: string;
	/** Para ordenar os itens com hora. */
	time?: number;
}

/** "14:32" hoje, "amanhã 08:10", "16/11". */
function clockAt(t: number, now: Date): string {
	const at = new Date(t);
	const days = Math.round((dayOf(at) - dayOf(now)) / 86_400_000);
	if (days <= 0) return hm(at);
	if (days === 1) return `amanhã ${hm(at)}`;
	return at.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' });
}

/** Quanto o último run feito da rotina levou: "~4 min". */
function typical(row: RoutineRow, runs: Run[]): string | null {
	const done = runsOfRow(row, runs).find((r) => r.state === 'done' && r.finishedAt);
	const d = done ? runDuration(done) : null;
	return d ? `~${d}` : null;
}

const minutes = (s: number) => Math.max(1, Math.round(s / 60));

/**
 * A Fila de uma conta: o que acontece agora (o run em andamento ou a pausa do Break Handler), as
 * rotinas prontas, as que têm hora e a próxima pausa na ordem da hora, os gatilhos e por fim as
 * seguras. A atividade principal do artboard não existe no servidor e fica de fora.
 */
export function accountQueue(
	rows: RoutineRow[],
	runs: Run[],
	brk: BreakState | undefined,
	now: Date,
	names?: Map<string, string>
): QueueItem[] {
	const out: QueueItem[] = [];
	const going = runs[0] && !['done', 'failed', 'canceled'].includes(runs[0].state) ? runs[0] : null;
	if (going) {
		const row = rows.find((r) =>
			going.routine ? r.id === going.routine : !!going.trigger && r.event === going.trigger
		);
		out.push({
			key: `run:${going.uid ?? going.id}`,
			at: 'agora',
			title: row?.name ?? pluginLabel(going.className, names),
			sub: [row ? pluginLabel(going.className, names) : '', RUN_LABEL[going.state]]
				.filter(Boolean)
				.join(' · '),
			clock: row?.clock,
			glyph: 'play',
			cur: true,
			routine: row ? fleetKey(row) : undefined
		});
	}
	if (brk?.active) {
		out.push({
			key: 'break:now',
			at: 'agora',
			title: 'Pausa',
			sub: brk.left_s ? `Break Handler · volta em ${minutes(brk.left_s)} min` : 'Break Handler',
			glyph: 'pause',
			cur: !going
		});
	}
	const kind = (r: RoutineRow) =>
		['rotina', r.recurring?.clock ? 'pelo relógio' : 'pelo intervalo', typical(r, runs)]
			.filter(Boolean)
			.join(' · ');
	for (const r of rows.filter((x) => x.tone === 'due')) {
		out.push({
			key: r.key,
			at: 'a seguir',
			title: r.name,
			sub: kind(r),
			clock: r.clock,
			glyph: 'clock',
			cur: false,
			routine: fleetKey(r)
		});
	}
	const timed: QueueItem[] = rows
		.filter((x) => x.tone === 'wait' && x.nextAt)
		.map((r) => {
			const time = Date.parse(r.nextAt as string);
			return {
				key: r.key,
				at: clockAt(time, now),
				title: r.name,
				sub: kind(r),
				clock: r.clock,
				glyph: 'clock' as const,
				cur: false,
				routine: fleetKey(r),
				time
			};
		});
	if (brk && !brk.active && brk.in_s !== undefined) {
		const time = now.getTime() + brk.in_s * 1000;
		timed.push({
			key: 'break:next',
			at: clockAt(time, now),
			title: 'Pausa',
			sub: 'Break Handler',
			glyph: 'pause',
			cur: false,
			time
		});
	}
	out.push(...timed.sort((a, b) => (a.time ?? 0) - (b.time ?? 0)));
	for (const r of rows.filter((x) => x.kind === 'trigger' && x.enabled)) {
		out.push({
			key: r.key,
			at: 'no evento',
			title: r.name,
			sub: r.line,
			glyph: 'eye',
			cur: false,
			routine: fleetKey(r)
		});
	}
	for (const r of rows.filter((x) => x.tone === 'held')) {
		out.push({
			key: r.key,
			at: 'segura',
			title: r.name,
			sub: 'falhou: espera você salvá-la de novo',
			clock: r.clock,
			glyph: 'alert',
			cur: false,
			routine: fleetKey(r)
		});
	}
	return out;
}
