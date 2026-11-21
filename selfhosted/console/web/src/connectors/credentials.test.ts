import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { viewerOf } from "../people/viewer.ts";
import { CONNECTOR, ONE_OWNER, TEAM_ADMIN, TEAM_MEMBER } from "../testing/fixtures.ts";
import { credentialsViewOf } from "./credentials.ts";

const PER_PERSON = { ...CONNECTOR, credential_mode: "per_person", signed_in: false, signed_in_count: 3, people_count: 5 } as const;

describe("credentialsViewOf", () => {
  describe("in a one-user install", () => {
    const view = credentialsViewOf(CONNECTOR, viewerOf(ONE_OWNER));

    test("offers no choice of mode", () => {
      assert.equal(view.modes, undefined);
    });

    test("keeps the shared forms", () => {
      assert.equal(view.forms, true);
    });
  });

  describe("for an Admin of a team", () => {
    test("offers who signs in", () => {
      assert.deepEqual(credentialsViewOf(CONNECTOR, viewerOf(TEAM_ADMIN)).modes, ["shared", "per_person"]);
    });

    test("counts the people signed in", () => {
      assert.deepEqual(credentialsViewOf(PER_PERSON, viewerOf(TEAM_ADMIN)).count, { signed: 3, of: 5 });
    });
  });

  describe("for a Member", () => {
    describe("on one account for everyone", () => {
      const view = credentialsViewOf(CONNECTOR, viewerOf(TEAM_MEMBER));

      test("shows the credential read-only", () => {
        assert.deepEqual([view.forms, view.modes], [false, undefined]);
      });

      test("has no sign-in of their own", () => {
        assert.equal(view.signedIn, undefined);
      });
    });

    describe("where each person signs in", () => {
      const view = credentialsViewOf(PER_PERSON, viewerOf(TEAM_MEMBER));

      test("lets them sign in", () => {
        assert.equal(view.forms, true);
      });

      test("shows their own state", () => {
        assert.deepEqual([view.signedIn, view.needed], [false, true]);
      });

      test("shows no count", () => {
        assert.equal(view.count, undefined);
      });
    });

    test("reads their sign-in once done", () => {
      assert.equal(credentialsViewOf({ ...PER_PERSON, signed_in: true }, viewerOf(TEAM_MEMBER)).needed, false);
    });
  });
});
