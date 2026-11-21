import { describe, test } from "node:test";
import { createElement } from "react";
import type { Skill } from "../api";
import { renderMarkup } from "../testing/render.ts";
import { SkillCard } from "./SkillCard.tsx";
import { SkillOverview } from "./SkillOverview.tsx";

const ICON = "https://versatile.sh/avatars/example.png";
const PROXIED = `/api/icon?u=${encodeURIComponent(ICON)}`;

const skill = (icon?: string): Skill => ({
  id: "s1",
  alias: "pdf",
  title: "PDF forms",
  description: "Fills and reads PDF forms.",
  publisher: "example",
  version: "0123456789abcdef0123456789abcdef01234567",
  state: "active",
  name: "pdf",
  source: { repository: "https://github.com/example/skills", path: "skills/pdf", commit: "0123456789abcdef0123456789abcdef01234567" },
  licence: { spdx: "MIT", redistributable: true },
  redistribution: "bundled",
  has_scripts: false,
  files: [{ path: "SKILL.md", bytes: 226 }],
  keys: ["claude-code"],
  needs: { tools: [], runtimes: [], network: false, mcp: [] },
  compat: [],
  warnings: [],
  access: [{ agent: "claude-code", access: "on" }],
  ...(icon ? { icon } : {}),
});

const card = (value: Skill) => renderMarkup(createElement(SkillCard, { skill: value, href: "/skills?open=s1", open: false, onOpen() {}, onClose() {} }), { path: "/skills" });

describe("SkillCard", () => {
  describe("with an icon", () => {
    const html = card(skill(ICON));

    test("draws it through the console's icon proxy, never from its own host", (t) => {
      t.assert.match(html, new RegExp(`<img[^>]*src="${PROXIED.replace(/[?.]/g, "\\$&")}"`));
      t.assert.doesNotMatch(html, /src="https:\/\/versatile\.sh/);
    });

    test("keeps the title beside the tile", (t) => {
      t.assert.match(html, />PDF forms<\/h2>/);
    });
  });

  describe("without an icon", () => {
    const html = card(skill());

    test("falls back to the initial of its title", (t) => {
      t.assert.doesNotMatch(html, /<img/);
      t.assert.match(html, /uppercase[^>]*>P<\/span>/);
    });
  });
});

describe("SkillOverview", () => {
  const overview = (value: Skill, compact: boolean) => renderMarkup(createElement(SkillOverview, { skill: value, compact }), { path: "/skills" });

  test("in the dialog, shows the icon beside the alias at its commit", (t) => {
    t.assert.match(overview(skill(ICON), false), /<img[^>]*src="\/api\/icon\?u=/);
  });

  test("on the card, leaves the icon to the card's own header", (t) => {
    t.assert.doesNotMatch(overview(skill(ICON), true), /<img/);
  });
});
