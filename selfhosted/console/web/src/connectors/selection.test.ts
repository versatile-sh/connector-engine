import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { PRESETS, pickedOf, presetNames, toggledAll, toggledOne, widestOf } from "./selection.ts";

describe("pickedOf", () => {
  test("is all, some or none of the names", () => {
    assert.equal(pickedOf(new Set(["a", "b"]), ["a", "b"]), "all");
    assert.equal(pickedOf(new Set(["a"]), ["a", "b"]), "some");
    assert.equal(pickedOf(new Set(["c"]), ["a", "b"]), "none");
  });

  describe("without names", () => {
    test("is none", () => {
      assert.equal(pickedOf(new Set(["a"]), []), "none");
    });
  });
});

describe("toggledAll", () => {
  test("picks every name when not all are picked, keeping picks outside them", () => {
    assert.deepEqual([...toggledAll(new Set(["a", "z"]), ["a", "b"])].sort(), ["a", "b", "z"]);
  });

  test("unpicks the names when all are picked", () => {
    assert.deepEqual([...toggledAll(new Set(["a", "b", "z"]), ["a", "b"])], ["z"]);
  });
});

describe("toggledOne", () => {
  test("flips one name", () => {
    assert.deepEqual([...toggledOne(new Set(["a"]), "b")].sort(), ["a", "b"]);
    assert.deepEqual([...toggledOne(new Set(["a"]), "a")], []);
  });
});

describe("widestOf", () => {
  test("is the widest default picked", () => {
    assert.equal(widestOf(["off", "ask"]), "ask");
    assert.equal(widestOf(["ask", "on", "off"]), "on");
  });

  test("is nothing without rows", () => {
    assert.equal(widestOf([]), undefined);
  });
});

describe("presetNames", () => {
  test("names the functions of the preset's kind", () => {
    const functions = [
      { name: "read_file", kind: "read" as const },
      { name: "write_file", kind: "write" as const },
      { name: "delete_file", kind: "destructive" as const },
    ];
    assert.deepEqual(
      PRESETS.map((preset) => presetNames(functions, preset)),
      [["read_file"], ["write_file"], ["delete_file"]],
    );
  });

  test("leaves a function whose server declares nothing out of every preset", () => {
    const functions = [{ name: "sharepoint_search", kind: "undeclared" as const }];
    assert.deepEqual(
      PRESETS.map((preset) => presetNames(functions, preset)),
      [[], [], []],
    );
  });
});
