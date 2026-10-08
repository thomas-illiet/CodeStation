/** Unit tests for package identifier path-safety constraints. */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { safeSegment } from './paths.js';

test('safeSegment accepts canonical identifiers', () => {
  assert.equal(safeSegment('hello-app'), 'hello-app');
});

test('safeSegment rejects traversal and mixed case', () => {
  assert.throws(() => safeSegment('../hello'));
  assert.throws(() => safeSegment('Hello'));
});
