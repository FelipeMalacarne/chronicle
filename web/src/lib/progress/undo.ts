import type { Board, Mark, MarkAction, MarkRestore } from './types.ts';

/**
 * Escopo de uma escrita do dono. Ids de passo se repetem entre guias (CLAUDE.md regra 7), então
 * desfazer só vale na mesma conta e na mesma rota em que a marca foi feita.
 */
export interface Scope {
	account: string;
	guide: string;
}

/**
 * Uma entrada do desfazer: uma marca (com a que o passo tinha antes), um lote "marcar antes" ou a
 * marca de um objetivo (`marked`: o que a escrita fez; desfazer faz o contrário).
 */
export type UndoEntry =
	| { kind: 'objective'; scope: Scope; key: string; label: string; marked: boolean }
	| { kind: 'mark'; scope: Scope; step: string; ref: string; prev?: Mark; command?: string }
	| { kind: 'batch'; scope: Scope; restore: MarkRestore[]; command?: string };

export function sameScope(a: Scope | null | undefined, b: Scope | null | undefined): boolean {
	return !!a && !!b && a.account === b.account && a.guide === b.guide;
}

/** O board pertence ao escopo? Um board de outra rota nunca alimenta a tela atual. */
export function boardIn(board: Board | null | undefined, scope: Scope | null): Board | null {
	return board && scope && board.account === scope.account && board.guide === scope.guide
		? board
		: null;
}

/**
 * A próxima entrada a desfazer no escopo atual e a pilha que sobra. Entradas de outro escopo são
 * descartadas: nunca reaplicadas na rota errada.
 */
export function popUndo(
	stack: readonly UndoEntry[],
	scope: Scope | null
): { entry: UndoEntry | null; rest: UndoEntry[] } {
	const mine = stack.filter((e) => sameScope(e.scope, scope));
	const entry = mine.at(-1) ?? null;
	return { entry, rest: entry ? mine.slice(0, -1) : mine };
}

/** Quantas entradas o escopo atual pode desfazer. */
export function undoable(stack: readonly UndoEntry[], scope: Scope | null): number {
	return stack.filter((e) => sameScope(e.scope, scope)).length;
}

/**
 * A ação que devolve um passo à marca anterior. Uma marca implied não volta pela API: desfazer
 * limpa a marca e o motor decide de novo.
 */
export function restoreAction(prev?: Mark): { kind: MarkAction; note?: string } {
	if (!prev || prev.kind === 'implied') return { kind: 'clear' };
	return { kind: prev.kind as MarkAction, note: prev.note };
}

/** O que desfazer uma marca de objetivo escreve: desmarcar o que foi marcado, e o inverso. */
export function objectiveUndoAction(entry: { marked: boolean }): 'mark' | 'unmark' {
	return entry.marked ? 'unmark' : 'mark';
}
