import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { guides } from "./clients.ts";

const url = "http://127.0.0.1:7465/mcp";
const key = "k3y";
const all = guides(url, key, (message, params) => `${message}${params ? JSON.stringify(params) : ""}`);
const guide = (id: string) => {
  const found = all.find((g) => g.id === id);
  assert.ok(found, id);
  return found;
};

describe("guides", () => {
  test("puts the key in every block", () => {
    for (const g of all) for (const block of g.blocks) assert.ok(block.text.includes(key), `${g.id} ${block.where}`);
  });

  describe("Cursor", () => {
    test("links the server object as base64 JSON", () => {
      const href = new URL(guide("cursor").link?.href ?? "");
      assert.equal(href.protocol, "cursor:");
      assert.equal(href.searchParams.get("name"), "versatile");
      const config: unknown = JSON.parse(atob(href.searchParams.get("config") ?? ""));
      assert.deepEqual(config, { url, headers: { Authorization: `Bearer ${key}` } });
    });
  });

  describe("VS Code", () => {
    test("links the named http server as URL-encoded JSON", () => {
      const href = guide("vscode").link?.href ?? "";
      assert.ok(href.startsWith("vscode:mcp/install?"));
      const config: unknown = JSON.parse(decodeURIComponent(href.slice("vscode:mcp/install?".length)));
      assert.deepEqual(config, { name: "versatile", type: "http", url, headers: { Authorization: `Bearer ${key}` } });
    });
  });

  describe("Claude Code", () => {
    test("puts the name and URL before the header flag", () => {
      assert.equal(guide("claude-code").blocks[0]?.text, `claude mcp add --transport http versatile ${url} --header "Authorization: Bearer ${key}"`);
    });
  });

  describe("Claude Desktop", () => {
    test("bridges with mcp-remote and keeps the header out of the args", () => {
      const config = JSON.parse(guide("claude-desktop").blocks[0]?.text ?? "");
      assert.deepEqual(config.mcpServers.versatile.args, ["-y", "mcp-remote", url, "--header", "Authorization:${AUTH_HEADER}"]);
      assert.equal(config.mcpServers.versatile.env.AUTH_HEADER, `Bearer ${key}`);
    });
  });
});
