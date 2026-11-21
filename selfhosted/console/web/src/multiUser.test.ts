import assert from "node:assert/strict";
import { describe, mock, test } from "node:test";
import { createElement } from "react";
import type { Session } from "./api.ts";
import { App } from "./App.tsx";
import { LinkMade, LinkSheet } from "./people/LinkSheet.tsx";
import { APPROVAL, CONNECTOR, TEAM_ADMIN, TEAM_MEMBER, TEAM_OWNER, pageData } from "./testing/fixtures.ts";
import { renderMarkup, type Seed } from "./testing/render.ts";

const page = (session: Session, path: string, extra: Seed["data"] = []) => renderMarkup(createElement(App), { path, data: [...pageData(session), ...extra] });

const count = (html: string, text: string) => html.split(text).length - 1;

const SIGNED_OUT = { owner_exists: true, signed_in: false, mcp_url: "http://127.0.0.1:7465/mcp" } as const satisfies Session;

const JOIN = { purpose: "join", role: "member", invited_by: "alice", username: "", expires_at: "2026-11-22T00:00:00Z" } as const;

mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-20T12:00:00Z") });

describe("the Account tabs", () => {
  describe("for an Owner", () => {
    const html = page(TEAM_OWNER, "/account/people");

    test("draw a tablist of three tabs", () => {
      assert.deepEqual([count(html, 'role="tablist"'), count(html, 'role="tab"')], [1, 3]);
    });

    test("choose the tab of the path", () => {
      assert.match(html, /aria-selected="true"[^>]*>People</);
    });
  });

  describe("for a Member", () => {
    test("draw no tab bar", () => {
      assert.equal(count(page(TEAM_MEMBER, "/account"), 'role="tablist"'), 0);
    });
  });
});

describe("the People tab", () => {
  describe("for an Owner", () => {
    const html = page(TEAM_OWNER, "/account/people");

    test("lists every person", () => {
      assert.deepEqual(["alice", "carol", "bob"].map((name) => html.includes(`>${name}</span>`)), [true, true, true]);
    });

    test("marks the viewer's own row", () => {
      assert.equal(count(html, ">you</span>"), 1);
    });

    test("offers Invite someone", () => {
      assert.ok(html.includes("Invite someone"));
    });

    test("gives each row one overflow menu", () => {
      assert.equal(count(html, 'role="combobox" aria-label="What to do with'), 3);
    });

    test("lists the waiting link", () => {
      assert.ok(html.includes("Join as Member") && html.includes("expires in 2 days"));
    });

    test("keeps recent changes behind a disclosure", () => {
      assert.match(html, /aria-expanded="false"[^>]*>.*Recent changes/);
    });
  });

  describe("for an Admin", () => {
    const html = page(TEAM_ADMIN, "/account/people");

    test("offers no invite", () => {
      assert.equal(html.includes("Invite someone"), false);
    });

    test("offers Reset password on the Member alone", () => {
      assert.deepEqual([count(html, ">Reset password</button>"), count(html, 'role="combobox" aria-label="What to do with')], [1, 0]);
    });
  });
});

describe("the invite sheet", () => {
  const sheet = renderMarkup(createElement(LinkSheet, { what: { purpose: "join" }, onClose() {} }));

  test("offers one card per role", () => {
    assert.equal(count(sheet, "aria-pressed="), 3);
  });

  test("starts on Member", () => {
    assert.match(sheet, /aria-pressed="true"[^>]*><span[^>]*>Member</);
  });

  describe("once the link is made", () => {
    const made = (link: string) => renderMarkup(createElement(LinkMade, { link: { id: "i2", link, expires_at: "2026-11-23T12:00:00Z" } }));

    test("shows the link to copy", () => {
      assert.ok(made("https://gw.example.com/invite#tok").includes("https://gw.example.com/invite#tok"));
    });

    test("says to send it privately", () => {
      assert.ok(made("https://gw.example.com/invite#tok").includes("Send it privately."));
    });

    test("warns about a loopback link", () => {
      assert.ok(made("http://127.0.0.1:7465/invite#tok").includes("CONSOLE_PUBLIC_URL"));
    });

    test("stays quiet for a public link", () => {
      assert.equal(made("https://gw.example.com/invite#tok").includes("CONSOLE_PUBLIC_URL"), false);
    });
  });
});

describe("the /invite page", () => {
  describe("with a join link", () => {
    const html = page(SIGNED_OUT, "/invite#tok", [[["link", "tok"], JOIN]]);

    test("asks to join", () => {
      assert.ok(html.includes("Join this") && html.includes("alice invited you as Member."));
    });

    test("asks for a name and the password twice", () => {
      assert.deepEqual([count(html, 'autoComplete="username"') + count(html, 'autocomplete="username"'), count(html, 'type="password"')], [1, 2]);
    });
  });

  describe("with a reset link", () => {
    const html = page(SIGNED_OUT, "/invite#tok", [[["link", "tok"], { ...JOIN, purpose: "reset", username: "bob" }]]);

    test("asks for a new password", () => {
      assert.ok(html.includes("Set a new") && html.includes("alice made this link for bob to set a new password."));
    });

    test("asks for no name", () => {
      assert.equal(count(html, 'autocomplete="username"'), 0);
    });
  });

  describe("with a dead link", () => {
    test("says it no longer works", () => {
      assert.ok(page(SIGNED_OUT, "/invite").includes("This link no longer"));
    });
  });
});

