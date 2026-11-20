import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { boxTransform, irisAt } from "./motion.ts";

describe("irisAt", () => {
  describe("with a pointer", () => {
    test("opens at it and reaches the farthest corner", () => assert.deepEqual(irisAt(100, 40, { x: 10, y: 30 }), { x: 10, y: 30, r: Math.hypot(90, 30) }));
  });
  describe("without one, as for a key press", () => {
    test("opens at the centre and reaches every corner", () => assert.deepEqual(irisAt(100, 40), { x: 50, y: 20, r: Math.hypot(50, 20) }));
  });
});

describe("boxTransform", () => {
  const modal = { left: 100, top: 50, width: 800, height: 600 };
  test("draws the element over the other box", () => assert.equal(boxTransform({ left: 20, top: 300, width: 200, height: 150 }, modal), "translate(-380px, 25px) scale(0.25, 0.25)"));
  describe("with the same box", () => {
    test("leaves the element where it is", () => assert.equal(boxTransform(modal, modal), "translate(0px, 0px) scale(1, 1)"));
  });
});
