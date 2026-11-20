import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { CLOSED, isMove, listbox, matching, placement, TYPEAHEAD_MS, type ListState } from "./select.ts";

const open = (active = 0): ListState => ({ ...CLOSED, open: true, active });

const texts = ["Every key", "claude-code", "cursor", "codex", "windsurf"];

const type = (state: ListState, char: string, at: number) => listbox(state, { type: "type", char, at, texts });

describe("listbox", () => {
  describe("when it opens", () => {
    test("lands on the chosen row", () => {
      assert.equal(listbox(CLOSED, { type: "open", selected: 3 }).active, 3);
    });

    test("lands on the first row with none chosen", () => {
      assert.equal(listbox(CLOSED, { type: "open", selected: -1 }).active, 0);
    });

    test("starts with no filter and nothing typed", () => {
      assert.deepEqual(listbox({ ...open(2), query: "co", typed: "c", at: 10 }, { type: "open", selected: 1 }), { ...CLOSED, open: true, active: 1 });
    });
  });

  describe("when it closes", () => {
    test("forgets its row, filter and typing", () => {
      assert.deepEqual(listbox({ ...open(4), query: "co", typed: "c", at: 10 }, { type: "close" }), CLOSED);
    });
  });

  describe("when a key moves the row", () => {
    test("goes one row down", () => {
      assert.equal(listbox(open(1), { type: "move", key: "ArrowDown", count: 5 }).active, 2);
    });

    test("stops at the last row", () => {
      assert.equal(listbox(open(4), { type: "move", key: "ArrowDown", count: 5 }).active, 4);
    });

    test("goes one row up", () => {
      assert.equal(listbox(open(3), { type: "move", key: "ArrowUp", count: 5 }).active, 2);
    });

    test("stops at the first row", () => {
      assert.equal(listbox(open(0), { type: "move", key: "ArrowUp", count: 5 }).active, 0);
    });

    test("jumps to the first row on Home", () => {
      assert.equal(listbox(open(3), { type: "move", key: "Home", count: 5 }).active, 0);
    });

    test("jumps to the last row on End", () => {
      assert.equal(listbox(open(1), { type: "move", key: "End", count: 5 }).active, 4);
    });

    test("stays on the first row of an empty list", () => {
      assert.equal(listbox(open(0), { type: "move", key: "End", count: 0 }).active, 0);
    });
  });

  describe("when the pointer moves over a row", () => {
    test("makes it the row", () => {
      assert.equal(listbox(open(0), { type: "point", row: 3 }).active, 3);
    });

    test("keeps the same state over the same row", () => {
      const state = open(3);
      assert.equal(listbox(state, { type: "point", row: 3 }), state);
    });
  });

  describe("when the filter changes", () => {
    test("keeps the filter and starts on its first match", () => {
      assert.deepEqual(listbox(open(4), { type: "filter", query: "cu" }), { ...open(0), query: "cu" });
    });
  });

  describe("when a letter is typed", () => {
    test("jumps to the next row it starts", () => {
      assert.equal(type(open(0), "c", 1000).active, 1);
    });

    test("ignores case", () => {
      assert.equal(type(open(3), "E", 1000).active, 0);
    });

    test("keeps the row when no text starts with it", () => {
      assert.equal(type(open(2), "z", 1000).active, 2);
    });

    describe("again in quick succession", () => {
      test("cycles through the rows it starts", () => {
        const twice = type(type(open(0), "c", 1000), "c", 1100);
        assert.equal(type(twice, "c", 1200).active, 3);
      });
    });
  });

  describe("when letters are typed in quick succession", () => {
    test("jumps to the row their prefix starts", () => {
      assert.equal(type(type(open(0), "c", 1000), "u", 1100).active, 2);
    });
  });

  describe("when a letter comes after a pause", () => {
    test("starts a new search", () => {
      const first = type(type(open(0), "c", 1000), "u", 1100);
      assert.equal(type(first, "w", 1100 + TYPEAHEAD_MS).active, 4);
    });
  });
});

describe("isMove", () => {
  test("knows the arrows, Home and End", () => {
    assert.deepEqual(["ArrowDown", "ArrowUp", "Home", "End"].map(isMove), [true, true, true, true]);
  });

  test("rejects other keys", () => {
    assert.deepEqual(["Enter", "a", "ArrowLeft", "toString"].map(isMove), [false, false, false, false]);
  });
});

describe("matching", () => {
  const options = texts.map((text) => ({ text }));

  describe("without a filter", () => {
    test("keeps every option", () => {
      assert.deepEqual(matching(options, " "), options);
    });
  });

  describe("with a filter", () => {
    test("keeps the options holding it, ignoring case", () => {
      assert.deepEqual(matching(options, "CO"), [{ text: "claude-code" }, { text: "codex" }]);
    });

    test("keeps nothing when nothing holds it", () => {
      assert.deepEqual(matching(options, "zed"), []);
    });
  });
});

describe("placement", () => {
  const viewport = { width: 1000, height: 800 };
  const trigger = { left: 100, top: 100, width: 160, height: 36 };

  describe("with room under the trigger", () => {
    test("opens under it", () => {
      assert.deepEqual(placement(trigger, { width: 200, height: 300 }, viewport), { up: false, top: 140, left: 100, width: 200, room: 652 });
    });
  });

  describe("near the bottom of the viewport", () => {
    test("opens over the trigger", () => {
      assert.deepEqual(placement({ ...trigger, top: 700 }, { width: 200, height: 300 }, viewport), { up: true, bottom: 104, left: 100, width: 200, room: 688 });
    });
  });

  describe("when more room is under the trigger", () => {
    test("stays under it even if the list is taller", () => {
      assert.equal(placement({ ...trigger, top: 300 }, { width: 200, height: 600 }, viewport).up, false);
    });
  });

  describe("with a list narrower than the trigger", () => {
    test("is as wide as the trigger", () => {
      assert.equal(placement(trigger, { width: 90, height: 100 }, viewport).width, 160);
    });
  });

  describe("near the right edge of the viewport", () => {
    test("moves left to stay inside it", () => {
      assert.equal(placement({ ...trigger, left: 900 }, { width: 200, height: 100 }, viewport).left, 792);
    });
  });

  describe("with a list wider than the viewport", () => {
    test("fills it, keeping its edges", () => {
      const at = placement(trigger, { width: 1400, height: 100 }, viewport);
      assert.deepEqual([at.left, at.width], [8, 984]);
    });
  });
});
