import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { en } from "./en.ts";
import { es } from "./es.ts";
import { paramsOf } from "./index.ts";
import { ptBR } from "./pt-br.ts";

const others = { "pt-BR": ptBR, es } as const;
const marks = (text: string) => text.split("*").length - 1;

describe("dictionaries", () => {
  for (const [locale, dictionary] of Object.entries(others)) {
    test(`${locale} has the same keys as English`, () => {
      assert.deepEqual(Object.keys(dictionary).sort(), Object.keys(en).sort());
    });

    test(`${locale} keeps every placeholder of English`, () => {
      for (const [key, text] of Object.entries(en)) assert.deepEqual(paramsOf(dictionary[key as keyof typeof en]), paramsOf(text), key);
    });

    test(`${locale} marks the lime word of the same titles as English`, () => {
      for (const [key, text] of Object.entries(en)) assert.equal(marks(dictionary[key as keyof typeof en]), marks(text), key);
    });

    test(`${locale} has no empty message`, () => {
      for (const [key, text] of Object.entries(dictionary)) assert.notEqual(text.trim(), "", key);
    });
  }

  test("end every plural with an other branch", () => {
    for (const dictionary of [en, ptBR, es])
      for (const [key, text] of Object.entries(dictionary)) for (const plural of text.matchAll(/\{\w+, plural,(.*?)\}\}/g)) assert.match(plural[1] ?? "", /other \{[^{}]*$/, key);
  });

  test("count nouns through a plural, never a bare number", () => {
    const bare = /\{(count|total|max|days)\} (functions|keys|calls|files|days|hours|minutes|bytes|errors|overrides|values)\b/;
    for (const [key, text] of Object.entries(en)) assert.doesNotMatch(text, bare, key);
  });

  test("close every lime mark it opens", () => {
    for (const dictionary of [en, ptBR, es]) for (const [key, text] of Object.entries(dictionary)) assert.equal(marks(text) % 2, 0, key);
  });

  test("keep the accents", () => {
    assert.equal(ptBR["nav.approvals"], "Aprovações");
    assert.equal(es["auth.line2"], "en tu máquina.");
  });
});
