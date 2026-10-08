import assert from "node:assert/strict";
import test from "node:test";
import { processStartId } from "./processIdentity.js";

test("reads a stable process identity on Linux when procfs is available", async () => {
  const first = await processStartId(process.pid);
  const second = await processStartId(process.pid);

  assert.equal(first, second);
  if (process.platform === "linux") assert.match(first ?? "", /^\d+$/);
});
