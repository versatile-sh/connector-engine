import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { PEOPLE, TEAM_ADMIN, TEAM_OWNER } from "../testing/fixtures.ts";
import { viewerOf } from "./viewer.ts";
import { actionValue, isLoopbackLink, mayReset, mayRevoke, nameOf, rowActionsOf, timeLeft } from "./people.ts";

const [alice, carol, bob] = PEOPLE.people;
const owner = viewerOf(TEAM_OWNER);
const admin = viewerOf(TEAM_ADMIN);

describe("rowActionsOf", () => {
  describe("for an Owner", () => {
    test("offers the other roles, reset and remove", () => {
      assert.deepEqual(rowActionsOf(owner, bob).map(actionValue), ["role:owner", "role:admin", "reset", "remove"]);
    });

    test("offers no reset or remove on their own row", () => {
      assert.deepEqual(rowActionsOf(owner, alice).map(actionValue), ["role:admin", "role:member"]);
    });
  });

  describe("for an Admin", () => {
    test("offers reset on a Member", () => {
      assert.deepEqual(rowActionsOf(admin, bob).map(actionValue), ["reset"]);
    });

    test("offers nothing on an Owner", () => {
      assert.deepEqual(rowActionsOf(admin, alice), []);
    });
  });
});

describe("mayReset", () => {
  test("lets an Owner reset an Admin", () => {
    assert.equal(mayReset(owner, carol), true);
  });

  test("refuses an Admin an equal account", () => {
    assert.equal(mayReset(admin, { ...carol, person: "p-dan", you: false }), false);
  });

  test("follows the server's may_reset", () => {
    assert.deepEqual([mayReset(admin, { ...carol, may_reset: true }), mayReset(owner, { ...carol, may_reset: false })], [true, false]);
  });

  test("refuses anyone their own row", () => {
    assert.equal(mayReset(admin, { ...carol, you: true, role: "member" }), false);
  });
});

describe("mayRevoke", () => {
  const reset = { id: "r1", purpose: "reset", role: "member", person: "p-bob", created_by: "p-carol", created_at: "", expires_at: "" } as const;

  test("lets an Admin revoke the reset link they made", () => {
    assert.equal(mayRevoke(admin, reset), true);
  });

  test("refuses an Admin a join link", () => {
    assert.equal(mayRevoke(admin, { ...reset, purpose: "join" }), false);
  });

  test("lets an Owner revoke any link", () => {
    assert.equal(mayRevoke(owner, { ...reset, created_by: "p-x" }), true);
  });
});

describe("isLoopbackLink", () => {
  test("knows localhost", () => {
    assert.equal(isLoopbackLink("http://localhost:7465/invite#t"), true);
  });

  test("knows 127.0.0.1", () => {
    assert.equal(isLoopbackLink("http://127.0.0.1:7465/invite#t"), true);
  });

  test("knows the IPv6 loopback", () => {
    assert.equal(isLoopbackLink("http://[::1]:7465/invite#t"), true);
  });

  test("passes a public address", () => {
    assert.equal(isLoopbackLink("https://gateway.example.com/invite#t"), false);
  });

  test("passes a link it cannot read", () => {
    assert.equal(isLoopbackLink("not a link"), false);
  });
});

describe("timeLeft", () => {
  const now = Date.parse("2026-11-20T00:00:00Z");

  test("counts whole days up", () => {
    assert.deepEqual(timeLeft("2026-11-21T12:00:00Z", now), { unit: "day", count: 2 });
  });

  test("counts hours under a day", () => {
    assert.deepEqual(timeLeft("2026-11-20T05:30:00Z", now), { unit: "hour", count: 6 });
  });

  test("never goes below an hour", () => {
    assert.deepEqual(timeLeft("2026-11-19T00:00:00Z", now), { unit: "hour", count: 1 });
  });
});

describe("nameOf", () => {
  test("names a person", () => {
    assert.equal(nameOf(PEOPLE.people, "p-bob"), "bob");
  });

  test("falls back to the id of someone gone", () => {
    assert.equal(nameOf(PEOPLE.people, "p-gone"), "p-gone");
  });
});
