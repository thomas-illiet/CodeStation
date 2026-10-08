import assert from "node:assert/strict";
import test from "node:test";
import { renderPackageMarkdown } from "./markdown.js";

test("package README supports common Markdown formatting", () => {
  const rendered = renderPackageMarkdown("Installs **jq** for `amd64`.\n\n```sh\nprintf ok\n```\n");
  assert.match(rendered, /<strong>jq<\/strong>/);
  assert.match(rendered, /<code>amd64<\/code>/);
  assert.match(rendered, /<pre><code class="language-sh">printf ok/);
});

test("package README does not execute embedded HTML", () => {
  const rendered = renderPackageMarkdown("<script>alert('unsafe')</script>");
  assert.doesNotMatch(rendered, /<script>/);
  assert.match(rendered, /&lt;script&gt;/);
});
