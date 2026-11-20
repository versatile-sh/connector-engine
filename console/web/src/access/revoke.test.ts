import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { revokeStep, type RevokeState } from "./revoke.ts";

const idle: RevokeState = { kind: "idle" };
const confirming: RevokeState = { kind: "confirming", name: "cursor" };
const revoking: RevokeState = { kind: "revoking", name: "cursor" };

describe("revokeStep", () => {
  describe("when nothing is asked", () => {
    test("asks about the key whose Revoke was pressed", () => {
      assert.deepEqual(revokeStep(idle, { type: "ask", name: "cursor" }), confirming);
    });

    test("revokes nothing on a stray confirmation", () => {
      assert.deepEqual(revokeStep(idle, { type: "confirm" }), idle);
    });
  });

  describe("while the question is shown", () => {
    test("revokes the key on confirmation", () => {
      assert.deepEqual(revokeStep(confirming, { type: "confirm" }), revoking);
    });

    test("goes back to the key's Revoke button on cancel", () => {
      assert.deepEqual(revokeStep(confirming, { type: "cancel" }), { kind: "idle", returnTo: "cursor" });
    });

    test("moves the question to another key", () => {
      assert.deepEqual(revokeStep(confirming, { type: "ask", name: "codex" }), { kind: "confirming", name: "codex" });
    });
  });

  describe("while a key is being revoked", () => {
    test("ignores cancel and other keys", () => {
      assert.deepEqual(revokeStep(revoking, { type: "cancel" }), revoking);
      assert.deepEqual(revokeStep(revoking, { type: "ask", name: "codex" }), revoking);
    });

    test("closes once the server revoked it", () => {
      assert.deepEqual(revokeStep(revoking, { type: "revoked" }), idle);
    });

    test("asks again when the server failed, so it can be retried", () => {
      assert.deepEqual(revokeStep(revoking, { type: "failed" }), confirming);
    });
  });
});
