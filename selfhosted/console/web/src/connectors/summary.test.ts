import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { accessCounts, asksOnce, commonAccess } from "./summary.ts";

describe("commonAccess", () => {
  test("is the access every choice shares", () => {
    assert.equal(commonAccess([{ access: "ask" }, { access: "ask" }]), "ask");
  });

  describe("when the choices differ", () => {
    test("is nothing", () => {
      assert.equal(commonAccess([{ access: "on" }, { access: "off" }]), undefined);
    });
  });

  describe("without choices", () => {
    test("is nothing", () => {
      assert.equal(commonAccess([]), undefined);
    });
  });
});

describe("accessCounts", () => {
  test("counts each access", () => {
    assert.deepEqual(accessCounts([{ access: "on" }, { access: "ask" }, { access: "on" }]), { off: 0, on: 2, ask: 1 });
  });
});

describe("asksOnce", () => {
  test("is true for a declared destructive function set on", () => {
    assert.equal(asksOnce({ capped: true }, "on"), true);
  });

  describe("when the function is not declared destructive", () => {
    test("is false, even with no class", () => {
      assert.equal(asksOnce({}, "on"), false);
    });
  });

  describe("when the function is not on", () => {
    test("is false", () => {
      assert.equal(asksOnce({ capped: true }, "ask"), false);
    });
  });
});
