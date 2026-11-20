import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { clock, listChoices, offeredChoices, secondsLeft, summarizeArguments } from "./choices.ts";

describe("offeredChoices", () => {
  describe("when the record offers the session answer", () => {
    test("keeps the button order", () => {
      assert.deepEqual(offeredChoices(["deny", "session", "once", "always"]), ["once", "session", "always", "deny"]);
    });
  });

  describe("without the session answer", () => {
    test("leaves it out", () => {
      assert.deepEqual(offeredChoices(["once", "always", "deny"]), ["once", "always", "deny"]);
    });
  });

  describe("without choices", () => {
    test("offers all but session", () => {
      assert.deepEqual(offeredChoices(undefined), ["once", "always", "deny"]);
    });
  });

  describe("with an unknown choice", () => {
    test("drops it", () => {
      assert.deepEqual(offeredChoices(["once", "maybe"]), ["once"]);
    });
  });
});

describe("listChoices", () => {
  describe("when the mode is ask_once", () => {
    test("offers the session answer", () => {
      assert.deepEqual(listChoices("ask_once"), ["once", "session", "always", "deny"]);
    });
  });

  describe("when the mode is ask", () => {
    test("leaves the session answer out", () => {
      assert.deepEqual(listChoices("ask"), ["once", "always", "deny"]);
    });
  });
});

describe("summarizeArguments", () => {
  describe("with short values", () => {
    test("prints them whole", () => {
      assert.deepEqual(summarizeArguments({ to: "a@b.c", n: 2 }), { text: JSON.stringify({ to: "a@b.c", n: 2 }, null, 2), cut: 0 });
    });
  });

  describe("with a long nested string", () => {
    test("cuts it and counts the rest", () => {
      const summary = summarizeArguments({ body: { lines: ["x".repeat(12)] } }, 5);
      assert.equal(summary?.cut, 1);
      assert.deepEqual(JSON.parse(summary?.text ?? ""), { body: { lines: ["xxxxx… (+7)"] } });
    });
  });

  describe("without arguments", () => {
    test("returns nothing", () => {
      assert.equal(summarizeArguments(undefined), undefined);
      assert.equal(summarizeArguments({}), undefined);
    });
  });
});

describe("secondsLeft", () => {
  test("rounds up to whole seconds", () => {
    assert.equal(secondsLeft("2026-01-01T00:01:00.000Z", Date.parse("2026-01-01T00:00:00.500Z")), 60);
  });

  describe("when the time is past", () => {
    test("returns zero", () => {
      assert.equal(secondsLeft("2026-01-01T00:00:00Z", Date.parse("2026-01-01T00:05:00Z")), 0);
    });
  });
});

describe("clock", () => {
  test("pads the seconds", () => {
    assert.equal(clock(65), "1:05");
  });
});
