import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { createTranslate } from "../i18n/index.ts";
import { codeTag, reasonText, requireTags, warningText } from "./compat.ts";

const t = createTranslate("en");
const none = { tools: [], runtimes: [], network: false, mcp: [] };

describe("warningText", () => {
  describe("for an app with no shell and a skill that ships scripts", () => {
    test("says it can load the instructions but not run the scripts", () =>
      assert.equal(warningText({ name: "Gemini CLI", reasons: ["runs-bundled-code"] }, t), "Gemini CLI can load these instructions but cannot run this skill's scripts."));
  });
  describe("for an app missing capabilities", () => {
    test("names each one", () =>
      assert.equal(warningText({ name: "Claude Desktop", reasons: ["no-shell", "no-files"] }, t), "Claude Desktop can load these instructions but lacks a shell, file access, so part of them cannot be followed."));
  });
  describe("in Portuguese", () => {
    test("is translated", () => assert.match(warningText({ name: "Claude Desktop", reasons: ["runs-bundled-code"] }, createTranslate("pt-BR")), /não consegue executar os scripts/));
  });
});

describe("reasonText", () => {
  test("a full status reads as having everything", () => assert.equal(reasonText({ status: "full", reasons: ["ok"] }, t), "Has everything this skill needs"));
  test("joins several reasons", () => assert.equal(reasonText({ status: "instructions", reasons: ["no-shell", "no-web"] }, t), "Needs a shell · Needs web access"));
  test("an unknown reason falls back", () => assert.equal(reasonText({ status: "no", reasons: ["made-up"] }, t), "Cannot use all of it"));
});

describe("codeTag", () => {
  test("scripts mean the skill runs code", () => assert.equal(codeTag({ has_scripts: true, needs: none }), "runs"));
  test("a Bash tool without scripts means it needs a shell", () => assert.equal(codeTag({ has_scripts: false, needs: { ...none, tools: ["Bash"] } }), "shell"));
  test("a runtime without scripts means it needs a shell", () => assert.equal(codeTag({ has_scripts: false, needs: { ...none, runtimes: ["git"] } }), "shell"));
  test("plain instructions carry no tag", () => assert.equal(codeTag({ has_scripts: false, needs: { ...none, tools: ["Read"] } }), undefined));
});

describe("requireTags", () => {
  test("lists tools, runtimes, network and MCP servers", () => assert.deepEqual(requireTags({ tools: ["Bash"], runtimes: ["python"], network: true, mcp: ["gh"] }), ["Bash", "python", "network", "mcp:gh"]));
  test("lists nothing for a plain skill", () => assert.deepEqual(requireTags(none), []));
});
