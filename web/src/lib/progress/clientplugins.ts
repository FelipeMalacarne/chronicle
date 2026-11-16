/*
 * Conta › Cliente › cartão Plugins (página 8 do canvas, V7ClientePlugins1440 e
 * V7ClientePluginsEstados1440; seção 13 do brief): os plugins do cliente da conta nos grupos que
 * saem do uso, como em /plugins (o que uma rotina roda é "Rotinas usam", o que o perfil liga no
 * boot é "Sempre ligados", o resto é "Jogam"), cada linha com o estado e o desvio do perfil como
 * selo. Os do próprio cliente ficam escondidos. Lógica pura, testada com node --test.
 */
import type {
	ClientDrift,
	GameView,
	PluginDrift,
	PluginsView,
	RecurringState,
	SettingDrift,
	Trigger
} from '../api/microbot.ts';
import { pluginClass, type ProfilePlugin } from './fleetplugins.ts';
import { isUtility } from './frota.ts';
import { pluginRows, type Source } from './plugins.ts';

export type CardGroup = 'plays' | 'routines' | 'always';

export interface Badge {
	text: string;
	title: string;
}

export interface CardRow {
	className: string;
	name: string;
	source: Source;
	group: CardGroup;
	protected: boolean;
	active: boolean;
	/** A versão que o cliente tem (plugins do Hub). */
	version?: string;
	lamp: 'ok' | 'rest' | 'bad' | 'off';
	/** "fishing · 1.820 voltas · 42 min", "parado · rotina Bird houses às 14:32". */
	line: string;
	tone?: 'bad';
	badges: Badge[];
	/** As opções do grupo de config do plugin que o cliente tem diferentes do perfil. */
	drift: SettingDrift[];
}

export interface CardInput {
	view: PluginsView | null;
	game: GameView | null;
	paused: boolean;
	/** Os plugins do perfil efetivo da conta (GET /api/accounts/client). */
	profile: ProfilePlugin[];
	triggers: Trigger[];
	recurring: RecurringState[];
	drift: ClientDrift | null;
	/** O grupo de config de cada plugin (do esquema que o Chronicle Link exporta), por classe. */
	groups: Record<string, string>;
	/** Desde quando cada plugin roda (o status do Agent Server), por classe. */
	startedAt?: Record<string, number>;
	now?: number;
}

export interface Card {
	plays: CardRow[];
	routines: CardRow[];
	always: CardRow[];
	/** Os do próprio cliente (RuneLite e Microbot), escondidos por padrão. */
	client: CardRow[];
	/** Quantos plugins têm desvio, contando os do perfil que nem estão instalados. */
	withDrift: number;
	/** Plugins do Hub que o perfil instala e o cliente não tem. */
	missing: PluginDrift[];
	/** Plugins do RuneLite que não dá para conferir. */
	unchecked: number;
}

const EVENT: Record<string, string> = { kitten_grown: 'quando o filhote crescer' };

const hhmm = (ms: number) =>
	new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });

function minutes(ms: number): string {
	const m = Math.max(0, Math.floor(ms / 60000));
	if (m < 60) return `${m} min`;
	return `${Math.floor(m / 60)} h ${m % 60} min`;
}

/** O pacote de uma classe: plugin e script do mesmo plugin moram no mesmo pacote. */
const pkg = (className: string) => className.slice(0, className.lastIndexOf('.'));

/** O que a rotina que roda o plugin faz a seguir: "rotina Bird houses às 14:32". */
function routineLine(className: string, triggers: Trigger[], recurring: RecurringState[]): string {
	const rec = recurring
		.filter((r) => r.enabled && r.run.className === className)
		.sort((a, b) => (a.nextAt ?? '~').localeCompare(b.nextAt ?? '~'))[0];
	if (rec)
		return rec.nextAt
			? `rotina ${rec.name} às ${hhmm(Date.parse(rec.nextAt))}`
			: `rotina ${rec.name}`;
	const trg = triggers.find((t) => t.enabled && t.run.className === className);
	if (trg) return `rotina ${EVENT[trg.event] ?? trg.event}`;
	return '';
}

