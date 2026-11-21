import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { pickTheme, resolveTheme } from "./theme.ts";

describe("pickTheme", () => {
  test("keeps a saved light or dark", () => {
    assert.deepEqual([pickTheme("light"), pickTheme("dark")], ["light", "dark"]);
  });

  test("follows the system with nothing or junk saved", () => {
    assert.deepEqual([pickTheme(null), pickTheme("system"), pickTheme("purple")], ["system", "system", "system"]);
  });
});

describe("resolveTheme", () => {
  test("with system, follows what the system prefers", () => {
    assert.deepEqual([resolveTheme("system", true), resolveTheme("system", false)], ["light", "dark"]);
  });

  test("with a forced choice, ignores the system", () => {
    assert.deepEqual([resolveTheme("dark", true), resolveTheme("light", false)], ["dark", "light"]);
  });
});
