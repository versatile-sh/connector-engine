import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { irisAt } from "./motion.ts";

describe("irisAt", () => {
  describe("with a pointer", () => {
    test("opens at it and reaches the farthest corner", () => assert.deepEqual(irisAt(100, 40, { x: 10, y: 30 }), { x: 10, y: 30, r: Math.hypot(90, 30) }));
  });
  describe("without one, as for a key press", () => {
    test("opens at the centre and reaches every corner", () => assert.deepEqual(irisAt(100, 40), { x: 50, y: 20, r: Math.hypot(50, 20) }));
  });
});
