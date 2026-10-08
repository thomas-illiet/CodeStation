import assert from "node:assert/strict";
import test from "node:test";
import { formatBytes } from "./storageFormat.js";

test("formatBytes renders compact binary units", () => {
  assert.equal(formatBytes(0), "0 B");
  assert.equal(formatBytes(1024), "1.00 KiB");
  assert.equal(formatBytes(15 * 1024), "15.0 KiB");
  assert.equal(formatBytes(150 * 1024 * 1024), "150 MiB");
});
