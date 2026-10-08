import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("sidebar loading state can be hidden after packages load", async () => {
  const source = await readFile("src/features/home/provider.ts", "utf8");
  assert.match(source, /\.loading\[hidden\]\{display:none\}/);
});

test("sidebar CSP permits registry icons embedded as data URLs", async () => {
  const source = await readFile("src/features/home/provider.ts", "utf8");
  assert.match(source, /img-src data:/);
});

test("package search does not render a sort dropdown", async () => {
  const source = await readFile("src/features/home/provider.ts", "utf8");

  assert.doesNotMatch(source, /id=\"package-sort\"/);
  assert.doesNotMatch(source, /getElementById\('package-sort'\)/);
});

test("tools section groups environment and proxy actions", async () => {
  const source = await readFile("src/features/home/provider.ts", "utf8");
  assert.match(source, /data-target="tools"/);
  assert.doesNotMatch(source, /data-target="environment"/);
  assert.doesNotMatch(source, /data-target="proxy"/);
  assert.match(source, /data-tool="codestation\.workspaceStorageUsage"/);
  assert.match(source, /data-tool="codestation\.testHttpProxy"/);
  assert.match(source, /data-tool="codestation\.testPort"/);
  assert.match(source, /data-tool="codestation\.reloadWindow"/);
  assert.match(source, /<span>HTTP proxy<\/span><span class="count">3<\/span>/);
});

test("home cards keep a compact fixed height", async () => {
  const source = await readFile("src/features/home/provider.ts", "utf8");

  assert.match(source, /\.card\{position:relative;height:68px;min-height:68px\}/);
  assert.match(source, /\.about-card\{height:44px;min-height:44px/);
});
