import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("package sidebar refreshes after every terminal operation event", async () => {
  const source = await readFile("src/features/packages/index.ts", "utf8");
  assert.match(source, /event\.phase === "succeeded" \|\| event\.phase === "failed"/);
  assert.match(source, /void this\.load\(\)/);
  assert.match(source, /this\.operationSubscription\.dispose\(\)/);
});
