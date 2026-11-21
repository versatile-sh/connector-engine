import assert from "node:assert/strict";
import { describe, mock, test } from "node:test";
import { createElement, type ReactElement } from "react";
import { AccessPage } from "./access/AccessPage.tsx";
import { ActivityPage } from "./activity/ActivityPage.tsx";
import { ApprovalsPage } from "./approvals/ApprovalsPage.tsx";
import { ConnectorsPage } from "./connectors/ConnectorsPage.tsx";
import { NotificationsPage } from "./notifications/NotificationsPage.tsx";
import { SkillsPage } from "./skills/SkillsPage.tsx";
import { CONNECTOR, ONE_OWNER, pageData } from "./testing/fixtures.ts";
import { renderMarkup } from "./testing/render.ts";

mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") });

const many = (count: number) => ({ connectors: Array.from({ length: count }, (_, at) => ({ ...CONNECTOR, id: `c${at + 1}`, alias: `app${at + 1}`, title: `App ${at + 1}` })) });

const withConnectors = (count: number) => pageData(ONE_OWNER).map(([key, value]) => (key[0] === "connectors" ? ([key, many(count)] as const) : ([key, value] as const)));

const draw = (element: ReactElement, count = 1) => renderMarkup(element, { data: withConnectors(count) });

const between = (html: string, from: string, to: string) => html.slice(html.indexOf(from), html.indexOf(to, html.indexOf(from)));

const PAGES: readonly (readonly [string, ReactElement])[] = [
  ["Connectors", createElement(ConnectorsPage)],
  ["Skills", createElement(SkillsPage)],
  ["Access", createElement(AccessPage, { endpoint: "http://127.0.0.1:7465/mcp" })],
  ["Approvals", createElement(ApprovalsPage)],
  ["Activity", createElement(ActivityPage, { endpoint: "http://127.0.0.1:7465/mcp" })],
  ["Notifications", createElement(NotificationsPage)],
];

describe("the pages' layout", () => {
  for (const [name, page] of PAGES) {
    describe(`${name}`, () => {
      const html = draw(page);

      test("stands in one column with one gap", () => {
        assert.match(html, /^<section aria-labelledby="page-title" class="flex flex-col gap-6">/);
      });

      test("opens with the header and its one h1", () => {
        assert.match(html, /^<section[^>]*><header class="mb-2"><div class="flex flex-wrap items-baseline gap-x-3 gap-y-4"><h1 id="page-title"/);
        assert.equal(html.split("<h1").length - 1, 1);
      });

      test("draws no panel tray inside the shell's surface", () => {
        assert.doesNotMatch(html, /tray-fill/);
      });
    });
  }
});

describe("ConnectorsPage", () => {
  describe("with connectors installed", () => {
    const html = draw(createElement(ConnectorsPage), 2);

    test("puts the count and the Import action on the title's own line", () => {
      const head = between(html, "<header", "</header>");
      assert.match(head, /<\/h1><span class="rounded-full bg-paper\/10[^"]*">2<\/span><div class="flex flex-wrap items-center gap-2 sm:ml-auto"><button[^>]*>.*Import<\/button><\/div>/);
    });

    test("fills the width with 18rem-or-wider columns", () => {
      assert.match(html, /<ul class="grid grid-cols-\[repeat\(auto-fill,minmax\(min\(100%,18rem\),1fr\)\)\] gap-6">/);
    });

    test("pins each card's access bar and facts to the card's foot", () => {
      assert.equal(html.split('<div class="mt-auto pt-4">').length - 1, 2);
    });

    test("shows no filter until a list is long", () => {
      assert.doesNotMatch(html, /type="search"/);
    });
  });

  describe("with a long list", () => {
    const html = draw(createElement(ConnectorsPage), 7);

    test("puts the filter in a toolbar row right under the header and over the grid", () => {
      const rest = html.slice(html.indexOf("</header>"));
      assert.match(rest, /^<\/header><div class="flex min-w-0 flex-wrap items-center gap-2 sm:flex-nowrap"><span[^>]*><svg.*?<input type="search"[^>]*><\/span><\/div><ul class="grid grid-cols-/);
    });

    test("lets the filter take the free width of its row, at most 24rem", () => {
      assert.match(html, /class="[^"]*min-w-0 flex-1 basis-48 sm:max-w-sm[^"]*"/);
    });
  });
});
