import { describe, test } from "node:test";
import { createElement } from "react";
import type { ImportSource, Preview } from "../api";
import { renderMarkup } from "../testing/render.ts";
import { ImportLanding } from "./ImportLanding.tsx";

const COMMIT = "57217cf86ba38148a57af9e90d14a4c049558330";
const SKILL_SOURCE: ImportSource = { listing: "https://versatile.sh/skills/catalog/find-skills.yaml" };
const CONNECTOR_SOURCE: ImportSource = { listing: "https://versatile.sh/connectors/catalog/mail.yaml" };

const SKILL: Preview = {
  kind: "skill",
  url: "https://versatile.sh/skills/catalog/find-skills.yaml",
  host: "versatile.sh",
  name: "find-skills",
  title: "Find Skills",
  description: "Helps you discover and install skills when you ask how to do something.",
  publisher: "vercel-labs",
  version: COMMIT,
  tools: [],
  items: [],
  skill: {
    name: "find-skills",
    source: { repository: "https://github.com/vercel-labs/skills", path: "skills/find-skills", commit: COMMIT },
    licence: { spdx: "MIT", redistributable: true },
    redistribution: "bundled",
    has_scripts: false,
    files: [{ path: "SKILL.md", bytes: 1200 }],
    keys: ["claude"],
    needs: { tools: [], runtimes: [], network: false, mcp: [] },
    compat: [],
    warnings: [],
  },
};

const CONNECTOR: Preview = {
  kind: "listing",
  url: "https://versatile.sh/connectors/catalog/mail.yaml",
  host: "versatile.sh",
  name: "mail",
  title: "Mail",
  description: "Read and send mail.",
  publisher: "Example Inc",
  version: "1.2.0",
  remote_url: "https://mcp.example.com/mcp",
  auth: ["oauth"],
  tools: [
    { name: "list_messages", read_only: true, destructive: false, kind: "read", proposed: "on" },
    { name: "send_message", read_only: false, destructive: false, kind: "write", proposed: "ask" },
  ],
  items: [],
};

const draw = (preview: Preview, source: ImportSource) => renderMarkup(createElement(ImportLanding, { source }), { path: "/import", data: [[["preview", source], preview]] });

const textOf = (html: string) => html.replace(/<[^>]+>/g, "");

const barOf = (html: string) => html.slice(html.indexOf("sticky bottom-0"), html.indexOf("</button></div></div></div>", html.indexOf("sticky bottom-0")));

