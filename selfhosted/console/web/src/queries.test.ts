import assert from "node:assert/strict";
import { afterEach, describe, test } from "node:test";
import { QueryClient } from "@tanstack/react-query";
import type { ApprovalView } from "./api.ts";
import { activityQuery, ALWAYS_WAIT_MS, approvalPolls, keysPageQuery } from "./queries.ts";

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

describe("approvalPolls", () => {
  const now = Date.parse("2026-11-20T12:00:00Z");
  const at = (ms: number) => new Date(now + ms).toISOString();
  const approval = (over: Partial<ApprovalView>) => ({ state: "pending", expires_at: at(60_000), ...over }) as ApprovalView;

  test("polls a call that waits until it expires", () => {
    assert.deepEqual([approvalPolls(approval({}), now), approvalPolls(approval({ expires_at: at(-1) }), now)], [true, false]);
  });

  describe("with an allowed call", () => {
    test("polls while an always allow may still be written", () => {
      assert.equal(approvalPolls(approval({ state: "executed", decided_at: at(-1_000) }), now), true);
    });

    test("stops once that time is over or the write failed", () => {
      assert.deepEqual(
        [approvalPolls(approval({ state: "executed", decided_at: at(-ALWAYS_WAIT_MS) }), now), approvalPolls(approval({ state: "approved", decided_at: at(-1_000), always_error: "refused" }), now)],
        [false, false],
      );
    });
  });

  test("stops for a denied call", () => {
    assert.equal(approvalPolls(approval({ state: "denied", decided_at: at(-1_000) }), now), false);
  });
});

describe("activityQuery", () => {
  const body = (calls: string[], next?: string) =>
    JSON.stringify({ from: "f", to: "t", calls: calls.map((id) => ({ id, at: "2026-11-20T10:00:00Z", agent: "a", function: "f", outcome: "ok", captured: true })), total: 3, hours: [], connectors: [], agents: [], next_cursor: next });

  test("keys a filter once, whatever the time of the fetch", () => {
    assert.deepEqual(activityQuery({ range: "24h", connector: "c1" }).queryKey, activityQuery({ range: "24h", connector: "c1" }).queryKey);
    assert.notDeepEqual(activityQuery({ range: "24h", connector: "c1" }).queryKey, activityQuery({ range: "24h", connector: "c2" }).queryKey);
  });

  test("reads one page of 50 and then the one after its cursor", async () => {
    const urls: string[] = [];
    globalThis.fetch = async (input) => {
      urls.push(String(input));
      return new Response(urls.length === 1 ? body(["c3", "c2"], "cur1") : body(["c1"]), { status: 200 });
    };
    const client = new QueryClient();
    const options = activityQuery({ range: "24h", outcome: "error" });
    const first = await client.fetchInfiniteQuery({ ...options, pages: 2 });
    assert.deepEqual(first.pages.map((page) => page.calls.length), [2, 1]);
    const [one, two] = urls.map((url) => new URL(url, "http://x").searchParams);
    assert.deepEqual([one?.get("limit"), one?.get("outcome"), one?.has("cursor")], ["50", "error", false]);
    assert.equal(two?.get("cursor"), "cur1");
  });

  describe("polling", () => {
    const polls = (pages: number) => {
      const refetch = activityQuery({ range: "24h" }).refetchInterval;
      if (typeof refetch !== "function") throw new Error("polling is decided per query");
      const query = { state: { data: { pages: Array.from({ length: pages }, () => ({})) } } };
      return refetch(query as never);
    };

    test("runs while only the first page is loaded", () => {
      assert.equal(polls(1), 10_000);
    });

    test("stops once more pages are loaded, so a long list is not read again and again", () => {
      assert.equal(polls(3), false);
    });
  });
});
