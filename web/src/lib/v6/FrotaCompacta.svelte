<script lang="ts">
	import { sessionCommand, type CompactGroup, type Lane } from '#lib/progress/frota';
	import { iconFor } from '#lib/progress/icons';
	import type { WorkerActionKind } from '#lib/progress/workers';
	import Glyph from './Glyph.svelte';

	/*
	 * Frota compacta (página 8 do canvas, V7FrotaCompacta1440): uma linha por conta, agrupada em
	 * Precisa de você, Jogando e Paradas. Sem a coluna Ritmo (entra com a V5) e com a rota no lugar
	 * do nível, como nas raias. Uma conta cuja sessão falta ou foi recusada copia o comando que
	 * manda uma nova; Pausar e "mais ações" ficam de fora: Parar e Reiniciar estão nas raias
	 * (docs/ui/decisoes-canvas.md, seção 7.7).
	 */
	let {
		groups,
		armed,
		busy,
		copied,
		onopen,
		onact,
		oncopy
	}: {
		groups: CompactGroup[];
		armed: string | null;
		busy: string | null;
		copied: string | null;
		onopen: (l: Lane, to?: '/visao' | '/automacao') => void;
		onact: (l: Lane, action: WorkerActionKind) => void;
		oncopy: (account: string, command: string) => void;
	} = $props();

	const GUIDANCE = 'Abra o launcher no zaros e envie a sessão';
</script>

