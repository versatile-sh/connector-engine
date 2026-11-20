import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { guideVisible } from "./visibility.ts";

const open = { ready: true, dismissed: false, done: 1, total: 3 };

describe("guideVisible", () => {
  test("shows while a step is left", () => {
    assert.equal(guideVisible(open), true);
  });

  test("waits for the data before showing", () => {
    assert.equal(guideVisible({ ...open, ready: false }), false);
  });

  describe("after a reload", () => {
    test("stays hidden once dismissed, since the dismissal comes back from the server", () => {
      assert.equal(guideVisible({ ...open, dismissed: true }), false);
    });

    test("stays hidden when every step is done, without a click", () => {
      assert.equal(guideVisible({ ...open, done: 3 }), false);
    });
  });
});
