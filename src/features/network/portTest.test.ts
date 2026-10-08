import assert from "node:assert/strict";
import test from "node:test";
import { validPortTarget } from "./portTestValidation.js";

test("port targets accept hostnames, IP addresses, and valid ports", () => {
  assert.equal(validPortTarget("registry.internal", 443), true);
  assert.equal(validPortTarget("127.0.0.1", 8080), true);
  assert.equal(validPortTarget("[::1]", 22), true);
});

test("port targets reject invalid and shell-like inputs", () => {
  assert.equal(validPortTarget("", 443), false);
  assert.equal(validPortTarget("host;whoami", 443), false);
  assert.equal(validPortTarget("host name", 443), false);
  assert.equal(validPortTarget("../socket", 443), false);
  assert.equal(validPortTarget("localhost", 0), false);
  assert.equal(validPortTarget("localhost", 65_536), false);
});
