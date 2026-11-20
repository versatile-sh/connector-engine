import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { Profile } from "../api.ts";
import { commonMode, overrideCount, profileAccess, profileHref, wider } from "./profiles.ts";

const custom: Profile = { agent: "beta", mode: "custom", overrides: { lookup: "off", file: "ask" } };

describe("profileAccess", () => {
  describe("for a key that follows", () => {
    test("is the default, not overridden", () => {
      assert.deepEqual(profileAccess({ agent: "alpha", mode: "follow" }, "lookup", "on"), { access: "on", overridden: false });
    });
  });

  describe("for a key that is off", () => {
    test("is off everywhere", () => {
      assert.deepEqual(profileAccess({ agent: "alpha", mode: "off" }, "lookup", "on"), { access: "off", overridden: false });
    });
  });

  describe("for a custom key", () => {
    test("is its narrowing", () => {
      assert.deepEqual(profileAccess(custom, "lookup", "on"), { access: "off", overridden: true });
    });

    test("is the default where it narrows nothing", () => {
      assert.deepEqual(profileAccess(custom, "send", "ask"), { access: "ask", overridden: false });
    });

    describe("when the default has dropped below its narrowing", () => {
      test("is the default, still overridden", () => {
        assert.deepEqual(profileAccess(custom, "file", "off"), { access: "off", overridden: true });
      });
    });
  });
});

describe("wider", () => {
  test("orders off, ask, on", () => {
    assert.equal(wider("on", "ask"), true);
    assert.equal(wider("ask", "on"), false);
    assert.equal(wider("off", "off"), false);
  });
});

describe("overrideCount", () => {
  test("counts a custom key's narrowings", () => {
    assert.equal(overrideCount(custom), 2);
  });

  test("is zero for a key that follows", () => {
    assert.equal(overrideCount({ agent: "alpha", mode: "follow" }), 0);
  });
});

describe("commonMode", () => {
  test("is the mode every key shares", () => {
    assert.equal(commonMode([{ agent: "a", mode: "off" }, { agent: "b", mode: "off" }]), "off");
  });

  describe("when a key is custom or they differ", () => {
    test("is nothing", () => {
      assert.equal(commonMode([custom, { agent: "a", mode: "follow" }]), undefined);
      assert.equal(commonMode([{ agent: "a", mode: "off" }, { agent: "b", mode: "follow" }]), undefined);
    });
  });
});

describe("profileHref", () => {
  test("names the key in the address", () => {
    assert.equal(profileHref("c 1", "claude code"), "/connectors/c%201?profile=claude%20code");
  });

  test("is the default page without a key", () => {
    assert.equal(profileHref("c1"), "/connectors/c1");
  });
});
