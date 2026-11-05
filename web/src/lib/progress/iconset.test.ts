import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseRanges } from './iconset.ts';

test('parseRanges expands single ids and ranges, ignoring junk', () => {
	assert.deepEqual([...parseRanges('1-3,7,9-10')], [1, 2, 3, 7, 9, 10]);
	assert.deepEqual([...parseRanges('')], []);
	assert.deepEqual([...parseRanges('5,x,8-6, 12 ')], [5, 12]);
});
