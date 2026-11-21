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

    test("centres its header", (t) => {
      t.assert.match(html, /<header class="flex min-w-0 flex-col items-center text-center">/);
    });

    test("names the link's origin in a caption", (t) => {
      t.assert.match(html, />Install link from versatile\.sh</);
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

    test("holds the review and the decision as two columns from lg", (t) => {
      t.assert.match(html, /class="grid items-start gap-6 lg:grid-cols-\[minmax\(0,1fr\)_26rem\] lg:gap-10"/);
      t.assert.match(html, /<h2 id="review-title"[^>]*>Review the skill</);
      t.assert.match(html, /<h2 id="summary-title"[^>]*>Who may load it</);
    });

    test("keeps the decision card sticky from lg, whatever the screen's height", (t) => {
      t.assert.match(html, / lg:sticky lg:top-6"/);
      t.assert.doesNotMatch(html, /tall:/);
    });

    test("puts the actions in the decision card from lg", (t) => {
      t.assert.match(html, /<div class="hidden lg:block">[^]*Add skill/);
    });

    test("fixes nothing over the review", (t) => {
      t.assert.doesNotMatch(html, /\bfixed\b/);
    });

    describe("the bar below lg", () => {
      const bar = barOf(html);

      test("is solid, with a hairline on top", (t) => {
        t.assert.match(bar, /border-t border-line bg-ink /);
      });

      test("is gone from lg", (t) => {
        t.assert.match(bar, /lg:hidden/);
      });

      test("holds Cancel and the primary action", (t) => {
        t.assert.match(bar, /Cancel/);
        t.assert.match(bar, /Add skill/);
      });

      test("stays in the flow after both columns, so it reserves its own height", (t) => {
        t.assert.ok(html.indexOf("sticky bottom-0") > html.indexOf("lg:grid-cols"));
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

    test("summarises the proposed access in the decision card", (t) => {
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
  });

  describe("when the link cannot be read", () => {
    const html = renderMarkup(createElement(ImportLanding, { source: SKILL_SOURCE }), { path: "/import", failed: [["preview", SKILL_SOURCE]] });

    test("titles the page with the host", (t) => {
      t.assert.match(html, /<h1[^>]*>versatile\.sh<\/h1>/);
    });

    test("offers the way back and no columns", (t) => {
      t.assert.match(html, /Go to the console/);
      t.assert.doesNotMatch(html, /lg:grid-cols|sticky bottom-0/);
    });
  });
});