export function clientCard(input: CardInput): Card {
	const { view, game, paused, profile, triggers, recurring, drift, groups } = input;
	const now = input.now ?? Date.now();
	const routine = new Set([
		...triggers.map((t) => t.run.className),
		...recurring.map((r) => r.run.className)
	]);
	const starts = new Map(profile.filter((p) => p.start).map((p) => [pluginClass(p), p]));
	const pinned = new Map(profile.map((p) => [pluginClass(p), p]));
	const beats = game?.observe?.scripts ?? [];
	const machines = game?.machines?.scripts ?? [];
	const card: Card = {
		plays: [],
		routines: [],
		always: [],
		client: [],
		withDrift: 0,
		missing: (drift?.plugins ?? []).filter((d) => d.problem === 'not_installed'),
		unchecked: drift?.unchecked.length ?? 0
	};
	const rows = pluginRows(view);
	const playing = rows.filter((r) => r.active && !r.protected && !isUtility(r.name));

	for (const r of rows) {
		const group: CardGroup = routine.has(r.className)
			? 'routines'
			: starts.has(r.className) || r.protected || isUtility(r.name)
				? 'always'
				: 'plays';
		const beat = beats.find((b) => pkg(b.scriptClassName) === pkg(r.className));
		const machine =
			machines.length === 1 && playing.length === 1 && playing[0] === r ? machines[0] : undefined;
		const stalled = r.active && beat?.status === 'NO_HEARTBEAT';
		let line: string;
		if (r.protected) line = `protegido · ${r.active ? 'ligado' : 'parado'}`;
		else if (!r.active)
			line = ['parado', group === 'routines' ? routineLine(r.className, triggers, recurring) : '']
				.filter(Boolean)
				.join(' · ');
		else if (stalled) line = 'sem heartbeat';
		else if (group === 'always') line = paused ? 'em pausa' : 'ligado';
		else {
			const loops = machine?.loopCount ?? beat?.loopCount;
			const since = input.startedAt?.[r.className];
			line =
				[
					paused ? 'em pausa' : '',
					machine?.currentState ? machine.currentState.toLowerCase().replace(/_/g, ' ') : '',
					loops ? `${loops.toLocaleString('pt-BR')} voltas` : '',
					since !== undefined && !paused ? minutes(now - since) : ''
				]
					.filter(Boolean)
					.join(' · ') || 'ligado';
		}

		const settings = groups[r.className]
			? (drift?.settings ?? []).filter((s) => s.group === groups[r.className])
			: [];
		const plugin = (drift?.plugins ?? []).filter(
			(d) => (d.source === 'hub' ? d.class_name : d.name) === r.className
		);
		const badges: Badge[] = [];
		if (settings.length)
			badges.push({
				text: `${settings.length} fora do perfil`,
				title: `${settings.length === 1 ? 'Uma opção' : `${settings.length} opções`} do cliente diferente${settings.length === 1 ? '' : 's'} do perfil da conta`
			});
		for (const d of plugin) {
			if (d.problem === 'version')
				badges.push({
					text: `${d.version || '?'} ≠ ${pinned.get(r.className)?.version ?? '?'}`,
					title: `O perfil da conta fixa a ${pinned.get(r.className)?.version ?? 'outra versão'}; o cliente roda a ${d.version || 'outra'}`
				});
			if (d.problem === 'not_started')
				badges.push({
					text: 'devia estar ligado',
					title: 'O perfil liga no boot; no cliente está parado'
				});
		}
		if (badges.length) card.withDrift++;

		const row: CardRow = {
			className: r.className,
			name: r.name,
			source: r.source,
			group,
			protected: r.protected,
			active: r.active,
			version: r.version,
			lamp: !r.active ? 'off' : stalled ? 'bad' : paused ? 'rest' : 'ok',
			line,
			tone: stalled ? 'bad' : undefined,
			badges,
			drift: settings
		};
		if (r.source === 'cliente' && group === 'plays' && !badges.length) card.client.push(row);
		else card[group].push(row);
	}
	card.withDrift += card.missing.length;
	// Quem joga primeiro; o resto na ordem de pluginRows (origem e nome).
	card.plays.sort((a, b) => Number(b.active) - Number(a.active));
	return card;
}

