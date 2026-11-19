import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { en } from "./en.ts";
import { es } from "./es.ts";
import { ptBR } from "./pt-br.ts";

const others = { "pt-BR": ptBR, es } as const;
const placeholders = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();

describe("dictionaries", () => {
  for (const [locale, dictionary] of Object.entries(others)) {
    test(`${locale} has the same keys as English`, () => {
      assert.deepEqual(Object.keys(dictionary).sort(), Object.keys(en).sort());
    });

    test(`${locale} keeps every placeholder of English`, () => {
      for (const [key, text] of Object.entries(en)) assert.deepEqual(placeholders(dictionary[key as keyof typeof en]), placeholders(text), key);
    });

    test(`${locale} has no empty message`, () => {
      for (const [key, text] of Object.entries(dictionary)) assert.notEqual(text.trim(), "", key);
    });
  }

  test("keep the accents", () => {
    assert.equal(ptBR["nav.approvals"], "Aprovações");
    assert.equal(es["auth.line2"], "en tu máquina.");
  });
});
