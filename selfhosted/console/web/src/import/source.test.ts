import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { readSource } from "./source.ts";

const plain = { local: false, headerSecret: false };

describe("readSource", () => {
  describe("with an MCP server URL", () => {
    test("reads it as an MCP source", () => {
      assert.deepEqual(readSource(" https://mcp.example.com/mcp ", "mcp", plain), { ok: true, kind: "mcp", source: { mcp: "https://mcp.example.com/mcp" } });
    });

    test("carries the local and secret-header choices", () => {
      assert.deepEqual(readSource("http://127.0.0.1:9000/mcp", "mcp", { local: true, headerSecret: true }), {
        ok: true,
        kind: "mcp",
        source: { mcp: "http://127.0.0.1:9000/mcp", local: true, header_secret: true },
      });
    });
  });

  describe("with a listing URL", () => {
    test("reads it as a listing and drops the MCP-only choices", () => {
      assert.deepEqual(readSource("https://versatile.sh/l/linear.yaml", "listing", { local: true, headerSecret: true }), {
        ok: true,
        kind: "listing",
        source: { listing: "https://versatile.sh/l/linear.yaml" },
      });
    });
  });

  describe("with a console install link", () => {
    test("unwraps a listing link whatever kind was chosen", () => {
      const link = "http://127.0.0.1:7465/import?listing=" + encodeURIComponent("https://versatile.sh/l/linear.yaml");
      assert.deepEqual(readSource(link, "mcp", plain), { ok: true, kind: "listing", source: { listing: "https://versatile.sh/l/linear.yaml" } });
    });

    test("unwraps an MCP link", () => {
      const link = "http://127.0.0.1:7465/import?mcp=" + encodeURIComponent("https://mcp.example.com/mcp");
      assert.deepEqual(readSource(link, "listing", plain), { ok: true, kind: "mcp", source: { mcp: "https://mcp.example.com/mcp" } });
    });

    test("refuses a link that carries no http address", () => {
      assert.deepEqual(readSource("http://127.0.0.1:7465/import?listing=javascript:alert(1)", "mcp", plain), { ok: false });
    });
  });

  describe("with something that is not an http URL", () => {
    for (const text of ["", "mcp.example.com", "ftp://example.com/x", "file:///etc/passwd"]) {
      test(`refuses ${JSON.stringify(text)}`, () => {
        assert.deepEqual(readSource(text, "mcp", plain), { ok: false });
      });
    }
  });
});
