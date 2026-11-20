import type { SkillFacts } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Notice } from "../ui/Notice";
import { formatBytes, shortCommit, skillWarnings, sourceLink, totalBytes } from "./summary";

type SkillFactsViewProps = { skill: SkillFacts; unbound?: string[] | undefined };

/** What a skill is: its warnings first (scripts, no licence, a missing connector), then licence, source at its commit, and every file with its size. */
export function SkillFactsView({ skill, unbound }: SkillFactsViewProps) {
  const t = useT();
  const warnings = skillWarnings({ ...skill, unbound });
  const link = sourceLink(skill.source);
  const licence = skill.licence.spdx ? t("skills.licence", { spdx: skill.licence.spdx }) : t("skills.noLicence");
  return (
    <div className="flex flex-col gap-4">
      {warnings.length > 0 ? (
        <div className="flex flex-col gap-2">
          {warnings.map((warning) => (
            <Notice key={warning}>
              {warning === "scripts" ? t("skills.warn.scripts") : warning === "metadataOnly" ? t("skills.warn.metadataOnly") : t("skills.warn.needs", { names: (unbound ?? []).join(", ") })}
            </Notice>
          ))}
        </div>
      ) : null}
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm text-muted">
        <dt>{t("skills.licenceLabel")}</dt>
        <dd className="min-w-0 truncate font-mono text-xs leading-5 text-paper" translate="no">
          {licence}
          {skill.licence.from ? ` · ${t("skills.licenceFrom", { from: skill.licence.from })}` : ""}
        </dd>
        <dt>{t("skills.source")}</dt>
        <dd className="min-w-0 truncate font-mono text-xs leading-5">
          <a href={link} target="_blank" rel="noreferrer noopener" className="text-paper underline decoration-line-strong underline-offset-4 hover:decoration-lime" title={link} translate="no">
            {skill.source.repository.replace(/^https?:\/\//, "")}/{skill.source.path}@{shortCommit(skill.source.commit)}
          </a>
          <span className="sr-only"> {t("common.opensNewTab")}</span>
        </dd>
      </dl>
      <details className="group bubble border border-line-strong bg-ink-raised/60">
        <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-2.5 text-sm">
          <span>{t("skills.files", { count: skill.files.length })}</span>
          <span className="font-mono text-xs text-muted">{formatBytes(totalBytes(skill.files))}</span>
        </summary>
        <ul className="border-t border-line px-4 py-2">
          {skill.files.map((file) => (
            <li key={file.path} className="flex items-center justify-between gap-4 py-1 font-mono text-xs">
              <code className={`min-w-0 truncate ${file.path.startsWith("scripts/") ? "text-amber" : "text-paper"}`} translate="no" title={file.path}>
                {file.path}
              </code>
              <span className="shrink-0 text-muted tabular-nums">{formatBytes(file.bytes)}</span>
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}
