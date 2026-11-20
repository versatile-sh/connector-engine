import { describe, test } from "node:test";
import { createElement } from "react";
import { App } from "./App.tsx";
import { ONE_OWNER, pageData } from "./testing/fixtures.ts";
import { renderMarkup } from "./testing/render.ts";

const PAGES = ["/", "/connectors/c1", "/access", "/approvals", "/activity", "/notifications", "/skills"];

describe("a one-user install", () => {
  for (const path of PAGES) {
    test(`draws ${path} as before multi-user`, (t) => {
      t.assert.snapshot(renderMarkup(createElement(App), { path, data: pageData(ONE_OWNER) }));
    });
  }
});
