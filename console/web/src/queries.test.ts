import assert from "node:assert/strict";
import { afterEach, describe, test } from "node:test";
import { QueryClient } from "@tanstack/react-query";
import { keysPageQuery } from "./queries.ts";

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
});

function serve(agents: { name: string; new: boolean }[], seen: number) {
  const calls: string[] = [];
  globalThis.fetch = async (input, init) => {
    const call = `${init?.method ?? "GET"} ${String(input)}`;
    calls.push(call);
    if (call === "POST /api/agents/seen") return new Response(null, { status: seen });
    return new Response(JSON.stringify({ agents, max_ttl_hours: 720 }), { status: 200 });
  };
  return calls;
}

describe("keysPageQuery", () => {
  describe("when a key is new", () => {
    test("marks the keys seen after the list arrives", async () => {
      const calls = serve([{ name: "cursor", new: true }], 204);
      const data = await new QueryClient().fetchQuery(keysPageQuery);
      assert.deepEqual(calls, ["GET /api/agents", "POST /api/agents/seen"]);
      assert.equal(data.agents[0]?.new, true);
    });

    describe("and the mark fails", () => {
      test("still answers the list", async () => {
        serve([{ name: "cursor", new: true }], 500);
        assert.equal((await new QueryClient().fetchQuery(keysPageQuery)).agents.length, 1);
      });
    });
  });

  describe("when no key is new", () => {
    test("reads the list only", async () => {
      const calls = serve([{ name: "cursor", new: false }], 204);
      await new QueryClient().fetchQuery(keysPageQuery);
      assert.deepEqual(calls, ["GET /api/agents"]);
    });
  });
});
