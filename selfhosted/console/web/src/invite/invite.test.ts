import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { linkViewOf, tokenOf } from "./invite.ts";

const JOIN = { purpose: "join", role: "member", invited_by: "alice", username: "", expires_at: "2026-11-22T00:00:00Z" } as const;

describe("tokenOf", () => {
  test("reads the fragment", () => {
    assert.equal(tokenOf("#abc_-1"), "abc_-1");
  });

  test("reads nothing from an empty fragment", () => {
    assert.equal(tokenOf("#"), undefined);
  });
});

describe("linkViewOf", () => {
  test("shows a join form for an invite", () => {
    assert.deepEqual(linkViewOf("t", JOIN, null), { kind: "join", facts: JOIN });
  });

  test("shows a password form for a reset", () => {
    assert.equal(linkViewOf("t", { ...JOIN, purpose: "reset", username: "bob" }, null).kind, "reset");
  });

  test("waits while the link is read", () => {
    assert.equal(linkViewOf("t", undefined, null).kind, "reading");
  });

  describe("with a dead link", () => {
    test("shows it gone on link_gone", () => {
      assert.equal(linkViewOf("t", undefined, { status: 410, message: "gone", code: "link_gone" }).kind, "gone");
    });

    test("shows it gone without a token", () => {
      assert.equal(linkViewOf(undefined, undefined, null).kind, "gone");
    });
  });

  test("shows another refusal as a failure", () => {
    assert.equal(linkViewOf("t", undefined, { status: 429, message: "slow down", code: "slow_down" }).kind, "failed");
  });
});
