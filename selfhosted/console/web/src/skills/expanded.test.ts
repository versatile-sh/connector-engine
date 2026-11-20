import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { expandedOf, skillsHref } from "./expanded.ts";

const skills = [{ id: "pdf" }, { id: "docx" }];

describe("expandedOf", () => {
  test("opens an installed skill named in the query", () => assert.equal(expandedOf(new URLSearchParams("skill=docx"), skills), "docx"));
  describe("with a skill that is not installed", () => {
    test("opens nothing", () => assert.equal(expandedOf(new URLSearchParams("skill=gone"), skills), undefined));
  });
  describe("without the parameter", () => {
    test("opens nothing", () => assert.equal(expandedOf(new URLSearchParams("oauth=connected"), skills), undefined));
  });
});

describe("skillsHref", () => {
  describe("when a skill opens", () => {
    test("names it in the query", () => assert.equal(skillsHref(new URLSearchParams(), "pdf"), "/skills?skill=pdf"));
    test("keeps the rest of the query", () => assert.equal(skillsHref(new URLSearchParams("oauth=connected"), "pdf"), "/skills?oauth=connected&skill=pdf"));
    test("is read back by expandedOf", () => {
      const id = "a b/c&d";
      assert.equal(expandedOf(new URL(skillsHref(new URLSearchParams(), id), "http://x").searchParams, [{ id }]), id);
    });
  });
  describe("when the dialog closes", () => {
    test("drops the parameter", () => assert.equal(skillsHref(new URLSearchParams("skill=pdf")), "/skills"));
    test("keeps the rest of the query", () => assert.equal(skillsHref(new URLSearchParams("oauth=connected&skill=pdf")), "/skills?oauth=connected"));
    test("opens nothing once read back", () => assert.equal(expandedOf(new URL(skillsHref(new URLSearchParams("skill=pdf")), "http://x").searchParams, skills), undefined));
  });
});