describe("ImportLanding", () => {
  describe("with a skill link", () => {
    const html = draw(SKILL, SKILL_SOURCE);

    test("has one h1, the skill's title", (t) => {
      t.assert.equal(html.split("<h1").length - 1, 1);
      t.assert.match(html, /<h1[^>]*>Find Skills<\/h1>/);
    });

    test("centres its header below lg and aligns it left from lg", (t) => {
      t.assert.match(html, /<header class="flex min-w-0 flex-col items-center text-center lg:items-start lg:text-left">/);
    });

    test("names the link's origin in a caption", (t) => {
      t.assert.match(html, />Install link from versatile\.sh</);
    });

    test("draws the initial of its title on the tile when it has no icon", (t) => {
      t.assert.doesNotMatch(html, /<img/);
      t.assert.match(html, /uppercase[^>]*>F<\/span>/);
    });

    test("draws its icon on the tile through the console's icon proxy", (t) => {
      const icon = "https://versatile.sh/avatars/vercel-labs.png";
      const withIcon = draw({ ...SKILL, icon }, SKILL_SOURCE);
      t.assert.match(withIcon, new RegExp(`<img[^>]*src="/api/icon\\?u=${encodeURIComponent(icon).replace(/\./g, "\\.")}"`));
      t.assert.doesNotMatch(withIcon, /uppercase[^>]*>F<\/span>/);
    });

    test("shows the owner and the licence as chips", (t) => {
      t.assert.match(textOf(html), /Skill/);
      t.assert.match(html, />by vercel-labs</);
      t.assert.match(html, />Licence MIT</);
    });

    test("renders the commit as seven characters", (t) => {
      t.assert.match(html, />57217cf</);
    });

    test("keeps the full hash out of the text", (t) => {
      t.assert.doesNotMatch(textOf(html), new RegExp(COMMIT));
    });

    test("keeps the full hash in the chip's tooltip", (t) => {
      t.assert.match(html, new RegExp(`<button[^>]*title="${COMMIT}"`));
      t.assert.match(html, /aria-label="Copy commit 57217cf"/);
    });

    test("gives one provenance line with the full link in its tooltip", (t) => {
      t.assert.match(html, /<a href="https:\/\/versatile\.sh\/skills\/catalog\/find-skills\.yaml"[^>]*title="https:\/\/versatile\.sh\/skills\/catalog\/find-skills\.yaml"[^>]*>[^]*From the versatile\.sh catalog/);
    });

    describe("from lg", () => {
      test("is two columns from the top, the hero on the left and one card on the right", (t) => {
        t.assert.match(html, /class="flex w-full min-w-0 flex-1 flex-col justify-start gap-10 lg:grid lg:grid-cols-\[minmax\(0,1fr\)_30rem\] lg:content-start lg:items-start"/);
        t.assert.ok(html.indexOf("<header") < html.indexOf('class="contents lg:sticky'));
        t.assert.ok(html.indexOf("<h1") < html.indexOf('class="contents lg:sticky'));
      });

      test("holds the review, who may load it and the notice in that one card", (t) => {
        const card = html.indexOf('class="contents lg:sticky');
        t.assert.ok(card < html.indexOf('<h2 id="review-title"'));
        t.assert.ok(html.indexOf('<h2 id="review-title"') < html.indexOf('<h2 id="summary-title"'));
        t.assert.ok(html.indexOf('<h2 id="summary-title"') < html.indexOf("Nothing is installed until you confirm"));
        t.assert.match(html, /<h2 id="review-title"[^>]*>Review the skill</);
        t.assert.match(html, /<h2 id="summary-title"[^>]*>Who may load it</);
      });

      test("keeps the card sticky, as tall as the screen allows and no taller", (t) => {
        t.assert.match(html, /class="contents lg:sticky lg:top-18 lg:flex lg:max-h-\[calc\(100dvh-9rem\)\] lg:flex-col"/);
        t.assert.doesNotMatch(html, /tall:/);
      });

      test("scrolls the review inside the card and keeps the actions out of that scroll", (t) => {
        t.assert.match(html, /lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:overscroll-contain lg:rounded-b-none lg:border-b-0/);
        t.assert.ok(html.indexOf("lg:overflow-y-auto") < html.indexOf('class="sticky bottom-0'));
        t.assert.ok(html.indexOf('<h2 id="summary-title"') < html.indexOf('class="sticky bottom-0'));
      });

      test("draws the actions once, as the foot of the card", (t) => {
        t.assert.equal(html.split("Add skill").length - 1, 1);
        t.assert.equal(html.split(">Cancel<").length - 1, 1);
        t.assert.match(html, /class="sticky bottom-0[^"]*lg:static[^"]*lg:rounded-b-2xl lg:border lg:bg-ink-raised/);
      });
    });

    test("fixes nothing over the review", (t) => {
      t.assert.doesNotMatch(html, /\bfixed\b/);
    });

    describe("the bar below lg", () => {
      const bar = barOf(html);

      test("is solid, with a hairline on top", (t) => {
        t.assert.match(bar, /border-t border-line bg-ink /);
      });

      test("becomes the card's foot from lg", (t) => {
        t.assert.match(bar, /lg:static/);
        t.assert.doesNotMatch(bar, /lg:hidden/);
      });

      test("holds Cancel and the primary action", (t) => {
        t.assert.match(bar, /Cancel/);
        t.assert.match(bar, /Add skill/);
      });

      test("stays in the flow after the panel, so it reserves its own height", (t) => {
        t.assert.ok(html.indexOf("sticky bottom-0") > html.indexOf("summary-title"));
      });
    });

    test("offers who may load it", (t) => {
      t.assert.match(html, /For your access key/);
    });

    test("stacks the access choice under its note until its container has 28rem", (t) => {
      t.assert.match(html, /<div class="@container"><div class="grid grid-cols-1 items-center gap-3 @md:grid-cols-\[minmax\(0,1fr\)_auto\]/);
    });
  });

  describe("with a connector link", () => {
    const html = draw(CONNECTOR, CONNECTOR_SOURCE);

    test("shows its version where a skill shows its commit", (t) => {
      t.assert.match(html, />1\.2\.0</);
      t.assert.doesNotMatch(html, /Copy commit/);
    });

    test("names the action Add connector", (t) => {
      t.assert.match(html, /Add connector/);
      t.assert.doesNotMatch(html, /Add skill/);
    });

    test("lists every function for review", (t) => {
      t.assert.match(html, /list_messages/);
      t.assert.match(html, /send_message/);
    });

    test("keeps its server and credentials in the review", (t) => {
      t.assert.match(html, /https:\/\/mcp\.example\.com\/mcp/);
      t.assert.match(html, />Credentials</);
    });

    test("summarises the proposed access under the review, in the same card", (t) => {
      t.assert.ok(html.indexOf('<h2 id="review-title"') < html.indexOf('<h2 id="summary-title"'));
      t.assert.match(html, /<h2 id="summary-title"[^>]*>Proposed access</);
      t.assert.match(html, />2 functions</);
    });
  });

  describe("with a connector that lists no function", () => {
    const html = draw({ ...CONNECTOR, tools: [], warning: "the server could not be reached to list its functions" }, CONNECTOR_SOURCE);

    test("draws no proposed access to decide", (t) => {
      t.assert.doesNotMatch(html, /summary-title|Proposed access|0 functions/);
    });

    test("still says why and offers Add connector", (t) => {
      t.assert.match(html, /could not be reached/);
      t.assert.match(html, /Add connector/);
    });
  });

  describe("while the link is read", () => {
    const html = renderMarkup(createElement(ImportLanding, { source: SKILL_SOURCE }), { path: "/import" });

    test("says what is being read", (t) => {
      t.assert.match(html, /Reading https:\/\/versatile\.sh\/skills\/catalog\/find-skills\.yaml/);
    });

    test("offers no action yet", (t) => {
      t.assert.doesNotMatch(html, /sticky bottom-0|Add skill|Add connector/);
    });

    test("still tells that nothing is installed before the owner confirms", (t) => {
      t.assert.match(html, /Nothing is installed until you confirm/);
    });

    test("keeps the card's rounded foot, as it has no actions to join", (t) => {
      t.assert.doesNotMatch(html, /lg:rounded-b-none/);
    });
  });

  describe("when the link cannot be read", () => {
    const html = renderMarkup(createElement(ImportLanding, { source: SKILL_SOURCE }), { path: "/import", failed: [["preview", SKILL_SOURCE]] });

    test("titles the page with the host", (t) => {
      t.assert.match(html, /<h1[^>]*>versatile\.sh<\/h1>/);
    });

    test("offers the way back and no columns", (t) => {
      t.assert.match(html, /Go to the console/);
      t.assert.doesNotMatch(html, /lg:grid-cols|sticky/);
    });
  });
});
