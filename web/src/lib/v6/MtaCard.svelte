<script lang="ts">
	import { ApiError, progressApi } from '#lib/api/client';
	import { itemIcon } from '#lib/progress/icons';
	import {
		buyNames,
		carriedRunes,
		magicLevel,
		NO_ENCHANT,
		ownedRewards,
		parsePoints,
		readPoints,
		ROOM_IDS,
		RUNE_LABEL,
		runeRows,
		type MtaResult,
		type MtaTable,
		type RoomId
	} from '#lib/progress/mta';
	import type { AccountState } from '#lib/progress/types';
	import { ago } from '#lib/progress/view';

	/*
	 * Mage Training Arena card of Conta › Personagem. Not drawn in the canvas yet (the owner asked
	 * for it ahead of the artboard): it reuses the v7 card, field and plaque-less list patterns.
	 * The arithmetic is the server's (POST /api/mta/calculate). The pizazz points come from Chronicle
	 * Link, which reads them in the arena (the minigame facet, mta.<room>); until it has read every
	 * room they are typed, and a typed value stays editable over a reading.
	 */
	let { state: st }: { state: AccountState } = $props();

	const KEY = 'chronicle.mta';
	let table = $state<MtaTable | null>(null);
	let failure = $state<string | null>(null);
	let picked = $state<string[]>([]);
	let points = $state<Record<RoomId, string>>({
		telekinetic: '',
		graveyard: '',
		enchantment: '',
		alchemist: ''
	});
	let result = $state<MtaResult | null>(null);
	let note = $state<string | null>(null);

	// The choice and the typed points are a per-viewer convenience: storage may be blocked.
	try {
		const raw = JSON.parse(localStorage.getItem(KEY) ?? 'null');
		if (raw && Array.isArray(raw.picked))
			picked = raw.picked.filter((x: unknown) => typeof x === 'string');
		if (raw?.points)
			for (const r of ROOM_IDS) if (typeof raw.points[r] === 'string') points[r] = raw.points[r];
	} catch {
		/* no storage: starts empty */
	}
	$effect(() => {
		const snap = JSON.stringify({ picked, points });
		try {
			localStorage.setItem(KEY, snap);
		} catch {
			/* no storage */
		}
	});

	$effect(() => {
		progressApi
			.mtaRewards()
			.then((t) => (table = t))
			.catch((e) => (failure = e instanceof Error ? e.message : String(e)));
	});

	const read = $derived(readPoints(st));
	$effect(() => {
		if (read) for (const r of ROOM_IDS) points[r] = String(read.points[r]);
	});

	const owned = $derived(table ? ownedRewards(table, st) : []);
	const magic = $derived(magicLevel(st));
	const have = $derived(carriedRunes(st));
	const rooms = $derived(table?.rooms ?? []);
	const cap = (id: RoomId) => rooms.find((r) => r.id === id)?.cap ?? 0;

	// One request per change, the newest answer wins.
	let seq = 0;
	$effect(() => {
		const rewards = picked;
		if (!table || !rewards.length) {
			result = null;
			note = null;
			return;
		}
		const input = {
			rewards,
			owned,
			points: Object.fromEntries(ROOM_IDS.map((r) => [r, parsePoints(points[r], cap(r))])),
			...(magic === undefined ? {} : { magic })
		};
		const mine = ++seq;
		progressApi
			.mtaCalculate(input)
			.then((r) => {
				if (mine !== seq) return;
				result = r;
				note = null;
			})
			.catch((e) => {
				if (mine !== seq) return;
				result = null;
				note = e instanceof ApiError && e.status === 422 ? NO_ENCHANT : String(e.message ?? e);
			});
	});

	function toggle(id: string) {
		picked = picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
	}
	const n = (v: number) => v.toLocaleString('pt-BR');
	const rows = $derived(result ? runeRows(result, have) : []);
	const buy = $derived(table && result ? buyNames(table, result.buy) : []);
	const roomName = (id: RoomId) => rooms.find((r) => r.id === id)?.name ?? id;
</script>

