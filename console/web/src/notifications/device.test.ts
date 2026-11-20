import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { browserFacts, deviceName } from "./device.ts";

const CHROME_LINUX = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";
const EDGE_WINDOWS = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0";
const SAFARI_MAC = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15";
const FIREFOX_LINUX = "Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0";
const OPERA_ANDROID = "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36 OPR/86.0.0.0";

describe("deviceName", () => {
  describe("with client hints", () => {
    test("names the platform and the real brand", () => {
      const hints = { platform: "macOS", brands: [{ brand: "Not)A;Brand" }, { brand: "Chromium" }, { brand: "Google Chrome" }] };
      assert.equal(deviceName({ userAgent: CHROME_LINUX, hints }), "macOS · Chrome");
    });

    test("keeps Chromium when it is the only brand", () => {
      assert.equal(deviceName({ userAgent: "", hints: { platform: "Chrome OS", brands: [{ brand: "Not_A Brand" }, { brand: "Chromium" }] } }), "ChromeOS · Chromium");
    });

    describe("without a known platform", () => {
      test("reads the system from the user agent", () => {
        assert.equal(deviceName({ userAgent: EDGE_WINDOWS, hints: { platform: "", brands: [{ brand: "Microsoft Edge" }] } }), "Windows · Edge");
      });
    });
  });

  describe("with the user agent alone", () => {
    for (const [userAgent, name] of [
      [CHROME_LINUX, "Linux · Chrome"],
      [EDGE_WINDOWS, "Windows · Edge"],
      [SAFARI_MAC, "macOS · Safari"],
      [FIREFOX_LINUX, "Linux · Firefox"],
      [OPERA_ANDROID, "Android · Opera"],
    ] as const) {
      test(`names ${name}`, () => {
        assert.equal(deviceName({ userAgent }), name);
      });
    }
  });

  describe("when nothing is known", () => {
    test("is empty, for the console to name", () => {
      assert.equal(deviceName({ userAgent: "curl/8.0" }), "");
    });
  });

  test("stays within 60 characters", () => {
    assert.equal(deviceName({ userAgent: "", hints: { platform: "p".repeat(50), brands: [{ brand: "b".repeat(50) }] } }).length, 60);
  });
});

describe("browserFacts", () => {
  test("takes the client hints a navigator has", () => {
    const hints = { platform: "Linux", brands: [{ brand: "Brave" }] };
    const navigator = { userAgent: CHROME_LINUX, userAgentData: hints };
    assert.deepEqual(browserFacts(navigator), { userAgent: CHROME_LINUX, hints });
  });

  describe("with hints of another shape", () => {
    test("keeps the user agent only", () => {
      const navigator = { userAgent: FIREFOX_LINUX, userAgentData: { platform: 1 } };
      assert.deepEqual(browserFacts(navigator), { userAgent: FIREFOX_LINUX });
    });
  });
});
