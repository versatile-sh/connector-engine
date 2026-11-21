import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { createElement } from "react";
import { renderMarkup } from "../testing/render.ts";
import { AccessBar } from "./AccessBar.tsx";

describe("AccessBar", () => {
  const html = renderMarkup(createElement(AccessBar, { counts: { on: 2, ask: 1, off: 1 } }));

  test("is a 2px hairline on the line track", () => {
    assert.match(html, /^<div aria-hidden="true" class="flex h-0\.5 overflow-hidden rounded-full bg-line /);
  });

  test("draws on in paper at 40%, never brighter", () => {
    assert.match(html, /<span class="bg-paper\/40" style="width:50%">/);
  });

  test("draws ask in amber at 50%", () => {
    assert.match(html, /<span class="bg-amber-400\/50" style="width:25%">/);
  });
});
