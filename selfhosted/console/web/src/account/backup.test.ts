import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { backupKindOf, daysSince, needsExport, passwordRefusal, restarted, secretProblem, NUDGE_DAYS, RESTART_FALLBACK_MS } from "./backup.ts";

const now = new Date("2026-11-20T12:00:00Z");

describe("daysSince", () => {
  test("counts whole days", () => {
    assert.equal(daysSince("2026-11-17T13:00:00Z", now), 2);
  });

  describe("when the time is later today", () => {
    test("gives zero", () => {
      assert.equal(daysSince("2026-11-20T11:59:00Z", now), 0);
    });
  });

  describe("without a time", () => {
    test("gives nothing", () => {
      assert.equal(daysSince(undefined, now), undefined);
    });
  });

  describe("with the zero time", () => {
    test("gives nothing", () => {
      assert.equal(daysSince("0001-01-01T00:00:00Z", now), undefined);
    });
  });

  describe("with an unreadable time", () => {
    test("gives nothing", () => {
      assert.equal(daysSince("soon", now), undefined);
    });
  });
});

describe("needsExport", () => {
  describe("when never exported", () => {
    test("nudges", () => {
      assert.equal(needsExport(undefined), true);
    });
  });

  describe("at the limit", () => {
    test("does not nudge", () => {
      assert.equal(needsExport(NUDGE_DAYS), false);
    });
  });

  describe("past the limit", () => {
    test("nudges", () => {
      assert.equal(needsExport(NUDGE_DAYS + 1), true);
    });
  });
});

describe("backupKindOf", () => {
  test("reads a .cebak as full", () => {
    assert.equal(backupKindOf("connector-engine-backup-1.CEBAK"), "full");
  });

  test("reads a .setup.yaml as a setup", () => {
    assert.equal(backupKindOf("connector-engine-setup-1.setup.yaml"), "setup");
  });

  test("reads a .yml as a setup", () => {
    assert.equal(backupKindOf("team.yml"), "setup");
  });

  describe("with another extension", () => {
    test("gives nothing", () => {
      assert.equal(backupKindOf("notes.txt"), undefined);
    });
  });
});

describe("secretProblem", () => {
  describe("when too short", () => {
    test("names the length", () => {
      assert.equal(secretProblem("short", "short", 12), "account.tooShort");
    });
  });

  describe("when the entries differ", () => {
    test("names the mismatch", () => {
      assert.equal(secretProblem("long enough phrase", "long enough phrasE", 12), "account.mismatch");
    });
  });

  describe("when long enough and equal", () => {
    test("finds nothing", () => {
      assert.equal(secretProblem("long enough phrase", "long enough phrase", 12), undefined);
    });
  });
});

describe("passwordRefusal", () => {
  test("names a wrong password", () => {
    assert.equal(passwordRefusal({ status: 401, message: "wrong username or password" }), "account.wrongPassword");
  });

  test("names too many tries", () => {
    assert.equal(passwordRefusal({ status: 429, message: "too many", code: "slow_down" }), "consent.tooMany");
  });

  describe("with another refusal", () => {
    test("gives nothing", () => {
      assert.equal(passwordRefusal({ status: 400, message: "too short" }), undefined);
    });
  });
});

describe("restarted", () => {
  describe("when ready after going down", () => {
    test("is done", () => {
      assert.equal(restarted(true, true, 0), true);
    });
  });

  describe("when ready without going down", () => {
    test("waits for the fallback", () => {
      assert.equal(restarted(true, false, RESTART_FALLBACK_MS - 1), false);
    });

    test("is done after the fallback", () => {
      assert.equal(restarted(true, false, RESTART_FALLBACK_MS), true);
    });
  });

  describe("when not ready", () => {
    test("waits", () => {
      assert.equal(restarted(false, true, RESTART_FALLBACK_MS * 10), false);
    });
  });
});
