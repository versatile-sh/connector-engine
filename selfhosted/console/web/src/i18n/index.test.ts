import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { format, paramsOf } from "./index.ts";

const en = new Intl.PluralRules("en");
const pt = new Intl.PluralRules("pt-BR");

const FUNCTIONS = "{count, plural, =0 {no functions} one {# function} other {# functions}}";

describe("format", () => {
  describe("with a plural", () => {
    test("takes the one branch for one", () => {
      assert.equal(format(FUNCTIONS, { count: 1 }, en), "1 function");
    });

    test("takes the other branch for many", () => {
      assert.equal(format(FUNCTIONS, { count: 12 }, en), "12 functions");
    });

    test("takes an exact branch before the rule", () => {
      assert.equal(format(FUNCTIONS, { count: 0 }, en), "no functions");
    });

    test("follows the locale's rule", () => {
      assert.equal(format("{count, plural, one {# chave} other {# chaves}}", { count: 0 }, pt), "0 chave");
    });

    test("falls back to other when the rule's branch is missing", () => {
      assert.equal(format("{count, plural, other {# keys}}", { count: 1 }, en), "1 keys");
    });

    test("fills the placeholders around it", () => {
      assert.equal(format("{mode} drops {count, plural, one {the override} other {the # overrides}} of {key}", { mode: "Off", count: 2, key: "cursor" }, en), "Off drops the 2 overrides of cursor");
    });

    test("fills many plurals in one message", () => {
      assert.equal(format("{a, plural, one {# on} other {# on}} · {b, plural, one {# key} other {# keys}}", { a: 3, b: 1 }, en), "3 on · 1 key");
    });

    test("stays as written without its number", () => {
      assert.equal(format(FUNCTIONS, {}, en), FUNCTIONS);
    });
  });

  test("leaves an unknown placeholder as written", () => {
    assert.equal(format("Hi {name}", undefined, en), "Hi {name}");
  });
});

describe("paramsOf", () => {
  test("names each plural number once with the plain placeholders", () => {
    assert.deepEqual(paramsOf("{key}: {count, plural, one {# key} other {# keys}} and {count, plural, one {#} other {#}}"), ["count", "key"]);
  });

  test("never reads a branch as a placeholder", () => {
    assert.deepEqual(paramsOf("{count, plural, one {key} other {keys}}"), ["count"]);
  });
});
