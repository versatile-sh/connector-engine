import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { SETUP, SETUPS, SLACK_MANIFEST, missingFields, settingsFor } from "./setup.ts";

const settingsNames = { discord: ["owner_user_id", "token"], slack: ["app_token", "bot_token", "owner_user_id"], telegram: ["owner_user_id", "token"], ntfy: ["token", "url"], discord_webhook: ["url"], slack_webhook: ["url"] };

describe("SETUPS", () => {
  test("covers every kind once", () => {
    assert.deepEqual(SETUPS.map((s) => s.kind).sort(), Object.keys(settingsNames).sort());
  });

  test("asks for the settings the API takes", () => {
    for (const setup of SETUPS) assert.deepEqual(setup.fields.map((f) => f.name).sort(), settingsNames[setup.kind], setup.kind);
  });

  test("gives every kind steps", () => {
    for (const setup of SETUPS) assert.ok(setup.steps.length >= 2, setup.kind);
  });

  describe("when a kind is notify-only", () => {
    test("asks for no owner id", () => {
      for (const setup of SETUPS.filter((s) => !s.interactive)) assert.ok(!setup.fields.some((f) => f.name === "owner_user_id"), setup.kind);
    });
  });

  test("marks tokens and URLs as secrets", () => {
    for (const setup of SETUPS) for (const field of setup.fields) assert.equal(field.secret, field.name !== "owner_user_id", `${setup.kind} ${field.name}`);
  });
});

describe("settingsFor", () => {
  describe("with an empty optional token", () => {
    test("leaves it out", () => {
      assert.deepEqual(settingsFor(SETUP.ntfy, { url: " https://ntfy.sh/x ", token: "  " }), { url: "https://ntfy.sh/x" });
    });
  });
});

describe("missingFields", () => {
  test("names the empty required ones", () => {
    assert.deepEqual(missingFields(SETUP.slack, { app_token: "xapp-1" }).map((f) => f.name), ["bot_token", "owner_user_id"]);
  });

  describe("without the optional ntfy token", () => {
    test("asks only for the URL", () => {
      assert.deepEqual(missingFields(SETUP.ntfy, {}).map((f) => f.name), ["url"]);
    });
  });
});

describe("SLACK_MANIFEST", () => {
  test("turns on Socket Mode and interactivity", () => {
    const manifest = JSON.parse(SLACK_MANIFEST);
    assert.equal(manifest.settings.socket_mode_enabled, true);
    assert.equal(manifest.settings.interactivity.is_enabled, true);
    assert.deepEqual(manifest.oauth_config.scopes.bot, ["chat:write", "im:write"]);
  });
});
