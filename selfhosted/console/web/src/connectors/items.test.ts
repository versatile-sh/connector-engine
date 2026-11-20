import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { Item } from "../api";
import { accessChoices, groupItems } from "./items.ts";

const item = (name: string, kind: Item["kind"]): Item => ({ name, kind, label: name, preview: "", access: "on" });

describe("groupItems", () => {
  describe("with items of every kind", () => {
    test("splits them by kind in listed order", () => {
      const groups = groupItems([item("prompt:b", "prompt"), item("res:x", "resource"), item("instructions:server", "instructions"), item("prompt:a", "prompt")]);
      assert.deepEqual(
        groups.prompt.map((it) => it.name),
        ["prompt:b", "prompt:a"],
      );
      assert.deepEqual(
        groups.resource.map((it) => it.name),
        ["res:x"],
      );
      assert.deepEqual(
        groups.instructions.map((it) => it.name),
        ["instructions:server"],
      );
    });
  });

  describe("without a list", () => {
    test("returns empty groups", () => {
      assert.deepEqual(groupItems(null), { instructions: [], prompt: [], resource: [] });
      assert.deepEqual(groupItems(undefined), { instructions: [], prompt: [], resource: [] });
    });
  });
});

describe("accessChoices", () => {
  describe("when the item is the server instructions", () => {
    test("offers only off and on", () => {
      assert.deepEqual(accessChoices("instructions"), ["off", "on"]);
    });
  });

  describe("when the item is a prompt or a resource", () => {
    test("offers off, on and ask", () => {
      assert.deepEqual(accessChoices("prompt"), ["off", "on", "ask"]);
      assert.deepEqual(accessChoices("resource"), ["off", "on", "ask"]);
    });
  });
});
