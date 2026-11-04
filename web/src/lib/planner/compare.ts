import type { Band, SectionVM, StepVM } from './types.ts';

export const stepKey = (guideId: string, localId: string): string => `${guideId}::${localId}`;

export function flattenSteps(sections: readonly SectionVM[]): StepVM[] {
	return sections.flatMap((s) => s.steps);
}

/** Longest common subsequence de marcos: mantém a ordem e evita conectores cruzados. */
export function sharedMilestones(left: readonly StepVM[], right: readonly StepVM[]): string[] {
	const a = left.flatMap((s) => (s.milestone ? [s.milestone] : []));
	const b = right.flatMap((s) => (s.milestone ? [s.milestone] : []));
	const dp: number[][] = Array.from({ length: a.length + 1 }, () =>
		new Array<number>(b.length + 1).fill(0)
	);
	for (let i = a.length - 1; i >= 0; i--) {
		for (let j = b.length - 1; j >= 0; j--) {
			dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
		}
	}
	const out: string[] = [];
	let i = 0;
	let j = 0;
	while (i < a.length && j < b.length) {
		if (a[i] === b[j]) {
			out.push(a[i]);
			i++;
			j++;
		} else if (dp[i + 1][j] >= dp[i][j + 1]) i++;
		else j++;
	}
	return out;
}

/**
 * Divide duas rotas em bandas. Cada banda começa num marco compartilhado (o único ponto
 * ligado por conector); entre marcos, as duas faixas são independentes.
 */
export function buildBands(left: readonly StepVM[], right: readonly StepVM[]): Band[] {
	const shared = new Set(sharedMilestones(left, right));
	const bands: Band[] = [];
	const open = (milestone: string | null): Band => {
		const b: Band = { milestone, leftAnchor: null, rightAnchor: null, left: [], right: [] };
		bands.push(b);
		return b;
	};
	const fill = (side: 'left' | 'right', steps: readonly StepVM[]) => {
		const anchor = side === 'left' ? 'leftAnchor' : 'rightAnchor';
		let current: Band | null = null;
		let idx = 0;
		const order = [...shared];
		for (const s of steps) {
			if (s.milestone && shared.has(s.milestone) && s.milestone === order[idx]) {
				idx++;
				current = bandFor(s.milestone);
				current[anchor] = s;
				continue;
			}
			current ??= bandFor(null);
			current[side].push(s);
		}
	};
	const byMilestone = new Map<string | null, Band>();
	function bandFor(m: string | null): Band {
		let b = byMilestone.get(m);
		if (!b) {
			b = open(m);
			byMilestone.set(m, b);
		}
		return b;
	}
	// Garante a ordem das bandas: inicial, depois cada marco na ordem do LCS.
	bandFor(null);
	for (const m of shared) bandFor(m);
	fill('left', left);
	fill('right', right);
	return bands.filter((b) => b.milestone !== null || b.left.length || b.right.length);
}

/** Janela visível para listas virtualizadas de altura de linha fixa. */
export function visibleWindow(opts: {
	count: number;
	rowHeight: number;
	scrollTop: number;
	viewport: number;
	overscan?: number;
}): { start: number; end: number; padTop: number; padBottom: number } {
	const { count, rowHeight, scrollTop, viewport, overscan = 6 } = opts;
	const first = Math.floor(Math.max(0, scrollTop) / rowHeight);
	const start = Math.max(0, first - overscan);
	const end = Math.min(count, Math.ceil((scrollTop + viewport) / rowHeight) + overscan);
	return {
		start,
		end,
		padTop: start * rowHeight,
		padBottom: Math.max(0, (count - end) * rowHeight)
	};
}
