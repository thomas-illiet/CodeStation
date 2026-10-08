/** Unit tests for About this Code Station inventory and export helpers. */
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  buildMachineReport,
  collectDevelopmentDependency,
  collectPythonPackages,
  parsePythonPackages,
  parseSystemPackages,
  type AboutSnapshot,
  type CommandRunner
} from "./aboutData.js";

test("parse and sort dpkg packages", () => {
  assert.deepEqual(parseSystemPackages("zlib\t1.3\tamd64\napt\t2.6\tamd64\n"), [
    { name: "apt", version: "2.6", architecture: "amd64" },
    { name: "zlib", version: "1.3", architecture: "amd64" }
  ]);
  assert.throws(() => parseSystemPackages("invalid-row"), /invalid package row/);
});

test("parse, validate and sort pip JSON", () => {
  assert.deepEqual(parsePythonPackages('[{"name":"zeta","version":"2"},{"name":"alpha","version":"1"}]'), [
    { name: "alpha", version: "1" }, { name: "zeta", version: "2" }
  ]);
  assert.throws(() => parsePythonPackages("not-json"), /invalid JSON/);
  assert.throws(() => parsePythonPackages('[{"name":"missing-version"}]'), /invalid package entry/);
});

test("collect a runtime path and normalized version", async () => {
  const runner: CommandRunner = async (command) => command === "which"
    ? { stdout: "/usr/bin/python3\n", stderr: "" }
    : { stdout: "Python 3.12.7\n", stderr: "" };
  assert.deepEqual(await collectDevelopmentDependency("python", runner), {
    id: "python", name: "Python", path: "/usr/bin/python3", version: "3.12.7"
  });
});

test("fall back from python3 to python for runtime and packages", async () => {
  const calls: string[] = [];
  const runner: CommandRunner = async (command, args) => {
    calls.push([command, ...args].join(" "));
    if (command === "which" && args[0] === "python3") throw new Error("missing");
    if (command === "which") return { stdout: "/usr/bin/python\n", stderr: "" };
    if (args.includes("--version")) return { stdout: "Python 3.11.9\n", stderr: "" };
    return { stdout: '[{"name":"pip","version":"25.0"}]', stderr: "" };
  };
  assert.equal((await collectDevelopmentDependency("python", runner)).path, "/usr/bin/python");
  assert.deepEqual(await collectPythonPackages(runner), [{ name: "pip", version: "25.0" }]);
  assert.ok(calls.includes("which python3"));
  assert.ok(calls.includes("which python"));
});

test("machine report keeps full inventories without identity fields", () => {
  const snapshot: AboutSnapshot = {
    development: { collectedAt: "2026-10-04T00:00:00.000Z", data: [{ id: "node", name: "Node.js", path: "/usr/bin/node", version: "22.0.0" }] },
    system: { data: [{ name: "apt", version: "2.6", architecture: "amd64" }] },
    python: { data: [{ name: "pip", version: "25.0" }] },
    codestation: { data: [{ namespace: "core", name: "jq", version: "1.0.0", status: "installed", installedAt: "2026-10-01", updatedAt: "2026-10-02", lastOperationId: "operation-1" }] }
  };
  const report = buildMachineReport(snapshot, "1.2.3", "4.140.0") as Record<string, unknown>;
  const serialized = JSON.stringify(report);
  assert.match(serialized, /core/);
  assert.match(serialized, /1\.0\.0/);
  assert.doesNotMatch(serialized, /operation-1/);
  assert.doesNotMatch(serialized, /hostname|username|environment/i);
  assert.deepEqual(report.machine, { os: process.platform, architecture: process.arch });
});

test("embedded About webview script is valid JavaScript", async () => {
  const source = await readFile("src/features/resources/aboutPanel.ts", "utf8");
  const marker = "const aboutScript = String.raw`";
  const start = source.indexOf(marker) + marker.length;
  const end = source.indexOf("`;\n\nconst aboutStyles", start);
  assert.ok(start >= marker.length && end > start, "embedded script is present");
  const script = source.slice(start, end);
  assert.doesNotMatch(script, /state\.loading/, "loading state uses the declared Set");
  assert.doesNotThrow(() => new Function(script));
});

test("manual About reloads keep their spinner visible", async () => {
  const source = await readFile("src/features/resources/aboutPanel.ts", "utf8");
  assert.match(source, /MINIMUM_RELOAD_DURATION_MS = 500/);
  assert.match(source, /await finishLoading\("system", startedAt, minimumDurationMs\)/);
  assert.match(source, /loadSystem\(MINIMUM_RELOAD_DURATION_MS\)/);
});
