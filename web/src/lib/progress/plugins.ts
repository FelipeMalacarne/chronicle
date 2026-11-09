/**
 * Automação › Plugins (V6MicrobotPlugins1440, doc 08 fase 3): a lista de plugins a partir do que o
 * agent devolve. O GET /scripts traz todo plugin do Microbot carregado no cliente, com o nome em HTML
 * ("<html>[<font>M</font>] Web Walker"); o GET /plugins?installed=true traz os do Plugin Hub, cujo
 * internalName é o nome simples da classe. Puro, testado em node.
 */
import type {
	HubPlugin,
	PluginSchema,
	PluginsView,
	SchemaItem,
	ScriptDetail
} from '../api/microbot.ts';

export type Source = 'hub' | 'meus' | 'cliente';

export interface PluginRow {
	className: string;
	name: string;
	source: Source;
	active: boolean;
	hub?: HubPlugin;
	/** Versão instalada e a do Hub quando há atualização. */
	version?: string;
	update?: string;
	/** Fora do alcance da web: o caminho de controle (Agent Server, núcleo) e o plugin do planner. */
	protected: boolean;
	tone: 'ok' | 'warn' | 'off';
}

const PROTECTED = new Set([
	'net.runelite.client.plugins.microbot.MicrobotPlugin',
	'net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin',
	'net.runelite.client.plugins.microbot.fm.devbootstrap.DevBootstrapPlugin',
	'net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin'
]);

/** O planner lê este plugin (doc 06); ele não é controlado daqui. */
export const PLANNER_PLUGIN =
	'net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin';

/** "<html>[<b><font>F</font><font>M</font></b>] Tick Fisher" vira "Tick Fisher". */
export function cleanName(raw: string): string {
	const text = raw
		.replace(/<[^>]*>/g, '')
		.replace(/&amp;/g, '&')
		.replace(/&#39;|&apos;/g, "'");
	return text.replace(/^\s*\[[^\]]{1,6}\]\s*/, '').trim() || raw;
}

export const simpleClass = (className: string) => className.split('.').pop() ?? className;

const ORDER: Record<Source, number> = { hub: 0, meus: 1, cliente: 2 };

export function pluginRows(v: PluginsView | null): PluginRow[] {
	if (!v) return [];
	const hub = new Map(v.installed.plugins.map((p) => [p.internalName, p]));
	const rows = v.scripts.scripts.map((s): PluginRow => {
		const h = hub.get(simpleClass(s.className));
		const source: Source = h ? 'hub' : s.className.includes('.microbot.fm.') ? 'meus' : 'cliente';
		const version = h?.installedVersion ?? undefined;
		const update =
			h?.latestVersion && version && h.latestVersion !== version ? h.latestVersion : undefined;
		return {
			className: s.className,
			name: h?.name.trim() || cleanName(s.name),
			source,
			active: s.active,
			hub: h,
			version,
			update,
			protected: PROTECTED.has(s.className),
			tone: update ? 'warn' : s.active ? 'ok' : 'off'
		};
	});
	return rows.sort(
		(a, b) =>
			ORDER[a.source] - ORDER[b.source] ||
			Number(b.active) - Number(a.active) ||
			a.name.localeCompare(b.name, 'pt-BR')
	);
}

/** A linha de baixo de cada plugin na lista. */
export function rowLine(r: PluginRow): string {
	if (r.className === PLANNER_PLUGIN) return 'tracking · somente leitura · não é controlado daqui';
	if (r.protected)
		return `${r.active ? 'rodando' : 'parado'} · núcleo do controle, fica fora daqui`;
	const parts = [r.active ? 'rodando' : 'parado'];
	if (r.update) parts.push('atualização disponível no Hub');
	return parts.join(' · ');
}

/** A versão à direita: "1.2.0 → 1.3.0", "1.4.2", ou a origem. */
export function rowVersion(r: PluginRow): string {
	if (r.update) return `${r.version} → ${r.update}`;
	if (r.version) return r.version;
	return r.source === 'meus' ? 'seu · sideload' : 'do cliente';
}

