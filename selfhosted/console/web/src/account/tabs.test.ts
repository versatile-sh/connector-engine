import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { viewerOf } from "../people/viewer.ts";
import { ONE_OWNER, TEAM_ADMIN, TEAM_MEMBER } from "../testing/fixtures.ts";
import { accountPathOf, accountTabOf, accountTabsOf } from "./tabs.ts";

describe("accountTabsOf", () => {
  test("gives an Owner every tab", () => {
    assert.deepEqual(accountTabsOf(viewerOf(ONE_OWNER)), ["profile", "people", "backups"]);
  });

  test("gives an Admin Backups for the setup", () => {
    assert.deepEqual(accountTabsOf(viewerOf(TEAM_ADMIN)), ["profile", "people", "backups"]);
  });

  test("gives a Member Profile alone", () => {
    assert.deepEqual(accountTabsOf(viewerOf(TEAM_MEMBER)), ["profile"]);
  });
});

describe("accountTabOf", () => {
  test("reads the tab of its path", () => {
    assert.equal(accountTabOf("/account/people", ["profile", "people"]), "people");
  });

  test("shows Profile for a tab the viewer may not open", () => {
    assert.equal(accountTabOf("/account/backups", ["profile", "people"]), "profile");
  });
});

describe("accountPathOf", () => {
  test("keeps Profile at /account", () => {
    assert.equal(accountPathOf("profile"), "/account");
  });
});
