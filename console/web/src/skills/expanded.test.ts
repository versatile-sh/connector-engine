import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { expandedOf, skillsHref, toggled } from "./expanded.ts";

const skills = [{ id: "pdf" }, { id: "docx" }];

describe("expandedOf", () => {
  test("reads an installed skill from the query", () => assert.equal(expandedOf(new URLSearchParams("skill=docx"), skills), "docx"));
  describe("with a skill that is not installed", () => {
    test("maximizes none", () => assert.equal(expandedOf(new URLSearchParams("skill=gone"), skills), undefined));
  });
  describe("without the parameter", () => {
    test("maximizes none", () => assert.equal(expandedOf(new URLSearchParams("oauth=connected"), skills), undefined));
  });
});

describe("skillsHref", () => {
  test("puts the maximized card in the query", () => assert.equal(skillsHref(new URLSearchParams(), "pdf"), "/skills?skill=pdf"));
  test("replaces the card maximized before", () => assert.equal(skillsHref(new URLSearchParams("skill=pdf"), "docx"), "/skills?skill=docx"));
  test("drops the parameter when every card is compact", () => assert.equal(skillsHref(new URLSearchParams("skill=pdf")), "/skills"));
  test("keeps the rest of the query", () => assert.equal(skillsHref(new URLSearchParams("oauth=connected&skill=pdf")), "/skills?oauth=connected"));
  test("is read back by expandedOf", () => {
    const id = "a b/c&d";
    assert.equal(expandedOf(new URL(skillsHref(new URLSearchParams(), id), "http://x").searchParams, [{ id }]), id);
  });
});

describe("toggled", () => {
  describe("with no card maximized", () => {
    test("maximizes the pressed one", () => assert.equal(toggled(undefined, "pdf"), "pdf"));
  });
  describe("with another card maximized", () => {
    test("moves to the pressed one, so one card at most is maximized", () => assert.equal(toggled("pdf", "docx"), "docx"));
  });
  describe("with the pressed card maximized", () => {
    test("makes every card compact", () => assert.equal(toggled("pdf", "pdf"), undefined));
  });
});