export function duration(ms: number): string {
	const m = Math.floor(ms / 60000);
	if (m < 1) return 'menos de 1 min';
	if (m < 60) return `${m} min`;
	return `${Math.floor(m / 60)} h ${m % 60} min`;
}

export interface Fact {
	k: string;
	v: string;
}

/** Status, saúde e último resultado do plugin escolhido. */
export function detailFacts(d: ScriptDetail | null): Fact[] {
	if (!d) return [];
	const out: Fact[] = [];
	const st = d.status;
	if (st)
		out.push({
			k: 'Status',
			v: [
				st.active ? 'rodando' : 'parado',
				st.runtimeMs ? `há ${duration(st.runtimeMs)}` : '',
				st.status && st.status !== 'RUNNING' && st.status !== 'STOPPED' ? st.status : ''
			]
				.filter(Boolean)
				.join(' · ')
		});
	const health = healthLine(d.health);
	out.push({ k: 'Saúde', v: health ?? (d.errors?.health ? 'sem script de loop' : '—') });
	const last = d.results?.results?.at(-1);
	out.push({
		k: 'Último resultado',
		v: last
			? Object.entries(last)
					.filter(([k]) => k !== 'timestamp' && k !== 'className')
					.slice(0, 4)
					.map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
					.join(' · ') || 'vazio'
			: 'nenhum'
	});
	return out;
}

interface Health {
	status?: string;
	loopCount?: number;
	stalledMs?: number;
	lastError?: { message?: string; recentCount?: number };
}

function healthLine(h: unknown): string | null {
	const list: Health[] = Array.isArray(h)
		? (h as Health[])
		: h && typeof h === 'object'
			? Array.isArray((h as { scripts?: unknown }).scripts)
				? (h as { scripts: Health[] }).scripts
				: [h as Health]
			: [];
	if (!list.length) return null;
	return list
		.map((x) => {
			const bits = [
				x.status === 'NO_HEARTBEAT' ? 'sem heartbeat' : (x.status ?? 'ok').toLowerCase()
			];
			if (x.loopCount) bits.push(`${x.loopCount} loops`);
			if (x.lastError?.message)
				bits.push(
					`erro: ${x.lastError.message}${x.lastError.recentCount ? ` (${x.lastError.recentCount}×)` : ''}`
				);
			return bits.join(' · ');
		})
		.join('; ');
}

export interface FormSection {
	key: string;
	name: string;
	description?: string;
	closed: boolean;
	items: SchemaItem[];
}

/** As opções visíveis por seção, na ordem do painel; as sem seção abrem a lista como "Geral". */
export function formSections(s: PluginSchema | null): FormSection[] {
	if (!s) return [];
	const visible = s.items.filter((i) => !i.hidden);
	const out: FormSection[] = [];
	const loose = visible.filter((i) => !i.section || !s.sections.some((x) => x.key === i.section));
	if (loose.length) out.push({ key: '', name: 'Geral', closed: false, items: loose });
	for (const sec of s.sections) {
		const items = visible.filter((i) => i.section === sec.key);
		if (items.length)
			out.push({
				key: sec.key,
				name: sec.name,
				description: sec.description || undefined,
				closed: !!sec.closedByDefault,
				items
			});
	}
	return out;
}

/** O valor de uma opção vale para gravar? Números no intervalo, enum entre os valores. */
export function fieldError(item: SchemaItem, raw: string): string | null {
	raw = String(raw ?? '');
	if (item.type === 'int' || item.type === 'double') {
		const n = Number(raw);
		if (raw.trim() === '' || !Number.isFinite(n)) return 'número';
		if (item.type === 'int' && !Number.isInteger(n)) return 'inteiro';
		if (item.min !== undefined && n < item.min) return `mínimo ${item.min}`;
		if (item.max !== undefined && n > item.max) return `máximo ${item.max}`;
	}
	if (item.type === 'enum' && !(item.values ?? []).some((v) => v.value === raw))
		return 'escolha um valor';
	if (item.type === 'boolean' && raw !== 'true' && raw !== 'false') return 'sim ou não';
	return null;
}
