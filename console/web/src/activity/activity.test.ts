import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { HourCount } from "../api.ts";
import { apiQueryOf, approvalStateKey, barsOf, durationText, filterOf, hrefOf, kindOf, prettyJSON } from "./activity.ts";

const hour = (start: Date, counts: Partial<Omit<HourCount, "start">> = {}): HourCount => ({ start: start.toISOString(), ok: 0, error: 0, denied: 0, pending: 0, running: 0, ...counts });

describe("filterOf", () => {
  test("reads the range, connector, app and outcome of the query", () => {
    assert.deepEqual(filterOf(new URLSearchParams("range=7d&connector=c1&agent=cursor&outcome=denied")), { range: "7d", connector: "c1", agent: "cursor", outcome: "denied" });
  });

  describe("with values it does not know", () => {
    test("falls back to a day and drops the outcome", () => {
      assert.deepEqual(filterOf(new URLSearchParams("range=1y&outcome=maybe")), { range: "24h" });
    });
  });
});

describe("hrefOf", () => {
  test("leaves the default range out", () => {
    assert.equal(hrefOf({ range: "24h", connector: "c1" }), "/activity?connector=c1");
  });

  test("is read back by filterOf", () => {
    const filter = { range: "30d", agent: "claude-code", outcome: "error" } as const;
    assert.deepEqual(filterOf(new URL(hrefOf(filter), "http://x").searchParams), filter);
  });
});

describe("apiQueryOf", () => {
  test("ends the range now and starts it the range's hours before", () => {
    const now = new Date("2026-11-20T12:00:00Z");
    const query = new URLSearchParams(apiQueryOf({ range: "7d", connector: "c1" }, now));
    assert.equal(query.get("to"), "2026-11-20T12:00:00.000Z");
    assert.equal(query.get("from"), "2026-11-13T12:00:00.000Z");
    assert.equal(query.get("connector"), "c1");
  });
});

describe("barsOf", () => {
  describe("over a day", () => {
    test("keeps one bar per hour with errors, denials and waits apart", () => {
      const bars = barsOf([hour(new Date(2026, 10, 20, 9), { ok: 2, error: 1 }), hour(new Date(2026, 10, 20, 10), { denied: 1, pending: 1, running: 2 })], "24h");
      assert.equal(bars.length, 2);
      assert.deepEqual(
        bars.map(({ ok, failed, denied, waiting, total, unit }) => ({ ok, failed, denied, waiting, total, unit })),
        [
          { ok: 2, failed: 1, denied: 0, waiting: 0, total: 3, unit: "hour" },
          { ok: 0, failed: 0, denied: 1, waiting: 3, total: 4, unit: "hour" },
        ],
      );
    });
  });

  describe("over a week", () => {
    test("sums the hours of each local day into one bar", () => {
      const hours = [hour(new Date(2026, 10, 19, 22), { ok: 1 }), hour(new Date(2026, 10, 19, 23), { ok: 2 }), hour(new Date(2026, 10, 20, 0), { error: 1 })];
      const bars = barsOf(hours, "7d");
      assert.deepEqual(
        bars.map((bar) => [bar.total, bar.unit]),
        [
          [3, "day"],
          [1, "day"],
        ],
      );
    });
  });
});

describe("durationText", () => {
  test("says milliseconds, seconds or minutes", () => {
    assert.deepEqual([durationText(42), durationText(1_500), durationText(42_000), durationText(180_000), durationText(undefined)], ["42 ms", "1.5 s", "42 s", "3 min", "—"]);
  });
});

describe("prettyJSON", () => {
  test("indents JSON", () => {
    assert.equal(prettyJSON('{"a":[1]}'), '{\n  "a": [\n    1\n  ]\n}');
  });

  describe("with text that is not JSON", () => {
    test("leaves it as it is", () => {
      assert.equal(prettyJSON('{"a":'), '{"a":');
    });
  });
});

describe("kindOf and approvalStateKey", () => {
  test("name only what the console has a label for", () => {
    assert.deepEqual([kindOf("write"), kindOf("admin"), kindOf(undefined)], ["write", undefined, undefined]);
    assert.deepEqual([approvalStateKey("denied"), approvalStateKey("unheard"), approvalStateKey(undefined)], ["approvalState.denied", undefined, undefined]);
  });
});
