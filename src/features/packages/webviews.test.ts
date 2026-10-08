import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("package catalog renders a namespace column", async () => {
  const source = await readFile("src/features/packages/webviews.ts", "utf8");

  assert.match(source, /class=\"namespace-column\"/);
  assert.match(source, /class=\"namespace-label\">Namespace/);
  assert.match(source, /esc\(p\.namespace\)/);
});
