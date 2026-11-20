import type { SkillFacts, SkillSource } from "../api";

/** A size the way the file list shows it: bytes, then KB and MB with one decimal. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** The first seven characters of a commit, as a page about it shows it. */
export const shortCommit = (commit: string) => commit.slice(0, 7);

/** The total size of a skill's files. */
export const totalBytes = (files: readonly { bytes: number }[]) => files.reduce((sum, file) => sum + file.bytes, 0);

/** The page about a skill's folder: the catalog's link, or the repository tree at the pinned commit; an `owner/repo` repository is read as GitHub. */
export function sourceLink(source: SkillSource): string {
  if (source.url) return source.url;
  const repository = /^https?:\/\//.test(source.repository) ? source.repository.replace(/\/+$/, "") : `https://github.com/${source.repository}`;
  return [`${repository}/tree/${source.commit}`, source.path].filter(Boolean).join("/");
}

/** What the owner is warned about before and after a skill is imported. */
export type SkillWarning = "scripts" | "metadataOnly" | "needs";

/** The warnings of a skill, most serious first. */
export function skillWarnings(skill: Pick<SkillFacts, "has_scripts" | "redistribution"> & { unbound?: string[] | undefined }): SkillWarning[] {
  const out: SkillWarning[] = [];
  if (skill.has_scripts) out.push("scripts");
  if (skill.redistribution === "metadata_only") out.push("metadataOnly");
  if (skill.unbound && skill.unbound.length > 0) out.push("needs");
  return out;
}