<section class="dense" aria-label="Contas">
	<div class="dr hd" aria-hidden="true">
		<span>Conta</span><span>Skill</span><span>Fazendo</span><span>Rota</span><span>Depois</span
		><span>Onde</span><span class="r">Ações</span>
	</div>
	{#each groups as g (g.key)}
		<div class="dg" class:dg-bad={g.key === 'need'}>{g.label}<span>{g.lanes.length}</span></div>
		{#each g.lanes as l (l.id)}
			{@const face = l.face ? iconFor(`skill:${l.face.skill}`) : undefined}
			{@const command = sessionCommand(l)}
			{@const start = !command && l.worker?.actions.includes('start') ? l.worker : null}
			<div class="dr">
				<span class="n">
					<span class="lamp {l.tone}"></span>
					<button type="button" class="nl" onclick={() => onopen(l)}><b>{l.rsn}</b></button>
				</span>
				<span class="sk"
					>{#if l.face}{#if face}<img src={face} alt={l.face.skill} />{/if}<span
							title="{l.face.skill} {l.face.level}">{l.face.level}</span
						>{/if}</span
				>
				<span class="a {l.tone}" title={l.detail}
					>{l.activity}{#if l.detail}<span> · {l.detail}</span>{/if}</span
				>
				<span class="p">
					{#if l.route}
						<span
							class="track"
							class:live={l.tone === 'ok'}
							role="progressbar"
							aria-valuenow={l.route.pct}
							aria-valuemin="0"
							aria-valuemax="100"
							aria-label="{l.route.label}: {l.route.done} de {l.route.total} passos"
							title="{l.route.label} · {l.route.done} de {l.route.total}{l.route.ref
								? ` · ${l.route.ref}`
								: ''}"><span style="width: {l.route.pct}%"></span></span
						>{l.route.pct}%
					{/if}
				</span>
				<span class="nx">
					{#if l.next[0]}
						<span class:rdy={l.next[0].ready}>{l.next[0].label} · {l.next[0].when}</span>
					{:else}
						<span>nenhum relógio</span>
					{/if}
				</span>
				<span class="o"
					>{#if l.where}<span class="tag" title={l.whereTitle}>{l.where}</span>{/if}</span
				>
				<span class="ac">
					{#if command}
						<button
							type="button"
							class="btn btn-s btn-p"
							title="{GUIDANCE}: {command}"
							onclick={() => oncopy(l.id, command)}
							>{copied === l.id ? 'Copiado' : 'Copiar comando'}</button
						>
					{:else if l.client}
						<button
							type="button"
							class="ib"
							aria-label="Ver o jogo de {l.rsn}"
							title="Ver o jogo"
							onclick={() => onopen(l, '/automacao')}><Glyph name="eye" size={13} /></button
						>
					{:else if start}
						{#if armed === `${l.id}:start`}
							<button
								type="button"
								class="btn btn-s"
								disabled={!!busy}
								onclick={() => onact(l, 'start')}>Confirmar</button
							>
						{:else}
							<button
								type="button"
								class="ib"
								aria-label="Iniciar {l.rsn}"
								title={start.blocked ?? 'Sobe o pod; o cliente para na tela de login'}
								disabled={!!busy || !!start.blocked}
								onclick={() => onact(l, 'start')}
								>{#if busy === `${l.id}:start`}…{:else}<Glyph name="play" size={13} />{/if}</button
							>
						{/if}
					{/if}
				</span>
			</div>
		{/each}
	{:else}
		<p class="none mu">Nenhuma conta com esse nome.</p>
	{/each}
</section>

<style>
	.dense {
		display: flex;
		flex-direction: column;
		border-radius: 6px;
		background: var(--surface);
		border: 1px solid var(--border);
		box-shadow: var(--bevel);
		color: var(--ink);
	}
	.dr {
		display: grid;
		grid-template-columns:
			150px 54px minmax(0, 1.5fr) minmax(0, 0.9fr) minmax(0, 1fr)
			84px 120px;
		gap: 12px;
		align-items: center;
		min-height: 46px;
		padding: 0 14px;
		border-top: 1px solid var(--hairline);
		font-size: 13px;
	}
	.dr:hover {
		background: var(--elevated);
	}
	.dr.hd {
		min-height: 30px;
		border-top: 0;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-subtle);
	}
	.dr.hd:hover {
		background: transparent;
	}
	.r {
		text-align: right;
	}
	.n {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.nl {
		display: flex;
		min-width: 0;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--ink);
		font: inherit;
		cursor: pointer;
	}
	.nl b {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nl:hover b {
		color: var(--gold-ink);
	}
	.lamp {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: 0 0 auto;
		box-sizing: border-box;
		background: var(--ink-subtle);
	}
	.lamp.ok {
		background: var(--done);
		box-shadow: 0 0 0 3px var(--done-tint);
	}
	.lamp.up {
		background: var(--auto);
	}
	.lamp.bad {
		background: var(--not-started);
	}
	.lamp.off {
		background: transparent;
		border: 1.5px solid var(--ink-subtle);
	}
	.sk {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font:
			600 12px 'JetBrains Mono',
			monospace;
	}
	.sk img {
		width: 20px;
		height: 20px;
		image-rendering: pixelated;
		object-fit: contain;
	}
	.a {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.a span {
		font-size: 12px;
		font-weight: 400;
		color: var(--ink-muted);
	}
	.a.bad {
		color: var(--not-started);
		font-weight: 600;
	}
	.a.up {
		color: var(--auto);
		font-weight: 600;
	}
	.p {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 30px;
		gap: 6px;
		align-items: center;
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.track {
		display: block;
		height: 8px;
		border-radius: 3px;
		background: var(--frame-2);
		box-shadow: var(--bevel-frame);
		overflow: hidden;
	}
	.track span {
		display: block;
		height: 100%;
		background-color: var(--done);
	}
	.track.live span {
		background-image: linear-gradient(
			110deg,
			rgba(255, 255, 255, 0.16) 0 25%,
			transparent 25% 50%,
			rgba(255, 255, 255, 0.16) 50% 75%,
			transparent 75%
		);
		background-size: 20px 8px;
		animation: flow 1.1s linear infinite;
	}
	@keyframes flow {
		from {
			background-position: 0 0;
		}
		to {
			background-position: 20px 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.track span {
			animation: none !important;
		}
	}
	.nx {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
		font-size: 12px;
		color: var(--ink-muted);
	}
	.nx span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nx .rdy {
		color: var(--done);
		font-weight: 600;
	}
	.tag {
		display: inline-flex;
		align-items: center;
		height: 18px;
		padding: 0 6px;
		border: 1px solid var(--hairline);
		border-radius: 3px;
		font:
			500 10px 'JetBrains Mono',
			monospace;
		color: var(--ink-muted);
		white-space: nowrap;
	}
	.ac {
		display: flex;
		gap: 4px;
		justify-content: flex-end;
	}
	.ac .btn {
		height: 28px;
		padding: 0 8px;
	}
	.ib {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		padding: 0;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--elevated);
		box-shadow: var(--bevel);
		color: var(--ink);
		cursor: pointer;
	}
	.ib:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.dg {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 32px;
		padding: 0 14px;
		border-top: 1px solid var(--border);
		background: var(--elevated);
		font:
			600 12px Inter,
			system-ui,
			sans-serif;
	}
	.dg span {
		font:
			400 11px 'JetBrains Mono',
			monospace;
		color: var(--ink-subtle);
	}
	.dg-bad {
		color: var(--not-started);
	}
	.none {
		margin: 0;
		padding: 14px;
		border-top: 1px solid var(--hairline);
		font-size: 13px;
	}
	@media (max-width: 900px) {
		.dr {
			grid-template-columns: minmax(0, 1fr) 54px auto;
			padding-block: 8px;
		}
		.dr.hd,
		.p,
		.nx,
		.o {
			display: none;
		}
		.a {
			grid-column: 1 / -1;
			order: 5;
		}
	}
</style>
