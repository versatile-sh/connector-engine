import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { edgesOf, revealOf, stepOf } from "./tabs.ts";

describe("stepOf", () => {
  test("moves right with the right arrow", () => {
    assert.equal(stepOf("ArrowRight", 1, 4), 2);
  });

  test("wraps from the last tab to the first", () => {
    assert.equal(stepOf("ArrowRight", 3, 4), 0);
  });

  test("wraps from the first tab to the last", () => {
    assert.equal(stepOf("ArrowLeft", 0, 4), 3);
  });

  test("goes to the ends with Home and End", () => {
    assert.deepEqual([stepOf("Home", 2, 4), stepOf("End", 1, 4)], [0, 3]);
  });

  test("moves nowhere on another key", () => {
    assert.equal(stepOf("Enter", 1, 4), undefined);
  });

  test("moves nowhere without tabs", () => {
    assert.equal(stepOf("ArrowRight", 0, 0), undefined);
  });

  test("starts from the first tab when none is chosen", () => {
    assert.equal(stepOf("ArrowRight", -1, 4), 1);
  });
});

describe("revealOf", () => {
  const row = { left: 100, width: 300 };

  test("stays when the tab shows whole", () => {
    assert.equal(revealOf({ left: 150, width: 80 }, row, 24), undefined);
  });

  test("scrolls back to a tab hidden at the start", () => {
    assert.equal(revealOf({ left: 90, width: 80 }, row, 24), 66);
  });

  test("scrolls on to a tab hidden at the end", () => {
    assert.equal(revealOf({ left: 380, width: 80 }, row, 24), 184);
  });

  test("never scrolls before the row's start", () => {
    assert.equal(revealOf({ left: 10, width: 80 }, { left: 20, width: 300 }, 24), 0);
  });
});

describe("edgesOf", () => {
  test("fades nothing when the row fits", () => {
    assert.deepEqual(edgesOf(0, 300, 300), { start: false, end: false });
  });

  test("fades the end while more lies past it", () => {
    assert.deepEqual(edgesOf(0, 600, 300), { start: false, end: true });
  });

  test("fades both ends in the middle", () => {
    assert.deepEqual(edgesOf(120, 600, 300), { start: true, end: true });
  });

  test("fades the start at the end", () => {
    assert.deepEqual(edgesOf(300, 600, 300), { start: true, end: false });
  });
});
