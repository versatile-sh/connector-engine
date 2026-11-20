import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { guides, oauthGuides } from "./clients.ts";

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

describe("oauthGuides", () => {
  const oauth = oauthGuides(url, (message) => message);
  const find = (id: string) => {
    const found = oauth.find((g) => g.id === id);
    assert.ok(found, id);
    return found;
  };

  test("never asks for a key or a header", () => {
    for (const g of oauth) {
      assert.ok(!(g.link?.href ?? "").includes("Authorization"), g.id);
      for (const block of g.blocks) assert.ok(!/Authorization|Bearer|header/i.test(block.text), `${g.id} ${block.where}`);
    }
  });

  test("Claude Code adds the URL alone", () => {
    assert.equal(find("claude-code").blocks[0]?.text, `claude mcp add --transport http versatile ${url}`);
  });

  test("Codex adds the URL alone", () => {
    assert.equal(find("codex").blocks[0]?.text, `codex mcp add versatile --url ${url}`);
  });

  test("Claude Desktop bridges the URL with mcp-remote", () => {
    assert.deepEqual(JSON.parse(find("claude-desktop").blocks[0]?.text ?? "").mcpServers.versatile.args, ["-y", "mcp-remote", url]);
  });

  test("the Cursor link carries only the URL", () => {
    const href = new URL(find("cursor").link?.href ?? "");
    assert.deepEqual(JSON.parse(atob(href.searchParams.get("config") ?? "")), { url });
  });

  test("the VS Code link carries only the name, type and URL", () => {
    const href = find("vscode").link?.href ?? "";
    assert.deepEqual(JSON.parse(decodeURIComponent(href.slice("vscode:mcp/install?".length))), { name: "versatile", type: "http", url });
  });
});
