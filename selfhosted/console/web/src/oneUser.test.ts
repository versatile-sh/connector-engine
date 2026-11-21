import { before, describe, mock, test } from "node:test";
import { createElement } from "react";
import { App } from "./App.tsx";
import { ONE_OWNER, pageData } from "./testing/fixtures.ts";
import { renderMarkup } from "./testing/render.ts";

const stableIds = (html: string) => html.replace(/_R_[0-9a-z]+_/g, "_R_");

const PAGES = ["/", "/connectors/c1", "/access", "/approvals", "/activity", "/notifications", "/skills"];

describe("a one-user install", () => {
  before(() => mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") }));

  for (const path of PAGES) {
    test(`draws ${path} as before multi-user`, (t) => {
      t.assert.snapshot(stableIds(renderMarkup(createElement(App), { path, data: pageData(ONE_OWNER) })));
    });
  }
});
