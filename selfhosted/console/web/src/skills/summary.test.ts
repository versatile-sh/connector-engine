import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { formatBytes, shortCommit, skillWarnings, sourceLink, totalBytes } from "./summary.ts";

const commit = "0123456789abcdef0123456789abcdef01234567";

describe("formatBytes", () => {
  test("keeps small sizes in bytes", () => assert.equal(formatBytes(226), "226 B"));
  test("shows kilobytes with one decimal", () => assert.equal(formatBytes(1536), "1.5 KB"));
  test("shows megabytes with one decimal", () => assert.equal(formatBytes(3 * 1024 * 1024), "3.0 MB"));
});

describe("shortCommit", () => {
  test("keeps seven characters", () => assert.equal(shortCommit(commit), "0123456"));
});

describe("totalBytes", () => {
  test("adds every file", () => assert.equal(totalBytes([{ bytes: 2 }, { bytes: 40 }]), 42));
});

describe("sourceLink", () => {
  describe("with a catalog link", () => {
    test("uses it", () => assert.equal(sourceLink({ repository: "https://github.com/a/b", path: "pdf", commit, url: "https://x.example/pdf" }), "https://x.example/pdf"));
  });
  describe("without one", () => {
    test("points at the tree at the pinned commit", () =>
      assert.equal(sourceLink({ repository: "https://github.com/a/b/", path: "skills/pdf", commit }), `https://github.com/a/b/tree/${commit}/skills/pdf`));
    test("reads owner/repo as GitHub and drops an empty root path", () =>
      assert.equal(sourceLink({ repository: "a/b", path: "", commit }), `https://github.com/a/b/tree/${commit}`));
  });
});

describe("skillWarnings", () => {
  describe("with scripts, no licence and a missing connector", () => {
    test("lists all three, scripts first", () =>
      assert.deepEqual(skillWarnings({ has_scripts: true, redistribution: "metadata_only", unbound: ["com.notion/mcp"] }), ["scripts", "metadataOnly", "needs"]));
  });
  describe("with nothing to warn about", () => {
    test("lists none", () => assert.deepEqual(skillWarnings({ has_scripts: false, redistribution: "bundled" }), []));
  });
});
