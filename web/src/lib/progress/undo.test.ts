import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Board } from './types.ts';
import { boardIn, popUndo, restoreAction, undoable, type UndoEntry } from './undo.ts';

const oziris = { account: 'acc_x', guide: 'oziris-main' };
const bruh = { account: 'acc_x', guide: 'bruhsailer-main' };
const other = { account: 'acc_y', guide: 'oziris-main' };

const mark = (scope: typeof oziris, step: string): UndoEntry => ({
	kind: 'mark',
	scope,
	step,
	ref: step
});

test('popUndo devolve a última entrada do escopo atual', () => {
	const stack = [mark(oziris, '1.1.1'), mark(oziris, '1.1.2')];
	const { entry, rest } = popUndo(stack, oziris);
	assert.equal(entry?.kind === 'mark' && entry.step, '1.1.2');
	assert.deepEqual(rest, [stack[0]]);
});

test('popUndo nunca desfaz na rota ou conta errada', () => {
	// O mesmo id existe nas duas rotas: a entrada do Oziris não pode virar desfazer no BRUHsailer.
	const stack = [mark(oziris, '1.1.5'), mark(other, '1.1.5')];
	assert.deepEqual(popUndo(stack, bruh), { entry: null, rest: [] });
	const { entry, rest } = popUndo(stack, oziris);
	assert.equal(entry?.scope, oziris);
	assert.deepEqual(rest, []);
	assert.deepEqual(popUndo(stack, null), { entry: null, rest: [] });
});

test('undoable conta só o escopo atual', () => {
	const stack = [
		mark(oziris, 'a'),
		mark(bruh, 'b'),
		{ kind: 'batch', scope: oziris, restore: [] } as UndoEntry
	];
	assert.equal(undoable(stack, oziris), 2);
	assert.equal(undoable(stack, bruh), 1);
	assert.equal(undoable(stack, other), 0);
});

test('boardIn recusa o board de outra rota ou conta', () => {
	const board = { guide: 'oziris-main', account: 'acc_x' } as Board;
	assert.equal(boardIn(board, oziris), board);
	assert.equal(boardIn(board, bruh), null);
	assert.equal(boardIn(board, other), null);
	assert.equal(boardIn(null, oziris), null);
	assert.equal(boardIn(board, null), null);
});

test('restoreAction volta à marca anterior, e implied ou nenhuma viram clear', () => {
	assert.deepEqual(restoreAction(undefined), { kind: 'clear' });
	assert.deepEqual(restoreAction({ kind: 'implied', implied_by_step_id: 'x' }), { kind: 'clear' });
	assert.deepEqual(restoreAction({ kind: 'skipped', note: 'depois' }), {
		kind: 'skipped',
		note: 'depois'
	});
});
