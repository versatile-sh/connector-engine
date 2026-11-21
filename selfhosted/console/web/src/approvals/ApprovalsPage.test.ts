import assert from "node:assert/strict";
import { describe, mock, test } from "node:test";
import type { QueryKey } from "@tanstack/react-query";
import { createElement } from "react";
import type { Session } from "../api.ts";
import { CHANNEL_KINDS, CONNECTOR, ONE_OWNER, TEAM_MEMBER, TEAM_OWNER, pageData } from "../testing/fixtures.ts";
import { renderMarkup } from "../testing/render.ts";
import { ApprovalsPage } from "./ApprovalsPage.tsx";

mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") });

type Overrides = readonly (readonly [QueryKey, unknown])[];

const draw = (session: Session, overrides: Overrides = [], path = "/approvals", failed: readonly QueryKey[] = []) => {
  const names = new Set([...overrides.map(([key]) => key), ...failed].map((key) => JSON.stringify(key)));
  const data = [...pageData(session).filter(([key]) => !names.has(JSON.stringify(key))), ...overrides];
  return renderMarkup(createElement(ApprovalsPage), { path, data, failed });
};

const NONE: Overrides = [[["approvals"], { approvals: [] }]];

const count = (html: string, text: string) => html.split(text).length - 1;

const slice = (html: string, from: string, to: string) => html.slice(html.indexOf(from), html.indexOf(to, html.indexOf(from)));

