/** Unit tests for pure cgroup parsers and CPU sampling calculations. */
import assert from "node:assert/strict";
import test from "node:test";
import { cpuUsagePercent, normalizeMemoryLimit, parseCpuMax, parseCpuStat } from "./cgroup.js";

test("parse le quota CPU cgroup v2", () => {
  assert.equal(parseCpuMax("200000 100000", 8), 2);
  assert.equal(parseCpuMax("max 100000", 8), 8);
});

test("parse cpu.stat", () => {
  assert.equal(parseCpuStat("usage_usec 12345\nuser_usec 12000"), 12345);
});

test("calcule le pourcentage CPU relativement au quota", () => {
  const previous = { usageMicros: 1_000, timestampMicros: 10_000, limitCores: 2 };
  const current = { usageMicros: 2_000, timestampMicros: 11_000, limitCores: 2 };
  assert.equal(cpuUsagePercent(previous, current), 50);
});

test("reconnaît les limites mémoire illimitées", () => {
  assert.equal(normalizeMemoryLimit("max", 1024), 1024);
  assert.equal(normalizeMemoryLimit("9223372036854771712", 1024), 1024);
  assert.equal(normalizeMemoryLimit("512", 1024), 512);
});
