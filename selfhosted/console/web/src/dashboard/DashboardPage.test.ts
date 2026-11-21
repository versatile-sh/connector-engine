import assert from "node:assert/strict";
import { describe, mock, test } from "node:test";
import type { QueryKey } from "@tanstack/react-query";
import { createElement } from "react";
import type { Session } from "../api.ts";
import { ONE_OWNER, QUIET, SUMMARY, TEAM_MEMBER, TEAM_OWNER, pageData } from "../testing/fixtures.ts";
import { renderMarkup } from "../testing/render.ts";
import { DashboardPage } from "./DashboardPage.tsx";

mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") });

type Overrides = readonly (readonly [QueryKey, unknown])[];

const seeded = (session: Session, overrides: Overrides = []) => {
  const names = new Set(overrides.map(([key]) => JSON.stringify(key)));
  return [...pageData(session).filter(([key]) => !names.has(JSON.stringify(key))), ...overrides];
};

const draw = (session: Session, overrides: Overrides = [], path = "/") => renderMarkup(createElement(DashboardPage), { path, data: seeded(session, overrides) });

const without = (key: QueryKey): Overrides => [[key, undefined]];

const count = (html: string, text: string) => html.split(text).length - 1;

describe("DashboardPage", () => {
  describe("with calls in the week", () => {
    const html = draw(ONE_OWNER);

    test("holds the page's one h1 in its header", () => {
      assert.equal(count(html, "<h1"), 1);
      assert.match(html, /^<section aria-labelledby="page-title" class="flex flex-col gap-6"><header class="mb-2 text-center">/);
    });

    test("draws four stat cards as equal cells of one strip", () => {
      assert.match(html, /<section aria-label="At a glance" class="grid gap-6 @xl:grid-cols-2 @4xl:grid-cols-4">/);
      assert.equal(count(html, "min-h-36"), 4);
    });

    test("counts the last day's calls against the day before", () => {
      assert.match(html, /Calls, last 24 hours<\/span>.*?>3<\/span>.*?\+50% on the day before/);
    });

    test("counts what failed in the last day, with a care mark", () => {
      assert.match(html, /Failed or denied, last 24 hours.*?>1<\/span>.*?bg-amber-400.*?33% of the day&#x27;s calls/);
    });

    test("counts the calls held for approval and links to them", () => {
      assert.match(html, /<a href="\/approvals"[^>]*>.*?Waiting for approval.*?>1<\/span>.*?bg-lime.*?Open Approvals to answer/);
    });

    test("counts connectors and keys in use, each linking to its page", () => {
      assert.match(html, /<a href="\/connectors"[^>]*>.*?>1<\/span>.*?connector<\/span><\/a>/);
      assert.match(html, /<a href="\/access"[^>]*>.*?>1<\/span>.*?access key<\/span><\/a>/);
    });

    test("draws a chart of the same calls the stat card counts", () => {
      assert.match(html, /3 calls; at the peak, 2 per hour\./);
    });

    test("draws the calls chart with its range toggle on the 24 hours", () => {
      assert.match(html, /<figure aria-labelledby="chart-title"/);
      assert.match(html, /aria-label="Chart range"/);
      assert.match(html, /aria-pressed="true"[^>]*>24 hours<\/button>/);
    });

    test("ranks the top connectors and keys on the week, never with numerals", () => {
      assert.match(html, /<h2 id="top-connectors"[^>]*>Top connectors<\/h2>/);
      assert.match(html, /<h2 id="top-keys"[^>]*>Top access keys<\/h2>/);
      assert.equal(count(html, "<ol"), 1);
      assert.match(html, /href="\/activity\?range=7d&amp;connector=c1"/);
      assert.match(html, /href="\/activity\?range=7d&amp;agent=laptop"/);
    });

    test("shows calls to a connector that is not installed without a link", () => {
      assert.match(html, /gone \(not installed\)/);
      assert.equal(count(html, "connector="), 1);
    });

    test("lists the latest calls with a link to all activity", () => {
      assert.match(html, /<h2 id="recent-title"[^>]*>Latest calls<\/h2>/);
      assert.match(html, /<a href="\/activity\?range=7d"[^>]*>All activity/);
      assert.match(html, /list_messages/);
    });

    test("shows no first-run guide once it is dismissed", () => {
      assert.doesNotMatch(html, /Get set up/);
    });

    test("shows no skeleton", () => {
      assert.doesNotMatch(html, /shimmer/);
    });
  });

  describe("with no call in the week", () => {
    const html = draw(ONE_OWNER, [[["activity", "summary", "mine"], QUIET]]);

    test("says so once and offers to connect an app", () => {
      assert.match(html, /No calls in the last <span class="text-lime">week<\/span>/);
      assert.match(html, /<button type="button"[^>]*>Connect an app<\/button>/);
      assert.equal(count(html, "Connect an app"), 1);
    });

    test("draws no stat card, chart or ranking, so no wall of zeros", () => {
      assert.doesNotMatch(html, /At a glance|<figure|Top connectors|Top access keys|Latest calls/);
    });
  });

  describe("on the first read", () => {
    const html = draw(ONE_OWNER, without(["activity", "summary", "mine"]));

    test("draws the dashboard's shape and neither numbers nor the empty state", () => {
      assert.match(html, /<div aria-hidden="true" class="@container flex flex-col gap-6">/);
      assert.match(html, /shimmer/);
      assert.doesNotMatch(html, /At a glance|No calls in the last/);
    });
  });

  describe("for a viewer who may see everyone's calls", () => {
    test("offers Mine or Everyone in a toolbar over the page", () => {
      const html = draw(TEAM_OWNER);
      assert.match(html, /<\/header><div class="flex min-w-0 flex-wrap items-center justify-center gap-2 sm:flex-nowrap"><div role="group" aria-label="Whose rows"/);
    });

    test("reads the everyone summary once Everyone is asked for", () => {
      const html = draw(TEAM_OWNER, [[["activity", "summary", "everyone"], { ...SUMMARY, total: 9, day: { total: 8, failed: 0 } }]], "/?scope=everyone");
      assert.match(html, /Calls, last 24 hours<\/span>.*?>8<\/span>/);
    });
  });

  describe("for a viewer looking at everyone's calls", () => {
    const html = draw(TEAM_OWNER, [], "/?scope=everyone");

    test("opens the activity, the approvals and the keys on everyone's too", () => {
      assert.match(html, /<a href="\/activity\?scope=everyone"/);
      assert.match(html, /<a href="\/approvals\?scope=everyone"/);
      assert.match(html, /<a href="\/access\?scope=everyone"/);
    });

    test("leaves the connectors link as it is, for connectors are not scoped", () => {
      assert.match(html, /<a href="\/connectors"/);
    });
  });

  describe("for a viewer looking at their own calls", () => {
    test("opens each page with no scope in the address", () => {
      const html = draw(TEAM_OWNER);
      assert.doesNotMatch(html, /scope=everyone"/);
    });
  });

  describe("for a Member", () => {
    const html = draw(TEAM_MEMBER);

    test("offers no scope switch", () => {
      assert.doesNotMatch(html, /Whose rows/);
    });

    test("counts only their own summary, even when the address asks for everyone", () => {
      const asked = draw(TEAM_MEMBER, [[["activity", "summary", "mine"], { ...SUMMARY, day: { total: 4, failed: 0 } }], [["activity", "summary", "everyone"], { ...SUMMARY, day: { total: 99, failed: 0 } }]], "/?scope=everyone");
      assert.match(asked, /Calls, last 24 hours<\/span>.*?>4<\/span>/);
      assert.doesNotMatch(asked, />99</);
    });

    test("shows no first-run guide", () => {
      const open = { ...TEAM_MEMBER, onboarding_dismissed: false };
      assert.doesNotMatch(draw(open, [[["connectors"], { connectors: [] }], [["agents"], { agents: [], max_ttl_hours: 720 }]]), /Get set up/);
    });
  });

  describe("for an owner whose setup is not finished", () => {
    const open = { ...ONE_OWNER, onboarding_dismissed: false };
    const html = draw(open, [[["connectors"], { connectors: [] }], [["agents"], { agents: [], max_ttl_hours: 720 }]]);

    test("draws the first-run guide above the numbers", () => {
      assert.match(html, /Get set up/);
      assert.ok(html.indexOf("Get set up") < html.indexOf("At a glance"));
    });
  });
});
