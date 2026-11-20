import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { attachmentName } from "./api.ts";

describe("attachmentName", () => {
  test("reads the quoted file name", () => {
    assert.equal(attachmentName('attachment; filename="connector-engine-backup-20261120T101500Z.cebak"'), "connector-engine-backup-20261120T101500Z.cebak");
  });

  test("reads a bare file name", () => {
    assert.equal(attachmentName("attachment; filename=setup.setup.yaml"), "setup.setup.yaml");
  });

  describe("without a header", () => {
    test("gives nothing", () => {
      assert.equal(attachmentName(null), undefined);
    });
  });
});
