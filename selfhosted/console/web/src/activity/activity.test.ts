import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { Connector, HourCount, Skill } from "../api.ts";
import { apiQueryOf, approvalStateKey, barsOf, callOf, clearedOf, inScope, daysBetween, durationText, filterOf, hrefOf, keptSince, kindOf, lastWeek, namedOf, prettyJSON, skillFile } from "./activity.ts";

const hour = (start: Date, counts: Partial<Omit<HourCount, "start">> = {}): HourCount => ({ start: start.toISOString(), ok: 0, error: 0, denied: 0, pending: 0, running: 0, ...counts });

describe("filterOf", () => {
  test("reads the range, connector, app, outcome and search of the query", () => {
    assert.deepEqual(filterOf(new URLSearchParams("range=7d&connector=c1&agent=cursor&outcome=denied&search=send")), { range: "7d", connector: "c1", agent: "cursor", outcome: "denied", search: "send" });
  });

  describe("with values it does not know", () => {
    test("falls back to a day and drops the outcome", () => {
      assert.deepEqual(filterOf(new URLSearchParams("range=1y&outcome=maybe")), { range: "24h" });
    });
  });

  describe("with two days", () => {
    test("reads a custom period, earliest first", () => {
      assert.deepEqual(filterOf(new URLSearchParams("from=2026-11-12&to=2026-11-03&range=7d")), { range: "custom", from: "2026-11-03", to: "2026-11-12" });
    });
  });

  describe("with a day that does not exist", () => {
    test("ignores the custom period", () => {
      assert.deepEqual(filterOf(new URLSearchParams("from=2026-02-30&to=2026-03-02")), { range: "24h" });
    });
  });
});

describe("hrefOf", () => {
  test("leaves the default range out", () => {
    assert.equal(hrefOf({ range: "24h", connector: "c1" }), "/activity?connector=c1");
  });

  for (const filter of [{ range: "30d", agent: "claude-code", outcome: "error" }, { range: "custom", from: "2026-11-01", to: "2026-11-07", connector: "c1", search: "list" }] as const) {
    test(`is read back by filterOf over ${filter.range}`, () => {
      assert.deepEqual(filterOf(new URL(hrefOf(filter), "http://x").searchParams), filter);
    });
  }

  describe("with an open call", () => {
    test("names it apart from the filter", () => {
      const url = new URL(hrefOf({ range: "7d", outcome: "ok" }, "c-42"), "http://x");
      assert.deepEqual([filterOf(url.searchParams), callOf(url.searchParams)], [{ range: "7d", outcome: "ok" }, "c-42"]);
    });
  });

  describe("without a call", () => {
    test("leaves none open", () => {
      assert.equal(callOf(new URL(hrefOf({ range: "24h" }), "http://x").searchParams), undefined);
    });
  });
});

describe("apiQueryOf", () => {
  test("ends a preset now and starts it the range's hours before", () => {
    const now = new Date("2026-11-20T12:00:00Z");
    const query = new URLSearchParams(apiQueryOf({ range: "7d", connector: "c1", search: "send" }, now));
    assert.equal(query.get("to"), "2026-11-20T12:00:00.000Z");
    assert.equal(query.get("from"), "2026-11-13T12:00:00.000Z");
    assert.deepEqual([query.get("connector"), query.get("search")], ["c1", "send"]);
  });

  describe("over custom days", () => {
    test("runs from the first's local midnight to the end of the last", () => {
      const query = new URLSearchParams(apiQueryOf({ range: "custom", from: "2026-11-02", to: "2026-11-03" }, new Date(2026, 10, 20)));
      assert.deepEqual([query.get("from"), query.get("to")], [new Date(2026, 10, 2).toISOString(), new Date(2026, 10, 4).toISOString()]);
    });

    test("ends now when the last day is today", () => {
      const now = new Date(2026, 10, 20, 15, 30);
      assert.equal(new URLSearchParams(apiQueryOf({ range: "custom", from: "2026-11-19", to: "2026-11-20" }, now)).get("to"), now.toISOString());
    });

    test("starts at most 30 days before its end", () => {
      const now = new Date(2026, 10, 20, 15, 30);
      const query = new URLSearchParams(apiQueryOf({ range: "custom", from: "2026-09-01", to: "2026-11-20" }, now));
      assert.equal(query.get("from"), new Date(now.getTime() - 30 * 24 * 3_600_000).toISOString());
    });
  });
});

