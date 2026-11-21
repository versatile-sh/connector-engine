import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { ActivitySummary, Agent, Connector, HourCount } from "../api.ts";
import { activeConnectors, deltaOf, failedShare, hoursOf, isQuiet, liveKeys, shareOf } from "./dashboard.ts";

const hour = (at: number): HourCount => ({ start: new Date(Date.UTC(2026, 10, 20, 0, 0) + at * 3_600_000).toISOString(), ok: at, error: 0, denied: 0, pending: 0, running: 0 });

const SUMMARY: ActivitySummary = {
  at: "2026-11-20T12:00:00Z",
  total: 5,
  day: { total: 3, failed: 1 },
  previous_day: { total: 2, failed: 0 },
  hours: Array.from({ length: 40 }, (_, at) => hour(at)),
  connectors: [],
  keys: [],
  recent: [],
};

describe("deltaOf", () => {
  describe("with nothing in either day", () => {
    test("reports none", () => assert.deepEqual(deltaOf(0, 0), { kind: "none" }));
  });

  describe("with calls and none the day before", () => {
    test("reports new", () => assert.deepEqual(deltaOf(7, 0), { kind: "new" }));
  });

  describe("with more calls than the day before", () => {
    test("reports up by a whole percent", () => assert.deepEqual(deltaOf(15, 12), { kind: "up", percent: 25 }));
  });

  describe("with fewer calls than the day before", () => {
    test("reports down by a whole percent", () => assert.deepEqual(deltaOf(1, 3), { kind: "down", percent: 67 }));
  });

  describe("with the same number of calls", () => {
    test("reports same", () => assert.deepEqual(deltaOf(4, 4), { kind: "same" }));
  });

  describe("with a change that rounds to nothing", () => {
    test("reports same", () => assert.deepEqual(deltaOf(1001, 1000), { kind: "same" }));
  });

  describe("with calls fallen to none", () => {
    test("reports down a hundred percent", () => assert.deepEqual(deltaOf(0, 5), { kind: "down", percent: 100 }));
  });
});

describe("isQuiet", () => {
  test("is true with no calls in the week", () => assert.equal(isQuiet({ ...SUMMARY, total: 0 }), true));
  test("is false with a call in the week, whatever the last day held", () => assert.equal(isQuiet({ ...SUMMARY, total: 1, day: { total: 0, failed: 0 } }), false));
});

describe("hoursOf", () => {
  describe("for the last 24 hours", () => {
    test("keeps the newest 24 hours in order", () => {
      const hours = hoursOf(SUMMARY, "24h");
      assert.equal(hours.length, 24);
      assert.equal(hours[0]?.ok, 16);
      assert.equal(hours[23]?.ok, 39);
    });

    test("keeps all of a shorter list", () => assert.equal(hoursOf({ ...SUMMARY, hours: SUMMARY.hours.slice(0, 5) }, "24h").length, 5));
  });

  describe("for seven days", () => {
    test("keeps every hour", () => assert.equal(hoursOf(SUMMARY, "7d").length, 40));
  });
});

describe("shareOf", () => {
  test("is a whole percent of the busiest row", () => assert.equal(shareOf(30, 120), 25));
  test("is the whole bar for the busiest row", () => assert.equal(shareOf(120, 120), 100));
  test("leaves a sliver for a row far behind", () => assert.equal(shareOf(1, 1000), 4));
  test("is nothing for a row without calls", () => assert.equal(shareOf(0, 10), 0));
  test("is nothing when no row has calls", () => assert.equal(shareOf(3, 0), 0));
});

describe("failedShare", () => {
  test("is the whole percent of the day's calls", () => assert.equal(failedShare({ total: 8, failed: 1 }), 13));
  test("is nothing without calls", () => assert.equal(failedShare({ total: 0, failed: 0 }), undefined));
});

describe("activeConnectors", () => {
  const connector = (enabled: boolean, blocked?: string) => ({ id: "c", alias: "a", enabled, ...(blocked ? { blocked } : {}), listing: "l", version: "1", title: "T", functions: [], profiles: [], slots: [] }) satisfies Connector;

  test("counts the connectors that are on and not blocked", () => {
    assert.equal(activeConnectors([connector(true), connector(false), connector(true, "needs a newer console"), connector(true)]), 2);
  });
});

describe("liveKeys", () => {
  const key = (expires_at: string): Agent => ({ name: expires_at, expires_at, new: false });

  test("counts the keys that have not expired", () => {
    const now = Date.parse("2026-11-20T12:00:00Z");
    assert.equal(liveKeys([key("2026-11-21T00:00:00Z"), key("2026-11-19T00:00:00Z"), key("2999-01-01T00:00:00Z")], now), 2);
  });
});
