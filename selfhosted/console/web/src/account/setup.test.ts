import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { SetupConnector } from "../api.ts";
import { capsOf, hostOf, keyMissing, optionsOf } from "./setup.ts";

const connector = (over: Partial<SetupConnector>): SetupConnector => ({ name: "github", functions: null, items: null, exists: false, ...over });

describe("hostOf", () => {
  test("gives the host and port", () => {
    assert.equal(hostOf("http://127.0.0.1:8080/mcp"), "127.0.0.1:8080");
  });

  describe("with an address that does not parse", () => {
    test("gives nothing", () => {
      assert.equal(hostOf("not a url"), undefined);
    });
  });
});

describe("capsOf", () => {
  test("pairs what the file asks with what is allowed", () => {
    const caps = capsOf(connector({ functions: { push: "on" }, items: { "prompt:x": "ask" }, above: { push: "ask", "prompt:x": "off" } }));
    assert.deepEqual(caps, [
      { name: "prompt:x", asks: "ask", allows: "off" },
      { name: "push", asks: "on", allows: "ask" },
    ]);
  });

  describe("without lowered choices", () => {
    test("gives none", () => {
      assert.deepEqual(capsOf(connector({})), []);
    });
  });
});

describe("optionsOf", () => {
  test("names the sets not kept", () => {
    const options = optionsOf({ a: { action: "keep" }, b: { action: "discard" }, c: { action: "apply", key: "cursor" } }, false);
    assert.deepEqual(options, { slots: { b: { action: "discard" }, c: { action: "apply", key: "cursor" } } });
  });

  describe("with the existing keys switch on", () => {
    test("asks for it", () => {
      assert.deepEqual(optionsOf({}, true), { existing_keys: true });
    });
  });

  describe("when every answer is the default", () => {
    test("gives nothing", () => {
      assert.equal(optionsOf({ a: { action: "keep" } }, false), undefined);
    });
  });
});

describe("keyMissing", () => {
  describe("when a set applies to no key yet", () => {
    test("is true", () => {
      assert.equal(keyMissing({ a: { action: "apply", key: "" } }), true);
    });
  });

  describe("when every applied set has a key", () => {
    test("is false", () => {
      assert.equal(keyMissing({ a: { action: "apply", key: "cursor" }, b: { action: "discard" } }), false);
    });
  });
});
