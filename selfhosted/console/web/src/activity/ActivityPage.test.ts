import { before, describe, mock, test } from "node:test";
import { createElement } from "react";
import { App } from "../App.tsx";
import { ACTIVITY, ONE_OWNER, pageData, TEAM_ADMIN, TEAM_MEMBER } from "../testing/fixtures.ts";
import { renderMarkup } from "../testing/render.ts";

const withPages = (session: Parameters<typeof pageData>[0], next?: string) =>
  pageData(session).map((entry) => (entry[0][0] === "activity" ? ([entry[0], { pages: [{ ...ACTIVITY, ...(next ? { next_cursor: next } : {}) }], pageParams: [""] }] as const) : entry));

describe("the activity page", () => {
  before(() => mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") }));

  describe("when the server names a cursor", () => {
    test("offers to load more calls", (t) => {
      t.assert.match(renderMarkup(createElement(App), { path: "/activity", data: withPages(ONE_OWNER, "c1") }), /Load more calls/);
    });
  });

  describe("when the last page holds the end", () => {
    test("offers no more", (t) => {
      t.assert.doesNotMatch(renderMarkup(createElement(App), { path: "/activity", data: withPages(ONE_OWNER) }), /Load more calls/);
    });
  });

  describe("for an owner", () => {
    test("offers the retention and both deletions", (t) => {
      const html = renderMarkup(createElement(App), { path: "/activity", data: withPages(ONE_OWNER) });
      t.assert.match(html, /Keep calls for/);
      t.assert.match(html, /Delete a period/);
      t.assert.match(html, /Delete all activity/);
    });
  });

  describe("for an admin", () => {
    test("offers the retention and no deletion", (t) => {
      const html = renderMarkup(createElement(App), { path: "/activity", data: withPages(TEAM_ADMIN) });
      t.assert.match(html, /Keep calls for/);
      t.assert.doesNotMatch(html, /Delete a period|Delete all activity/);
    });
  });

  describe("for a member", () => {
    test("offers neither the retention nor a deletion", (t) => {
      const html = renderMarkup(createElement(App), { path: "/activity", data: withPages(TEAM_MEMBER) });
      t.assert.doesNotMatch(html, /Keep calls for|Delete a period|Delete all activity/);
    });
  });
});
