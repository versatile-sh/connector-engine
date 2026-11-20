import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { keptOf, type Kept } from "./kept.ts";

const drawn = (...keys: string[]): Kept<string>[] => keys.map((key) => ({ row: key, key, phase: "stay" }));

const phases = (kept: readonly Kept<string>[]) => kept.map((row) => `${row.key}:${row.phase}`);

const same = (row: string) => row;

describe("keptOf", () => {
  describe("when nothing changed", () => {
    test("keeps every row staying", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "b"), ["a", "b"], same)), ["a:stay", "b:stay"]);
    });
  });

  describe("when rows arrive", () => {
    test("marks them entering in their place", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "c"), ["a", "b", "c"], same)), ["a:stay", "b:enter", "c:stay"]);
    });
  });

  describe("when rows go", () => {
    test("keeps them leaving before the next row that stays", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "b", "c", "d"), ["c", "d"], same)), ["a:leave", "b:leave", "c:stay", "d:stay"]);
    });

    test("keeps them leaving at the end after the last row that stays", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "b", "c"), ["a"], same)), ["a:stay", "b:leave", "c:leave"]);
    });

    test("keeps every row leaving when the list empties", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "b"), [], same)), ["a:leave", "b:leave"]);
    });
  });

  describe("when rows go and arrive at once", () => {
    test("marks each in its place", () => {
      assert.deepEqual(phases(keptOf(drawn("a", "b", "c"), ["c", "x"], same)), ["a:leave", "b:leave", "c:stay", "x:enter"]);
    });
  });

  describe("when a row was already leaving", () => {
    test("drops it", () => {
      const before: Kept<string>[] = [{ row: "a", key: "a", phase: "leave" }, ...drawn("b")];
      assert.deepEqual(phases(keptOf(before, ["b"], same)), ["b:stay"]);
    });

    test("brings it back entering when it returns", () => {
      const before: Kept<string>[] = [{ row: "a", key: "a", phase: "leave" }, ...drawn("b")];
      assert.deepEqual(phases(keptOf(before, ["a", "b"], same)), ["a:enter", "b:stay"]);
    });
  });

  test("draws the newest copy of a row that stays", () => {
    const before: Kept<{ id: string; n: number }>[] = [{ row: { id: "a", n: 1 }, key: "a", phase: "stay" }];
    assert.deepEqual(keptOf(before, [{ id: "a", n: 2 }], (row) => row.id)[0]?.row, { id: "a", n: 2 });
  });
});