describe("Mine and Everyone", () => {
  test("show for an Owner of a team", () => {
    assert.equal(count(page(TEAM_OWNER, "/approvals"), 'aria-label="Whose rows"'), 1);
  });

  test("stay hidden from a Member", () => {
    assert.equal(count(page(TEAM_MEMBER, "/approvals"), 'aria-label="Whose rows"'), 0);
  });

  test("show on Access and Activity too", () => {
    assert.deepEqual([page(TEAM_ADMIN, "/access"), page(TEAM_ADMIN, "/activity")].map((html) => count(html, 'aria-label="Whose rows"')), [1, 1]);
  });

  describe("with Everyone chosen", () => {
    const bobs = { ...APPROVAL, id: "a2", owner: { person: "p-bob", username: "bob" }, choices: ["cancel"] };
    const html = page(TEAM_OWNER, "/approvals?scope=everyone", [
      [["connectors"], { connectors: [{ ...CONNECTOR, credential_mode: "per_person" }] }],
      [["approvals", "everyone"], { approvals: [APPROVAL, bobs] }],
    ]);

    test("counts everyone", () => {
      assert.ok(html.includes("everyone · 2"));
    });

    test("waits for bob", () => {
      assert.ok(html.includes("waiting for bob"));
    });

    test("offers only Cancel on bob's own sign-in", () => {
      assert.ok(html.includes("Cancel this call") && html.includes("bob decides: it uses the own sign-in of bob."));
    });

    test("names the owner on the viewer's row", () => {
      assert.ok(html.includes(">alice</span>"));
    });
  });
});

describe("the role badge", () => {
  test("shows a Member their role", () => {
    assert.match(page(TEAM_MEMBER, "/"), /title="Your role"[^>]*>Member</);
  });

  test("stays hidden in a one-user install", () => {
    assert.equal(count(page({ ...TEAM_OWNER, people: 1 }, "/"), 'title="Your role"'), 0);
  });
});

describe("a connector", () => {
  describe("for a Member", () => {
    const html = page(TEAM_MEMBER, "/connectors/c1");

    test("says admins set its access", () => {
      assert.ok(html.includes("Set by your admins. Each of your keys can only narrow it."));
    });

    test("locks every function's access", () => {
      const groups = html.match(/role="group" aria-label="Access of [^"]*"[^>]*>(?:(?!<\/div>).)*/g) ?? [];
      assert.deepEqual([groups.length > 0, groups.every((group) => count(group, "<button") === count(group, 'disabled=""'))], [true, true]);
    });

    test("leaves the access open to an Owner", () => {
      const groups = page(TEAM_OWNER, "/connectors/c1").match(/role="group" aria-label="Access of [^"]*"[^>]*>(?:(?!<\/div>).)*/g) ?? [];
      assert.deepEqual([groups.length > 0, groups.some((group) => group.includes('disabled=""'))], [true, false]);
    });

    test("offers no removal", () => {
      assert.equal(html.includes(">Remove</button>"), false);
    });

    test("shows the shared credential read-only", () => {
      assert.ok(html.includes(">Set by your admins</p>"));
    });

    describe("where each person signs in", () => {
      const own = page(TEAM_MEMBER, "/connectors/c1", [[["connectors"], { connectors: [{ ...CONNECTOR, credential_mode: "per_person", signed_in: false }] }]]);

      test("shows their own sign-in state", () => {
        assert.ok(own.includes("You are not signed in") && own.includes("Sign in to Gmail"));
      });
    });
  });

  describe("for an Admin of a team", () => {
    const html = page(TEAM_ADMIN, "/connectors/c1", [[["connectors"], { connectors: [{ ...CONNECTOR, credential_mode: "per_person", signed_in: true, signed_in_count: 3, people_count: 5 }] }]]);

    test("offers who signs in", () => {
      assert.ok(html.includes("Who signs in") && html.includes("Only the key&#x27;s owner approves calls."));
    });

    test("counts the people signed in", () => {
      assert.ok(html.includes("3 of 5 people signed in"));
    });
  });
});


describe("the page of someone else's held call", () => {
  const view = {
    id: "a2",
    state: "pending",
    mode: "ask",
    connector: { id: "c1", alias: "gmail", title: "Gmail" },
    function: "send_message",
    kind: "write",
    app: "laptop",
    session: "s-0123456789",
    created_at: "2026-11-20T11:59:00Z",
    expires_at: "2026-11-20T12:05:00Z",
    may_have_run: false,
    reaches_outside: false,
    choices: ["cancel"],
    owner: { person: "p-bob", username: "bob" },
  } as const;
  const html = page(TEAM_OWNER, "/approve/a2", [
    [["connectors"], { connectors: [{ ...CONNECTOR, credential_mode: "per_person" }] }],
    [["approval", "a2"], view],
  ]);

  test("waits for bob", () => {
    assert.ok(html.includes("waiting for bob"));
  });

  test("offers only Cancel", () => {
    assert.deepEqual([html.includes("Cancel this call"), html.includes("Allow once")], [true, false]);
  });
});

describe("the Backups tab of an Admin", () => {
  const html = page(TEAM_ADMIN, "/account/backups");

  test("opens for the setup", () => {
    assert.match(html, /aria-selected="true"[^>]*>Backups</);
  });

  test("says full backups stay with Owners", () => {
    assert.ok(html.includes("full backups stay with Owners"));
  });
});
