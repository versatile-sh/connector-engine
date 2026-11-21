import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { Connector, NotificationsData } from "../api.ts";
import { askedOf, decisionOf, previewOf, reachOf, relativeOf } from "./recent.ts";

const NOW = Date.parse("2026-11-20T12:00:00Z");

const connector = (id: string, title: string, access: readonly ("on" | "ask" | "off")[], items: readonly "ask"[] = []): Connector => ({
  id,
  alias: id,
  enabled: true,
  listing: "l",
  version: "1",
  title,
  functions: access.map((state, at) => ({ name: `fn${at}`, class: "read", kind: "read", state: "active", access: state })),
  items: items.map((state, at) => ({ name: `prompt${at}`, kind: "prompt", label: "p", preview: "", access: state })),
  profiles: [],
  slots: [],
});

describe("decisionOf", () => {
  test("reads an allowed call as approved whatever it did after", () => {
    for (const state of ["approved", "executing", "executed", "failed"]) assert.equal(decisionOf(state), "approved", state);
  });

  test("keeps denied, expired and cancelled as they are", () => {
    for (const state of ["denied", "expired", "cancelled"]) assert.equal(decisionOf(state), state);
  });

  test("gives a state it does not know no kind", () => {
    assert.equal(decisionOf("pending"), undefined);
    assert.equal(decisionOf("sideways"), undefined);
  });
});

describe("askedOf", () => {
  describe("with functions at Ask", () => {
    test("lists the connectors that have some, by title, with what asks", () => {
      const asked = askedOf([connector("c2", "Zed", ["ask", "on"]), connector("c1", "Alpha", ["ask", "ask", "off"]), connector("c3", "Quiet", ["on", "off"])]);
      assert.deepEqual(asked.map((row) => [row.title, row.names]), [["Alpha", ["fn0", "fn1"]], ["Zed", ["fn0"]]]);
    });

    test("counts items at Ask beside the functions", () => {
      assert.deepEqual(askedOf([connector("c1", "Alpha", ["on"], ["ask"])]).map((row) => row.names), [["prompt0"]]);
    });
  });

  describe("with nothing at Ask", () => {
    test("lists no connector", () => {
      assert.deepEqual(askedOf([connector("c1", "Alpha", ["on", "off"])]), []);
      assert.deepEqual(askedOf([]), []);
    });
  });

  test("copes with a connector that has not read its functions", () => {
    assert.deepEqual(askedOf([{ ...connector("c1", "Alpha", []), functions: null, items: undefined }]), []);
  });
});

describe("reachOf", () => {
  const data = (over: Partial<NotificationsData>): NotificationsData => ({ channels: [], devices: [], apps: [], kinds: [], ...over });

  test("counts the apps that ask in the AI app", () => {
    assert.equal(reachOf(data({ apps: [{ name: "a", elicit: true }, { name: "b", elicit: false }, { name: "c", elicit: true }] })).elicit, 2);
  });

  describe("with nothing set up but the console", () => {
    test("says nothing else reaches the viewer", () => assert.equal(reachOf(data({})).extra, false));
  });

  describe("with a paired computer", () => {
    test("says something else does", () => assert.equal(reachOf(data({ devices: [{ id: "d", name: "laptop", created_at: "2026-11-01T00:00:00Z" }] })).extra, true));
  });

  describe("with only an app that asks", () => {
    test("still says nothing but the console and the app", () => assert.equal(reachOf(data({ apps: [{ name: "a", elicit: true }] })).extra, false));
  });
});

describe("relativeOf", () => {
  const at = (ms: number) => new Date(NOW + ms).toISOString();

  test("reads a past instant in minutes", () => assert.equal(relativeOf(at(-5 * 60_000), NOW, "en"), "5 minutes ago"));
  test("reads a future instant in minutes", () => assert.equal(relativeOf(at(3 * 60_000), NOW, "en"), "in 3 minutes"));
  test("reads under a minute and a half in seconds", () => assert.equal(relativeOf(at(-30_000), NOW, "en"), "30 seconds ago"));
  test("reads hours past an hour and a half", () => assert.equal(relativeOf(at(-3 * 3_600_000), NOW, "en"), "3 hours ago"));
  test("reads days past a day and a half", () => assert.equal(relativeOf(at(-3 * 86_400_000), NOW, "en"), "3 days ago"));
  test("speaks the console's language", () => assert.equal(relativeOf(at(-5 * 60_000), NOW, "pt-BR"), "há 5 minutos"));
});

describe("previewOf", () => {
  describe("with an object of arguments", () => {
    test("lists the fields as name: value", () => assert.equal(previewOf({ to: "bob@example.com", subject: "Hi", count: 3 }), "to: bob@example.com, subject: Hi, count: 3"));
    test("writes a nested value as compact JSON", () => assert.equal(previewOf({ filter: { q: "x" } }), 'filter: {"q":"x"}'));
    test("folds line breaks into spaces", () => assert.equal(previewOf({ body: "one\n\ntwo" }), "body: one two"));
  });

  describe("with a long value", () => {
    test("cuts at the limit with an ellipsis", () => {
      const line = previewOf({ body: "x".repeat(300) }, 20);
      assert.equal(line.length, 20);
      assert.ok(line.endsWith("…"));
    });
  });

  describe("with no arguments", () => {
    test("gives nothing", () => {
      assert.equal(previewOf(undefined), "");
      assert.equal(previewOf(null), "");
      assert.equal(previewOf({}), "");
    });
  });

  test("keeps a plain string or an array as it is", () => {
    assert.equal(previewOf("hello"), "hello");
    assert.equal(previewOf([1, 2]), "[1,2]");
  });
});
