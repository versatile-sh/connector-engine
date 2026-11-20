import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, test } from "node:test";

const ROOT = join(import.meta.dirname, "..");

const sources = (dir: string): string[] =>
  readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.tsx?$/.test(entry.name) && !entry.name.endsWith(".test.ts") ? [path] : [];
  });

const files = sources(ROOT).map((path) => ({ name: relative(ROOT, path), text: readFileSync(path, "utf8") }));

const using = (pattern: RegExp) => files.filter((file) => pattern.test(file.text)).map((file) => file.name).sort();

describe("the console's design vocabulary", () => {
  test("sets the serif only in Display and the logotype", () => {
    assert.deepEqual(using(/\bfont-serif\b/), ["Brand.tsx", "ui/Display.tsx"]);
  });

  test("draws no speech-bubble shapes", () => {
    assert.deepEqual(using(/(?<![\w-])bubble(-word|-sent)?(?![\w-])/), []);
  });

  test("uses no type tokens outside the theme", () => {
    assert.deepEqual(using(/\bfont-(display|bold|black|extrabold)\b/), []);
  });

  test("draws every dropdown with Select, never the native select or datalist", () => {
    assert.deepEqual(using(/<(select|datalist|option)\b/), []);
  });

  test("draws no native date or time field, whose picker the system paints", () => {
    assert.deepEqual(using(/type="(date|datetime-local|month|week|time)"/), []);
  });

  test("fills with lime only a dot, the filter mark or the caret", () => {
    const shapes = /\bsize-1\.5\b|\bw-0\.5\b|\bw-\[0\.42em\]/;
    const fills = files.flatMap((file) => file.text.split("\n").filter((line) => /\b(bg|fill)-lime\b/.test(line) && !shapes.test(line)).map((line) => `${file.name}: ${line.trim()}`));
    assert.deepEqual(fills, []);
  });

  test("uses no colour or effect class the stylesheet no longer defines", () => {
    assert.deepEqual(using(/\b(text|bg|border|fill|stroke|outline|ring|decoration|from|via|to)-amber(?![\w-])|\bbackdrop-(grid|beam|glow)\b|\bride-in\b/), []);
  });
});