<section class="card mta">
	<div class="h">
		<h2>Mage Training Arena</h2>
		<span>runas das recompensas · OSRS Wiki</span>
	</div>
	<div class="b">
		{#if failure}
			<p class="mu">Sem a tabela de recompensas: {failure}</p>
		{:else if !table}
			<p class="mu" aria-busy="true">Lendo a tabela…</p>
		{:else}
			<div class="cols">
				<div class="rw">
					<p class="lbl">Recompensas</p>
					{#each table.rewards as r (r.id)}
						{@const icon = itemIcon(r.item_id)}
						{@const has = owned.includes(r.id)}
						<label class="rr" class:has>
							<input
								type="checkbox"
								checked={picked.includes(r.id)}
								onchange={() => toggle(r.id)}
							/>
							{#if icon}<img src={icon} alt="" />{:else}<span class="ph"></span>{/if}
							<span>{r.name}</span>
							<span class="w" title="Ela já tem este item (inventário, equipamento ou banco)"
								>{has ? 'já tem' : ''}</span
							>
						</label>
					{/each}
				</div>

				<div class="pt">
					<p class="lbl">Pontos de pizazz que ela tem</p>
					{#each ROOM_IDS as id (id)}
						<label class="pf">
							<span>{roomName(id)}</span>
							<input
								class="mono"
								inputmode="numeric"
								placeholder="0"
								aria-label={`Pontos em ${roomName(id)}`}
								bind:value={points[id]}
							/>
						</label>
					{/each}
					<p class="mu small">
						{magic === undefined ? 'Magic sem leitura.' : `Magic ${magic}`} ·
						{read
							? `lidos no jogo ${ago(read.observedAt, Date.now())}`
							: 'digitados à mão até o Chronicle Link ler a arena'}
					</p>
				</div>
			</div>

			{#if !picked.length}
				<p class="mu">Marque as recompensas que ela quer.</p>
			{:else if note}
				<p class="mu warn">{note}</p>
			{:else if result}
				{#if !result.buy.length}
					<p class="mu">Ela já tem tudo o que marcou.</p>
				{:else}
					<p class="lbl">Comprar, nesta ordem</p>
					<p class="order">{buy.join(' → ')}</p>
					<p class="lbl">Pontos que faltam</p>
					<div class="miss">
						{#each ROOM_IDS as id (id)}
							<span class:zero={!result.missing[id]}
								>{roomName(id)}
								<b class="mono">{n(result.missing[id])}</b></span
							>
						{/each}
					</div>
					<p class="lbl">
						Runas{result.enchant_level ? ` · Enchant nível ${result.enchant_level}` : ''}
					</p>
					<div class="runes">
						{#each rows as r (r.rune)}
							{@const icon = itemIcon(r.rune === 'law' ? 563 : r.rune === 'nature' ? 561 : 564)}
							<div class="rune" class:short={!!r.short}>
								{#if icon}<img src={icon} alt="" />{/if}
								<span>{RUNE_LABEL[r.rune]}</span>
								<b class="mono">{n(r.need)}</b>
								<span class="mu small"
									>{r.have === null
										? 'sem leitura'
										: r.short
											? `faltam ${n(r.short)} (tem ${n(r.have)})`
											: `tem ${n(r.have)}`}</span
								>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		{/if}
	</div>
</section>

<style>
	.cols {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(260px, 100%), 1fr));
		gap: 20px;
		align-items: start;
	}
	.rw,
	.pt {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.rr {
		display: grid;
		grid-template-columns: 18px 24px minmax(0, 1fr) auto;
		gap: 8px;
		align-items: center;
		min-height: 32px;
		cursor: pointer;
	}
	.rr img,
	.ph {
		width: 24px;
		height: 24px;
		object-fit: contain;
	}
	.rr .w {
		font-size: 11px;
		color: var(--done);
	}
	.rr.has > span:nth-of-type(1) {
		color: var(--ink-subtle);
	}
	.pf {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 96px;
		gap: 8px;
		align-items: center;
		min-height: 36px;
	}
	.pf input {
		height: 32px;
		padding: 0 8px;
		text-align: right;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--surface);
		color: var(--ink);
	}
	.small {
		font-size: 12px;
	}
	.warn {
		color: var(--stale);
	}
	.order {
		font-family: Alegreya, Georgia, serif;
		font-size: 15px;
	}
	.miss {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: 6px 16px;
	}
	.miss span {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}
	.miss .zero {
		color: var(--ink-subtle);
	}
	.runes {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.rune {
		display: grid;
		grid-template-columns: 24px 64px 72px minmax(0, 1fr);
		gap: 8px;
		align-items: center;
	}
	.rune img {
		width: 24px;
		height: 24px;
		object-fit: contain;
	}
	.rune b {
		text-align: right;
	}
	.rune.short .mu {
		color: var(--stale);
	}
</style>
