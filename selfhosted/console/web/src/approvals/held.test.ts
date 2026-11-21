import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { viewerOf } from "../people/viewer.ts";
import { APPROVAL, TEAM_MEMBER, TEAM_OWNER } from "../testing/fixtures.ts";
import { heldViewOf } from "./held.ts";

const BOBS = { ...APPROVAL, owner: { person: "p-bob", username: "bob" } };
const owner = viewerOf(TEAM_OWNER);

describe("heldViewOf", () => {
  describe("for the viewer's own call", () => {
    test("names nobody to wait for", () => {
      assert.equal(heldViewOf(APPROVAL, owner, "shared").waitingFor, undefined);
    });

    test("offers the server's choices", () => {
      assert.deepEqual(heldViewOf(APPROVAL, owner, "shared").decide, ["once", "always", "deny"]);
    });
  });

  describe("for someone else's call on a shared connector", () => {
    const view = heldViewOf({ ...BOBS, choices: ["once", "session", "deny", "cancel"] }, owner, "shared");

    test("waits for its owner", () => {
      assert.equal(view.waitingFor, "bob");
    });

    test("offers the answers and Cancel", () => {
      assert.deepEqual([view.decide, view.cancel], [["once", "session", "deny"], true]);
    });
  });

  describe("for someone else's call on their own sign-in", () => {
    const view = heldViewOf({ ...BOBS, choices: ["cancel"] }, owner, "per_person");

    test("offers only Cancel", () => {
      assert.deepEqual([view.decide, view.cancel], [[], true]);
    });

    test("names who decides", () => {
      assert.equal(view.decidesAlone, "bob");
    });
  });

  describe("without choices from the server", () => {
    test("keeps today's answers on the viewer's own call", () => {
      assert.deepEqual(heldViewOf({ ...APPROVAL, choices: undefined }, owner, undefined).decide, ["once", "always", "deny"]);
    });

    test("offers nothing on someone else's call", () => {
      assert.deepEqual(heldViewOf({ ...APPROVAL, choices: undefined }, viewerOf(TEAM_MEMBER), undefined), { waitingFor: "alice", decide: [], cancel: false, decidesAlone: undefined, open: true });
    });
  });

  test("reads the owner from on_behalf_of without one", () => {
    assert.equal(heldViewOf({ ...APPROVAL, owner: undefined, on_behalf_of: "p-bob" }, owner, "shared").waitingFor, "p-bob");
  });

  test("names a removed owner by their id", () => {
    assert.equal(heldViewOf({ ...APPROVAL, owner: { person: "p-gone", username: "" } }, owner, "shared").waitingFor, "p-gone");
  });
});