const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

/** "Ligar outro instalado": os plugins do cliente com o nome, fora os protegidos, os do cliente por último. */
export function findInstalled(card: Card, q: string): CardRow[] {
	const needle = fold(q.trim());
	if (!needle) return [];
	return [...card.plays, ...card.routines, ...card.always, ...card.client]
		.filter((r) => !r.protected && fold(r.name).includes(needle))
		.sort((a, b) => Number(a.source === 'cliente') - Number(b.source === 'cliente'))
		.slice(0, 8);
}

/** O título do cartão: "2 com desvio", "nada jogando", "em pausa". */
export function cardStatus(card: Card, paused: boolean): { text: string; tone?: 'stale' | 'bad' } {
	const stalled = [...card.plays, ...card.routines].filter((r) => r.lamp === 'bad').length;
	if (stalled) return { text: `${stalled} sem resposta`, tone: 'bad' };
	if (card.withDrift) return { text: `${card.withDrift} com desvio`, tone: 'stale' };
	if (paused) return { text: 'em pausa' };
	if (![...card.plays, ...card.routines].some((r) => r.active)) return { text: 'nada jogando' };
	return { text: '' };
}

/** O que a gaveta diz do plugin no topo: "1.4.0 · Plugin Hub · rodando". */
export function drawerMeta(r: CardRow): string {
	const origin =
		r.source === 'hub' ? 'Plugin Hub' : r.source === 'meus' ? 'deste repositório' : 'do cliente';
	return [r.version ?? '', origin, r.line].filter(Boolean).join(' · ');
}

/** O valor do perfil de cada opção com desvio, por chave (a gaveta mostra "perfil: …"). */
export function profileValues(drift: SettingDrift[]): Record<string, string> {
	return Object.fromEntries(drift.map((d) => [d.key, d.profile]));
}

/** As state machines do snapshot de um plugin numa conta (/debug/snapshot), com o estado atual. */
export function snapshotMachines(
	machines: unknown
): { name: string; state: string; detail: string }[] {
	const list = (machines as { scripts?: unknown } | null)?.scripts;
	if (!Array.isArray(list)) return [];
	return list.map(
		(m: {
			name?: string;
			currentState?: string | null;
			loopCount?: number;
			msInCurrentState?: number;
		}) => ({
			name: m.name ?? '',
			state: m.currentState ?? 'NOT_INITIALIZED',
			detail: [
				m.msInCurrentState !== undefined ? `há ${minutes(m.msInCurrentState)}` : '',
				m.loopCount ? `${m.loopCount.toLocaleString('pt-BR')} voltas` : ''
			]
				.filter(Boolean)
				.join(' · ')
		})
	);
}

/** O último resultado que o plugin publicou, como pares (sem a hora e a classe). */
export function lastResult(
	results: { results?: Record<string, unknown>[] } | undefined
): { k: string; v: string }[] {
	const last = results?.results?.at(-1);
	if (!last) return [];
	return Object.entries(last)
		.filter(([k]) => k !== 'timestamp' && k !== 'className')
		.slice(0, 6)
		.map(([k, v]) => ({
			k,
			v:
				typeof v === 'number'
					? v.toLocaleString('pt-BR')
					: typeof v === 'object'
						? JSON.stringify(v)
						: String(v)
		}));
}
