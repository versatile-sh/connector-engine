import { describe, test } from "node:test";
import { createElement } from "react";
import type { ClientCompat, CompatStatus, SkillFacts } from "../api";
import { renderMarkup } from "../testing/render.ts";
import { SkillCompatView } from "./SkillCompatView.tsx";

const APPS = [
  ["claude-code", "Claude Code"],
  ["codex-cli", "Codex CLI"],
  ["cursor", "Cursor"],
  ["gemini-cli", "Gemini CLI"],
  ["vscode-copilot", "VS Code Copilot"],
  ["opencode", "opencode / oh-my-pi"],
  ["claude-desktop", "Claude Desktop"],
  ["chat-mcp", "ChatGPT and other chat-only MCP clients"],
] as const;

const none: SkillFacts["needs"] = { tools: [], runtimes: [], network: false, mcp: [] };

const compat = (status: (client: string) => CompatStatus): ClientCompat[] =>
  APPS.map(([client, name]) => {
    const own = status(client);
    return { client, name, status: own, reasons: own === "full" ? ["ok"] : own === "instructions" ? ["runs-bundled-code"] : ["no-shell"], source: "derived" };
  });

const draw = (skill: Pick<SkillFacts, "needs" | "compat">, details = false) => renderMarkup(createElement(SkillCompatView, { skill, details }));

const marks = (html: string) => html.match(/role="img"/g)?.length ?? 0;

describe("SkillCompatView", () => {
  describe("when every app works", () => {
    const html = draw({ needs: none, compat: compat(() => "full") });

    test("says so in one line", (t) => {
      t.assert.match(html, />Works in every AI app</);
      t.assert.equal(html.split(">Works in").length - 1, 1);
    });

    test("draws one mark per app", (t) => {
      t.assert.equal(marks(html), APPS.length);
    });

    test("names each app, its status and the reason, in the mark's accessible name and tooltip", (t) => {
      t.assert.match(html, /role="img" aria-label="Claude Code: Works\. Has everything this skill needs" title="Claude Code: Works\. Has everything this skill needs"/);
      t.assert.match(html, /aria-label="ChatGPT and other chat-only MCP clients: Works\./);
    });

    test("draws no table and no requirements", (t) => {
      t.assert.doesNotMatch(html, /<table|Requires|Details/);
    });
  });

  describe("when some apps run only the instructions or not at all", () => {
    const html = draw({ needs: { ...none, runtimes: ["bash"] }, compat: compat((client) => (client === "claude-desktop" ? "instructions" : client === "chat-mcp" ? "no" : "full")) });

    test("groups the marks by status under their label", (t) => {
      t.assert.ok(html.indexOf(">Works in<") < html.indexOf(">Instructions only<"));
      t.assert.ok(html.indexOf(">Instructions only<") < html.indexOf(">Does not run<"));
      t.assert.doesNotMatch(html, /Works in every AI app/);
    });

    test("draws one mark per app", (t) => {
      t.assert.equal(marks(html), APPS.length);
    });

    test("gives the reason of an app that cannot run it", (t) => {
      t.assert.match(html, /aria-label="Claude Desktop: Instructions only\. Ships scripts and has no shell to run them"/);
      t.assert.match(html, /aria-label="ChatGPT and other chat-only MCP clients: Does not run\. Needs a shell"/);
    });

    test("marks the status with a lime, an amber and a muted dot, and strikes the app that does not run", (t) => {
      t.assert.equal(html.split("bg-lime").length - 1, APPS.length - 2);
      t.assert.equal(html.split("bg-amber-400").length - 1, 1);
      t.assert.equal(html.split("-rotate-45").length - 1, 1);
    });

    test("lists what it requires", (t) => {
      t.assert.match(html, />Requires</);
      t.assert.match(html, /title="bash"[^>]*><span class="truncate">bash</);
    });
  });

  describe("with an app the console has no glyph for", () => {
    test("shows the first letter of its name", (t) => {
      const html = draw({ needs: none, compat: [{ client: "future-app", name: "Future App", status: "full", reasons: ["ok"], source: "derived" }] });
      t.assert.match(html, /uppercase[^>]*>F</);
    });
  });

  describe("with no app listed", () => {
    const html = draw({ needs: none, compat: [] });

    test("draws no mark, no line and no disclosure", (t) => {
      t.assert.equal(marks(html), 0);
      t.assert.doesNotMatch(html, /Works in<|Details/);
    });

    test("draws nothing at all when nothing is required either", (t) => {
      t.assert.equal(html, "");
    });
  });

  describe("with details", () => {
    const skill = { needs: none, compat: compat((client) => (client === "chat-mcp" ? "no" : "full")) };

    test("keeps the full table behind a Details disclosure", (t) => {
      const html = draw(skill, true);
      t.assert.match(html, /<button[^>]*aria-expanded="false"[^>]*>[^]*Details/);
      t.assert.equal(html.split("<tr").length - 1, APPS.length + 1);
      t.assert.match(html, /Derived from the skill&#x27;s files/);
    });

    test("forces no width on its table, whatever the name or the reason", (t) => {
      const html = draw(skill, true);
      t.assert.doesNotMatch(html, /whitespace-nowrap|overflow-x-auto|min-w-\[/);
      t.assert.match(html, /<th scope="row" class="[^"]*\[overflow-wrap:anywhere\]/);
    });

    test("without them draws no table at all", (t) => {
      t.assert.doesNotMatch(draw(skill), /<table|Details/);
    });
  });
});
