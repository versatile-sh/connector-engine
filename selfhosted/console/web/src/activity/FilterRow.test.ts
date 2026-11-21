import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { createElement } from "react";
import type { ActivityView } from "./activity.ts";
import { FilterRow } from "./FilterRow.tsx";
import type { ActivityFilter, Named } from "./activity.ts";
import { renderMarkup } from "../testing/render.ts";

const DATA: ActivityView = {
  from: "2026-11-19T00:00:00Z",
  to: "2026-11-20T00:00:00Z",
  total: 3,
  hours: [],
  connectors: [
    { connector: "c1", alias: "gmail", total: 2, failed: 0 },
    { connector: "c2", alias: "pubmed", total: 1, failed: 0 },
  ],
  agents: ["laptop", "desktop"],
  calls: [],
  more: false,
  pages: 1,
};

const NAMED: ReadonlyMap<string, Named> = new Map([
  ["c1", { title: "Gmail" }],
  ["c2", { title: "PubMed" }],
]);

const row = (filter: ActivityFilter, scoped = false) => renderMarkup(createElement(FilterRow, { filter, data: DATA, named: NAMED, scoped, keptDays: 30, onChange() {} }));

const labels = (html: string) => [...html.matchAll(/role="combobox" aria-label="([^"]+)"/g)].map((match) => match[1]);

const attr = (html: string, tag: string, name: string) => new RegExp(`<${tag}[^>]*aria-label="${name}"[^>]*class="([^"]*)"|<${tag}[^>]*class="([^"]*)"[^>]*aria-label="${name}"`).exec(html)?.slice(1).find(Boolean);

const clearButton = (html: string) => /<button[^>]*>Clear filters<\/button>/.exec(html)?.[0] ?? "";

describe("FilterRow", () => {
  const open = row({ range: "24h" });
  const chosen = row({ range: "24h", connector: "c2" });

  describe("with and without a connector chosen", () => {
    test("draws the same controls in the same order", () => {
      assert.deepEqual(labels(chosen), labels(open));
      assert.deepEqual(labels(open), ["Connector", "Access key", "Outcome"]);
    });

    test("gives the connector control one box", () => {
      assert.ok(attr(open, "button", "Connector"));
      assert.equal(attr(chosen, "button", "Connector"), attr(open, "button", "Connector"));
    });

    test("names the chosen connector in that same control", () => {
      assert.match(chosen, /<span class="min-w-0 flex-1 truncate ">PubMed<\/span>/);
      assert.match(open, /<span class="min-w-0 flex-1 truncate ">All connectors<\/span>/);
    });

    test("keeps the toolbar's classes", () => {
      const toolbar = (html: string) => /<div role="group" aria-label="Filters" class="([^"]*)"/.exec(html)?.[1];
      assert.equal(toolbar(chosen), toolbar(open));
    });
  });

  describe("Clear filters", () => {
    test("stays drawn, invisible and inert while nothing narrows the page", () => {
      const clear = clearButton(open);
      assert.match(clear, /aria-hidden="true"/);
      assert.match(clear, /tabindex="-1"/);
      assert.match(clear, /inert=""/);
      assert.match(clear, /\binvisible\b/);
    });

    test("shows and takes focus once a connector narrows the page", () => {
      const clear = clearButton(chosen);
      assert.doesNotMatch(clear, /aria-hidden|inert|invisible|tabindex/);
    });

    test("holds one place whether shown or not", () => {
      assert.equal(chosen.split("Clear filters").length, open.split("Clear filters").length);
    });
  });

  describe("on a wide container", () => {
    test("shares one row that does not wrap when only the range and narrowing controls show", () => {
      assert.match(open, /@5xl:flex-row/);
      assert.match(open, /@5xl:flex-nowrap/);
    });

    test("makes the search the one flexible control", () => {
      assert.match(open, /<form role="search" class="min-w-0 flex-1">/);
    });

    test("keeps two rows once custom days or Everyone add controls", () => {
      assert.doesNotMatch(row({ range: "custom", from: "2026-11-14", to: "2026-11-20" }), /@5xl:flex-row/);
      assert.doesNotMatch(row({ range: "24h", scope: "everyone" }, true), /@5xl:flex-row/);
    });
  });

  describe("with Everyone", () => {
    test("adds the person control and leads with Mine or Everyone", () => {
      const everyone = row({ range: "24h", scope: "everyone" }, true);
      assert.deepEqual(labels(everyone), ["Connector", "Access key", "Person", "Outcome"]);
      assert.ok(everyone.indexOf('aria-label="Whose rows"') < everyone.indexOf('aria-label="Range"'));
    });
  });
});
