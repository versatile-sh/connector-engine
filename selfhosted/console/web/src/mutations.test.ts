import assert from "node:assert/strict";
import { afterEach, beforeEach, describe, test } from "node:test";
import { MutationObserver, QueryClient, type QueryKey, type UseMutationOptions } from "@tanstack/react-query";
import * as mutations from "./mutations.ts";
import { accountQuery, activityQuery, activitySettingsQuery, approvalQuery, approvalsQuery, connectorsQuery, keysQuery, notificationsQuery, sessionQuery, skillsQuery } from "./queries.ts";

const KNOWN: readonly QueryKey[] = [
  sessionQuery.queryKey,
  connectorsQuery.queryKey,
  skillsQuery.queryKey,
  keysQuery.queryKey,
  approvalsQuery.queryKey,
  activityQuery({ range: "24h" }).queryKey,
  activitySettingsQuery.queryKey,
  notificationsQuery.queryKey,
  accountQuery.queryKey,
];

const realFetch = globalThis.fetch;

beforeEach(() => {
  globalThis.fetch = async () => new Response(JSON.stringify({ outcome: { id: "a1", state: "approved" } }), { status: 200 });
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

async function staleAfter<TVariables>(options: UseMutationOptions<unknown, Error, TVariables>, variables: TVariables) {
  const client = new QueryClient();
  for (const key of KNOWN) client.setQueryData(key, {});
  await new MutationObserver(client, options).mutate(variables);
  return { stale: client.getQueryCache().getAll().filter((query) => query.state.isInvalidated).map((query) => query.queryKey[0]), client };
}

describe("mutations", () => {
  const cases: { name: string; run: () => ReturnType<typeof staleAfter>; stale: string[] }[] = [
    { name: "signIn", run: () => staleAfter(mutations.signIn, { mode: "login", username: "o", password: "p" }), stale: ["session"] },
    { name: "mintKey", run: () => staleAfter(mutations.mintKey, { name: "cursor", ttl_hours: 24 }), stale: ["session", "agents"] },
    { name: "revokeKey", run: () => staleAfter(mutations.revokeKey, "cursor"), stale: ["session", "agents"] },
    { name: "setFunctionAccess", run: () => staleAfter(mutations.setFunctionAccess, { connector: "c1", name: "f", access: "on" }), stale: ["connectors"] },
    { name: "setItemAccess", run: () => staleAfter(mutations.setItemAccess, { connector: "c1", name: "prompt:x", access: "on" }), stale: ["connectors"] },
    { name: "sealHeader", run: () => staleAfter(mutations.sealHeader, { connector: "c1", header: "Authorization", value: "v" }), stale: ["connectors"] },
    { name: "connectOAuth", run: () => staleAfter(mutations.connectOAuth, { connector: "c1", clientID: "", clientSecret: "" }), stale: [] },
    { name: "removeConnector", run: () => staleAfter(mutations.removeConnector, "c1"), stale: ["session", "connectors", "skills"] },
    { name: "installImport", run: () => staleAfter(mutations.installImport, { mcp: "https://x", choices: {} }), stale: ["session", "connectors", "skills"] },
    { name: "setSkillAccess", run: () => staleAfter(mutations.setSkillAccess, { skill: "s1", agent: "cursor", access: "off" }), stale: ["skills"] },
    { name: "setSkillAccessAll", run: () => staleAfter(mutations.setSkillAccessAll, { skill: "s1", access: "on" }), stale: ["skills"] },
    { name: "removeSkill", run: () => staleAfter(mutations.removeSkill, "s1"), stale: ["skills", "session"] },
    { name: "saveRecording", run: () => staleAfter(mutations.saveRecording, { record_arguments: true, record_results: false }), stale: [] },
    { name: "deleteActivity", run: () => staleAfter(mutations.deleteActivity, undefined), stale: ["activity", "activity-settings"] },
    { name: "setElicit", run: () => staleAfter(mutations.setElicit, { name: "cursor", elicit: true }), stale: ["notifications"] },
    { name: "addChannel", run: () => staleAfter(mutations.addChannel, { kind: "ntfy", label: "n", settings: {} }), stale: ["notifications"] },
    { name: "testChannel", run: () => staleAfter(mutations.testChannel, "ch1"), stale: [] },
    { name: "removeChannel", run: () => staleAfter(mutations.removeChannel, "ch1"), stale: ["notifications"] },
    { name: "pairDevice", run: () => staleAfter(mutations.pairDevice, "Linux · Firefox"), stale: ["notifications"] },
    { name: "renameDevice", run: () => staleAfter(mutations.renameDevice, { id: "d1", name: "Desk" }), stale: ["notifications"] },
    { name: "revokeDevice", run: () => staleAfter(mutations.revokeDevice, "d1"), stale: ["notifications"] },
    { name: "changePassword", run: () => staleAfter(mutations.changePassword, { current: "old", password: "new password" }), stale: [] },
    { name: "renameOwner", run: () => staleAfter(mutations.renameOwner, "owner"), stale: ["account"] },
    { name: "exportBackup", run: () => staleAfter(mutations.exportBackup, { mode: "setup" }), stale: ["account"] },
    { name: "inspectBackup", run: () => staleAfter(mutations.inspectBackup, { file: new Blob(["x"]), passphrase: "" }), stale: [] },
  ];

  for (const { name, run, stale } of cases) {
    test(`${name} makes ${stale.length > 0 ? stale.join(", ") : "nothing"} stale`, async () => {
      assert.deepEqual((await run()).stale.sort(), [...stale].sort());
    });
  }

  describe("decideApproval", () => {
    describe("when signed in", () => {
      test("refreshes the held list", async () => {
        assert.deepEqual((await staleAfter(mutations.decideApproval, { id: "a1", choice: "once" })).stale, ["approvals"]);
      });

      test("refreshes connectors and skills after Always", async () => {
        assert.deepEqual((await staleAfter(mutations.decideApproval, { id: "a1", choice: "always" })).stale.sort(), ["approvals", "connectors", "skills"]);
      });
    });

    describe("with a link token", () => {
      test("makes nothing stale", async () => {
        assert.deepEqual((await staleAfter(mutations.decideApproval, { id: "a1", choice: "always", token: "t" })).stale, []);
      });
    });

    test("caches the record it answers", async () => {
      const { client } = await staleAfter(mutations.decideApproval, { id: "a1", choice: "once" });
      assert.deepEqual(client.getQueryData(approvalQuery("a1", null).queryKey), { id: "a1", state: "approved" });
    });
  });

  describe("importBackup", () => {
    describe("with a setup", () => {
      test("refreshes connectors, skills and session", async () => {
        globalThis.fetch = async () => new Response(JSON.stringify({ added: ["github"], kept: [], failed: {}, sign_ins: [] }), { status: 200 });
        assert.deepEqual((await staleAfter(mutations.importBackup, { file: new Blob(["x"]), passphrase: "" })).stale.sort(), ["connectors", "session", "skills"]);
      });
    });

    describe("with the owner's password", () => {
      test("sends it in the form", async () => {
        let sent: FormData | undefined;
        globalThis.fetch = async (_input, init) => {
          sent = init?.body instanceof FormData ? init.body : undefined;
          return new Response(JSON.stringify({ kind: "full", counts: {}, replaces: true }), { status: 200 });
        };
        await staleAfter(mutations.importBackup, { file: new Blob(["x"]), passphrase: "p", password: "secret" });
        assert.equal(sent?.get("password"), "secret");
      });
    });

    describe("without a password", () => {
      test("sends no password field", async () => {
        let sent: FormData | undefined;
        globalThis.fetch = async (_input, init) => {
          sent = init?.body instanceof FormData ? init.body : undefined;
          return new Response(JSON.stringify({ kind: "full", counts: {}, replaces: false }), { status: 200 });
        };
        await staleAfter(mutations.importBackup, { file: new Blob(["x"]), passphrase: "p" });
        assert.equal(sent?.has("password"), false);
      });
    });

    describe("with a full backup", () => {
      test("makes nothing stale", async () => {
        globalThis.fetch = async () => new Response(JSON.stringify({ kind: "full", counts: {}, replaces: true }), { status: 200 });
        assert.deepEqual((await staleAfter(mutations.importBackup, { file: new Blob(["x"]), passphrase: "p" })).stale, []);
      });
    });
  });

  describe("signOut", () => {
    test("forgets every cached answer", async () => {
      const { client } = await staleAfter(mutations.signOut, undefined);
      assert.equal(client.getQueryCache().getAll().length, 0);
    });
  });

  describe("dismissGuide", () => {
    test("marks the guide dismissed in the session", async () => {
      const { client } = await staleAfter(mutations.dismissGuide, undefined);
      assert.deepEqual(client.getQueryData(sessionQuery.queryKey), { onboarding_dismissed: true });
    });
  });
});
