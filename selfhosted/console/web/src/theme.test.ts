import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { REVEAL_OVERSHOOT, otherTheme, pickTheme, resolveTheme, revealRadius } from "./theme.ts";

describe("pickTheme", () => {
  test("keeps a saved light or dark", () => {
    assert.deepEqual([pickTheme("light"), pickTheme("dark")], ["light", "dark"]);
  });

  test("returns null with nothing or junk saved", () => {
    assert.deepEqual([pickTheme(null), pickTheme("system"), pickTheme("purple")], [null, null, null]);
  });
});

describe("resolveTheme", () => {
  test("with nothing saved, follows what the system prefers", () => {
    assert.deepEqual([resolveTheme(null, true), resolveTheme(null, false)], ["light", "dark"]);
  });

  test("with a saved theme, ignores the system", () => {
    assert.deepEqual([resolveTheme("dark", true), resolveTheme("light", false)], ["dark", "light"]);
  });
});

describe("otherTheme", () => {
  test("flips light and dark", () => {
    assert.deepEqual([otherTheme("light"), otherTheme("dark")], ["dark", "light"]);
  });
});

describe("revealRadius", () => {
  test("from the top left ends past the far corner", () => {
    assert.equal(revealRadius({ x: 0, y: 0 }, 300, 400), 500 * REVEAL_OVERSHOOT);
  });

  test("from the centre ends past every corner", () => {
    assert.equal(revealRadius({ x: 150, y: 200 }, 300, 400), 250 * REVEAL_OVERSHOOT);
  });

  test("always ends well beyond the screen, so the edge never crawls over the last corner", () => {
    assert.ok(REVEAL_OVERSHOOT >= 1.4);
  });
});
