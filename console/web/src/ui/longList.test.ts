import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { LONG_LIST_FROM, shownOf } from "./longList.ts";

const rows = Array.from({ length: 12 }, (_, i) => `key-${i + 1}`);
const same = (row: string) => row;

describe("shownOf", () => {
  describe("with a short list", () => {
    test("shows every row", () => {
      assert.deepEqual(shownOf(rows.slice(0, LONG_LIST_FROM), same, { filter: "", all: false }), { shown: rows.slice(0, LONG_LIST_FROM), hidden: 0 });
    });
  });

  describe("with a long list", () => {
    test("shows the first rows and counts the rest", () => {
      assert.deepEqual(shownOf(rows, same, { filter: "", all: false }), { shown: rows.slice(0, LONG_LIST_FROM), hidden: 6 });
    });

    test("shows every row once asked", () => {
      assert.equal(shownOf(rows, same, { filter: "", all: true }).shown.length, 12);
    });
  });

  describe("with a filter", () => {
    test("shows every match, ignoring case and spaces", () => {
      assert.deepEqual(shownOf(rows, same, { filter: "  KEY-1 ", all: false }), { shown: ["key-1", "key-10", "key-11", "key-12"], hidden: 0 });
    });

    test("shows nothing when nothing matches", () => {
      assert.deepEqual(shownOf(rows, same, { filter: "cursor", all: true }), { shown: [], hidden: 0 });
    });
  });
});
