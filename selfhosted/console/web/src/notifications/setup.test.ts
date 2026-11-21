import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { CHANNEL_KINDS } from "../testing/fixtures.ts";
import { SLACK_MANIFEST, missingFields, setupsOf, settingsFor, textOf } from "./setup.ts";

const [discord, slack, ntfy, future] = setupsOf(CHANNEL_KINDS);

describe("setupsOf", () => {
  test("lists one setup per kind the server sends, in its order", () => {
    assert.deepEqual(setupsOf(CHANNEL_KINDS).map((s) => s.kind), ["discord", "slack", "ntfy", "future_chat"]);
  });

  test("takes fields, secrets and interactivity from the server", () => {
    assert.deepEqual(slack?.fields.map((f) => [f.name, f.secret]), [["app_token", true], ["bot_token", true], ["owner_user_id", false]]);
    assert.deepEqual([discord?.interactive, ntfy?.interactive], [true, false]);
  });

  describe("for a kind the console has words for", () => {
    test("adds its translated title, steps and labels", () => {
      assert.equal(discord?.titleKey, "notify.kind.discord");
      assert.ok((discord?.steps.length ?? 0) >= 2);
      assert.equal(discord?.fields[0]?.labelKey, "notify.field.botToken");
    });
  });

  describe("for a kind it has no words for", () => {
    test("keeps the server's English and shows no steps", () => {
      assert.deepEqual([future?.titleKey, future?.title, future?.steps.length], [undefined, "Future chat", 0]);
      assert.equal(textOf((key) => `t:${key}`, future?.titleKey, future?.title ?? ""), "Future chat");
      assert.equal(textOf((key) => `t:${key}`, discord?.titleKey, "x"), "t:notify.kind.discord");
    });
  });
});

describe("settingsFor", () => {
  describe("with an empty optional token", () => {
    test("leaves it out", () => {
      assert.ok(ntfy);
      assert.deepEqual(settingsFor(ntfy, { url: " https://ntfy.sh/x ", token: "  " }), { url: "https://ntfy.sh/x" });
    });
  });
});

describe("missingFields", () => {
  test("names the empty required ones", () => {
    assert.ok(slack);
    assert.deepEqual(missingFields(slack, { app_token: "xapp-1" }).map((f) => f.name), ["bot_token", "owner_user_id"]);
  });

  describe("without the optional ntfy token", () => {
    test("asks only for the URL", () => {
      assert.ok(ntfy);
      assert.deepEqual(missingFields(ntfy, {}).map((f) => f.name), ["url"]);
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