describe("ApprovalsPage", () => {
  describe("with nothing waiting", () => {
    const html = draw(ONE_OWNER, NONE);

    test("keeps one serif headline, the page's own", () => {
      assert.equal(count(html, "<h1"), 1);
      assert.equal(count(html, "font-serif"), 1);
    });

    test("says nothing is waiting in one sans line on a compact centred card", () => {
      assert.match(html, /<h2 class="text-lg[^"]*">Nothing is <span class="text-lime">waiting<\/span><\/h2>/);
      assert.match(html, /class="arrive mx-auto flex w-full max-w-xl flex-col items-center text-center /);
    });

    test("offers no lone button under the empty line", () => {
      assert.doesNotMatch(html, /Review connectors/);
    });

    test("lays the main column and the side column out as two columns on a wide screen", () => {
      assert.match(html, /<div class="grid items-start gap-6 xl:grid-cols-\[minmax\(0,1fr\)_20rem\] xl:gap-10">/);
    });

    test("fills the main column with the empty card then the recent decisions", () => {
      const main = slice(html, '<div class="flex min-w-0 flex-col gap-6">', "<aside");
      assert.ok(main.indexOf("Nothing is") < main.indexOf("Recently decided"));
    });

    test("fills the side column with the functions set to Ask then where approvals reach the viewer", () => {
      const side = html.slice(html.indexOf("<aside"));
      assert.ok(side.indexOf("Functions set to Ask") < side.indexOf("Where approvals reach you"));
    });
  });

  describe("the functions set to Ask", () => {
    test("list each connector with its functions, linking to its page", () => {
      const html = draw(ONE_OWNER, NONE);
      assert.match(html, /<a href="\/connectors\/c1"[^>]*>.*?Gmail.*?send_message/);
    });

    test("say how to set one when none is", () => {
      const quiet = { ...CONNECTOR, functions: CONNECTOR.functions.map((fn) => ({ ...fn, access: "on" as const })) };
      const html = draw(ONE_OWNER, [...NONE, [["connectors"], { connectors: [quiet] }]]);
      assert.match(html, /No function is set to Ask yet/);
      assert.match(html, /Open connectors/);
    });
  });

  describe("where approvals reach the viewer", () => {
    test("always names this console", () => {
      assert.match(draw(ONE_OWNER, NONE), /This console.*?Answer on this page/);
    });

    test("says only the console shows held calls when nothing else is set up, and links to Notifications", () => {
      const html = draw(ONE_OWNER, NONE);
      assert.match(html, /Only this console shows held calls so far/);
      assert.match(html, /<a href="\/notifications"[^>]*>Notification settings/);
    });

    test("lists a paired computer and a chat channel with their status", () => {
      const seen = new Date(Date.now() - 3 * 3_600_000).toISOString();
      const html = draw(ONE_OWNER, [
        ...NONE,
        [["notifications"], { apps: [{ name: "claude-code", elicit: true }], devices: [{ id: "d", name: "laptop", created_at: "2026-11-01T00:00:00Z", last_seen: seen }], channels: [{ id: "ch", kind: "discord", label: "Team chat", interactive: true, created_at: "2026-11-01T00:00:00Z", status: "connected" }], kinds: CHANNEL_KINDS }],
      ]);
      assert.match(html, /laptop.*?Seen 3 hours ago/);
      assert.match(html, /Team chat.*?Discord bot.*?connected/);
      assert.match(html, /Asks in 1 app/);
      assert.doesNotMatch(html, /Only this console shows/);
    });
  });

  describe("the recent decisions", () => {
    const html = draw(ONE_OWNER, NONE);

    test("list how each ended, the function, who answered and when", () => {
      assert.match(html, /approved.*?send_message.*?by alice.*?5 minutes ago/);
      assert.match(html, /denied.*?delete_message/);
      assert.match(html, /expired.*?send_message/);
    });

    test("keep the newest first", () => {
      assert.ok(html.indexOf("5 minutes ago") < html.indexOf("3 hours ago"));
    });

    test("say nothing was decided yet when the list is empty", () => {
      assert.match(draw(ONE_OWNER, [...NONE, [["approvals", "recent", "mine"], { decisions: [] }]]), /Nothing has been decided yet/);
    });
  });

  describe("with calls waiting", () => {
    const html = draw(ONE_OWNER);

    test("makes the waiting list the main column, with no empty card", () => {
      assert.doesNotMatch(html, /Nothing is <span/);
      const main = slice(html, '<div class="flex min-w-0 flex-col gap-6">', "<aside");
      assert.ok(main.indexOf("send_message") < main.indexOf("Recently decided"));
    });

    test("shows each call's function, connector, key, when it was asked and when it runs out", () => {
      assert.match(html, /send_message/);
      assert.match(html, /on Gmail/);
      assert.match(html, /from laptop/);
      assert.match(html, /Asked 1 minute ago|Asked in /);
      assert.match(html, /expires in/);
    });

    test("offers the answers as buttons", () => {
      assert.match(html, />Allow once<\/button>/);
      assert.match(html, />Deny<\/button>/);
    });

    test("keeps the sections beside the list", () => {
      assert.match(html, /<aside[^>]*>.*Functions set to Ask.*Where approvals reach you/);
    });

    test("previews the arguments once they are read", () => {
      const withArgs = draw(ONE_OWNER, [[["approval", "a1", "arguments"], { arguments: { to: "bob@example.com", subject: "Lunch?" } }]]);
      assert.match(withArgs, /to: bob@example\.com, subject: Lunch\?/);
    });

    test("draws a placeholder where the arguments are on their way", () => {
      assert.match(html, /shimmer rounded-full mt-3 h-8 w-full max-w-md/);
    });
  });

  describe("when a read fails", () => {
    test("says so in the waiting list's place and never that nothing is waiting", () => {
      const html = draw(ONE_OWNER, [], "/approvals", [["approvals"]]);
      assert.match(html, /The console is down\./);
      assert.doesNotMatch(html, /Nothing is <span/);
    });

    test("says each side card could not be read, with no placeholder that never ends", () => {
      const html = draw(ONE_OWNER, NONE, "/approvals", [["connectors"], ["notifications"], ["approvals", "recent", "mine"]]);
      assert.equal(count(html, "That could not be read."), 3);
      assert.doesNotMatch(html, /shimmer/);
    });
  });

  describe("for a Member", () => {
    test("offers no scope switch", () => {
      assert.doesNotMatch(draw(TEAM_MEMBER, NONE), /Whose rows/);
    });

    test("reads only their own decisions, even when the address asks for everyone", () => {
      const html = draw(TEAM_MEMBER, [...NONE, [["approvals", "recent", "mine"], { decisions: [] }]], "/approvals?scope=everyone");
      assert.match(html, /Nothing has been decided yet/);
    });
  });

  describe("for an owner of a team", () => {
    test("centres Mine or Everyone in a toolbar over the two columns", () => {
      const html = draw(TEAM_OWNER, NONE);
      assert.match(html, /<\/header><div class="flex min-w-0 flex-wrap items-center justify-center gap-2 sm:flex-nowrap"><div role="group" aria-label="Whose rows"/);
      assert.equal(count(html, "Whose rows"), 1);
    });
  });
});