describe("lastWeek", () => {
  test("is the last seven local days, today included", () => {
    assert.deepEqual(lastWeek(new Date(2026, 10, 3, 9)), { range: "custom", from: "2026-10-28", to: "2026-11-03" });
  });
});

describe("daysBetween", () => {
  test("lists the days newest first, both ends included", () => {
    assert.deepEqual(daysBetween("2026-10-30", "2026-11-02"), ["2026-11-02", "2026-11-01", "2026-10-31", "2026-10-30"]);
  });

  test("lists one day when both ends are the same", () => {
    assert.deepEqual(daysBetween("2026-11-02", "2026-11-02"), ["2026-11-02"]);
  });

  test("lists nothing when the start is after the end", () => {
    assert.deepEqual(daysBetween("2026-11-03", "2026-11-02"), []);
  });

  test("lists nothing for a day that does not exist", () => {
    assert.deepEqual(daysBetween("2026-02-30", "2026-03-02"), []);
  });
});

describe("keptSince", () => {
  test("is thirty local days before today", () => {
    assert.equal(keptSince(new Date(2026, 10, 3, 9)), "2026-10-04");
  });
});

describe("barsOf", () => {
  describe("over a day", () => {
    test("keeps one bar per hour with errors, denials and waits apart", () => {
      const bars = barsOf([hour(new Date(2026, 10, 20, 9), { ok: 2, error: 1 }), hour(new Date(2026, 10, 20, 10), { denied: 1, pending: 1, running: 2 })]);
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
      const start = new Date(2026, 10, 13, 22);
      const hours = Array.from({ length: 24 * 7 }, (_, i) => hour(new Date(start.getTime() + i * 3_600_000), { ok: i < 2 ? i + 1 : 0, error: i === 2 ? 1 : 0 }));
      const bars = barsOf(hours).slice(0, 2);
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

describe("namedOf", () => {
  test("names connectors and skills by id, marking the skills", () => {
    const connector = { id: "c1", title: "Notes", icon: "https://x/i.png" } as Connector;
    const skill = { id: "s1", title: "", alias: "pdf" } as Skill;
    const named = namedOf([connector], [skill]);
    assert.deepEqual(named.get("c1"), { id: "c1", title: "Notes", icon: "https://x/i.png", skill: false });
    assert.deepEqual(named.get("s1"), { id: "s1", title: "pdf", skill: true });
  });
});

describe("skillFile", () => {
  test("is SKILL.md unless a file was named", () => {
    assert.deepEqual([skillFile(undefined), skillFile(""), skillFile("reference/fields.md")], ["SKILL.md", "SKILL.md", "reference/fields.md"]);
  });
});

describe("the activity scope", () => {
  describe("filterOf", () => {
    test("reads everyone and its person", () => {
      assert.deepEqual(filterOf(new URLSearchParams("scope=everyone&person=p-bob")), { range: "24h", scope: "everyone", person: "p-bob" });
    });

    test("drops a person outside everyone", () => {
      assert.deepEqual(filterOf(new URLSearchParams("person=p-bob")), { range: "24h" });
    });
  });

  describe("inScope", () => {
    test("drops everyone and the person for mine", () => {
      assert.deepEqual(inScope({ range: "7d", scope: "everyone", person: "p-bob" }, "mine"), { range: "7d" });
    });

    test("keeps the person for everyone", () => {
      assert.deepEqual(inScope({ range: "7d", person: "p-bob" }, "everyone"), { range: "7d", scope: "everyone", person: "p-bob" });
    });
  });

  describe("clearedOf", () => {
    test("keeps whose calls it shows", () => {
      assert.deepEqual(clearedOf({ range: "7d", scope: "everyone", person: "p-bob", outcome: "ok" }), { range: "7d", scope: "everyone" });
    });
  });

  test("sends the scope and person to the API", () => {
    const query = new URLSearchParams(apiQueryOf({ range: "24h", scope: "everyone", person: "p-bob" }, new Date("2026-11-20T00:00:00Z")));
    assert.deepEqual([query.get("scope"), query.get("person")], ["everyone", "p-bob"]);
  });
});
