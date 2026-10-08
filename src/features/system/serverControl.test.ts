import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("code-server restart requires explicit confirmation", async () => {
  const source = await readFile("src/features/system/serverControl.ts", "utf8");

  assert.match(source, /showWarningMessage/);
  assert.match(source, /modal: true/);
  assert.match(source, /process\.kill\(1, "SIGTERM"\)/);
  assert.doesNotMatch(source, /CODESTATION_ALLOW_SERVER_RESTART/);
});
