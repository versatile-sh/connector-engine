import assert from "node:assert/strict";
import { describe, mock, test } from "node:test";
import { createElement } from "react";
import type { Session } from "../api.ts";
import { App } from "../App.tsx";
import { ONE_OWNER, TEAM_MEMBER, TEAM_OWNER, pageData } from "../testing/fixtures.ts";
import { renderMarkup } from "../testing/render.ts";

mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") });

const page = (session: Session, path: string) => renderMarkup(createElement(App), { path, data: pageData(session) });

const count = (html: string, text: string) => html.split(text).length - 1;

const at = (html: string, text: string) => html.indexOf(text);

const CURRENT: readonly (readonly [string, string])[] = [
  ["/", "/"],
  ["/connectors/c1", "/"],
  ["/skills", "/skills"],
  ["/access", "/access"],
  ["/agents", "/access"],
  ["/approvals", "/approvals"],
  ["/activity", "/activity"],
  ["/notifications", "/notifications"],
  ["/account", "/account"],
];

describe("the shell", () => {
  describe("on each section's path", () => {
    for (const [path, link] of CURRENT) {
      test(`marks only ${link} as the current page on ${path}`, () => {
        const html = page(ONE_OWNER, path);
        assert.equal(count(html, 'aria-current="page"'), 1);
        assert.equal(count(html, `<a href="${link}" aria-current="page"`), 1);
      });
    }

    test("marks Account on one of its tabs", () => {
      assert.equal(count(page(TEAM_OWNER, "/account/people"), '<a href="/account" aria-current="page"'), 1);
    });

    test("holds the page's own title as the only h1", () => {
      for (const [path] of CURRENT) assert.equal(count(page(ONE_OWNER, path), "<h1"), 1, path);
    });

    test("draws no eyebrow over the title", () => {
      assert.equal(count(page(ONE_OWNER, "/activity"), "connector gateway ·"), 0);
    });
  });

  describe("as landmarks", () => {
    const html = page(ONE_OWNER, "/");

    test("names the sidebar's links as the Console nav", () => {
      assert.equal(count(html, '<nav aria-label="Console"'), 1);
    });

    test("puts the skip link before the sidebar and aims it at main", () => {
      assert.ok(at(html, 'href="#main"') >= 0 && at(html, 'href="#main"') < at(html, '<nav aria-label="Console"'));
      assert.equal(count(html, '<main id="main" tabindex="-1"'), 1);
    });

    test("puts the strip with the endpoint beside the content, not above the sidebar", () => {
      assert.ok(at(html, '<nav aria-label="Console"') < at(html, "Copy the endpoint http://127.0.0.1:7465/mcp"));
      assert.equal(count(html, "Copy the endpoint http://127.0.0.1:7465/mcp"), 1);
    });
  });

  describe("at rest on a wide screen", () => {
    const html = page(ONE_OWNER, "/");
    const sidebar = /aria-label="Open the menu" aria-controls="([^"]+)" aria-expanded="false"/.exec(html)?.[1] ?? "";

    test("ties the menu button to the sidebar it opens", () => {
      assert.notEqual(sidebar, "");
      assert.equal(count(html, `<div id="${sidebar}"`), 1);
    });

    test("ties the collapse handle to the same sidebar", () => {
      assert.equal(count(html, `aria-controls="${sidebar}" aria-label="Collapse the sidebar"`), 1);
    });

    test("is not a dialog and leaves the page live", () => {
      assert.equal(count(html, 'role="dialog"'), 0);
      assert.equal(count(html, 'aria-modal="true"'), 0);
      assert.equal(count(html, "inert"), 0);
    });
  });

  describe("with calls held for approval", () => {
    test("counts them on Approvals alone", () => {
      const html = page(ONE_OWNER, "/");
      assert.equal(count(html, "1 waiting"), 1);
      assert.ok(at(html, "1 waiting") > at(html, 'href="/approvals"') && at(html, "1 waiting") < at(html, 'href="/activity"'));
    });
  });

  describe("for the signed-in user", () => {
    test("names them in the footer's menu button", () => {
      assert.equal(count(page(ONE_OWNER, "/"), 'aria-label="alice, account menu" aria-expanded="false"'), 1);
    });

    test("keeps the role out of a one-user install", () => {
      assert.equal(count(page(ONE_OWNER, "/"), 'title="Your role"'), 0);
    });

    test("shows a Member their role once", () => {
      const html = page(TEAM_MEMBER, "/");
      assert.equal(count(html, 'aria-label="bob, account menu" aria-expanded="false"'), 1);
      assert.equal(count(html, 'title="Your role"'), 1);
    });

    test("offers the language and sign-out beside the user", () => {
      const html = page(ONE_OWNER, "/");
      assert.equal(count(html, 'aria-label="Language"'), 1);
      assert.equal(count(html, ">Sign out<"), 1);
    });
  });
});
