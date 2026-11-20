import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { ONE_OWNER, TEAM_MEMBER, TEAM_OWNER } from "../testing/fixtures.ts";
import { offersScope, roleBadgeOf, scopedHref, scopeOf, viewerOf } from "./viewer.ts";

describe("viewerOf", () => {
  test("grants what the session lists", () => {
    assert.equal(viewerOf(TEAM_MEMBER).can("keys.own"), true);
  });

  test("grants nothing the session leaves out", () => {
    assert.equal(viewerOf(TEAM_MEMBER).can("keys.any"), false);
  });

  describe("without capabilities", () => {
    test("grants nothing", () => {
      assert.equal(viewerOf({ owner_exists: true, signed_in: true, mcp_url: "" }).can("people.read"), false);
    });

    test("reads as a one-user install", () => {
      assert.equal(viewerOf({ owner_exists: true, signed_in: true, mcp_url: "" }).team, false);
    });
  });
});

describe("offersScope", () => {
  describe("in a one-user install", () => {
    test("offers no switch", () => {
      assert.equal(offersScope(viewerOf(ONE_OWNER), "keys.any"), false);
    });
  });

  describe("in a team", () => {
    test("offers it with the any capability", () => {
      assert.equal(offersScope(viewerOf(TEAM_OWNER), "approvals.any"), true);
    });

    test("offers no switch to a Member", () => {
      assert.equal(offersScope(viewerOf(TEAM_MEMBER), "activity.any"), false);
    });
  });
});

describe("scopeOf", () => {
  test("shows everyone when asked and offered", () => {
    assert.equal(scopeOf(new URLSearchParams("scope=everyone"), true), "everyone");
  });

  test("shows the viewer's own rows when not offered", () => {
    assert.equal(scopeOf(new URLSearchParams("scope=everyone"), false), "mine");
  });

  test("shows the viewer's own rows by default", () => {
    assert.equal(scopeOf(new URLSearchParams(), true), "mine");
  });
});

describe("scopedHref", () => {
  test("adds everyone and keeps the query", () => {
    assert.equal(scopedHref("/activity", new URLSearchParams("range=7d"), "everyone"), "/activity?range=7d&scope=everyone");
  });

  test("leaves mine out", () => {
    assert.equal(scopedHref("/access", new URLSearchParams("scope=everyone"), "mine"), "/access");
  });
});

describe("roleBadgeOf", () => {
  test("shows the role in a team", () => {
    assert.equal(roleBadgeOf(viewerOf(TEAM_MEMBER)), "member");
  });

  test("shows nothing in a one-user install", () => {
    assert.equal(roleBadgeOf(viewerOf(ONE_OWNER)), undefined);
  });
});
