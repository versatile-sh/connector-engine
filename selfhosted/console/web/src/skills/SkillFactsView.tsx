import type { SkillFacts } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Disclosure } from "../ui/Disclosure";
import { Notice } from "../ui/Notice";
import { WELL } from "../ui/styles";
import { formatBytes, shortCommit, skillWarnings, sourceLink, totalBytes } from "./summary";

type SkillFactsViewProps = { skill: SkillFacts; unbound?: string[] | undefined };

/** What a skill is: its warnings first (scripts, no licence, a missing connector), then licence, source at its commit, and every file with its size. */
export function SkillFactsView({ skill, unbound }: SkillFactsViewProps) {
  const t = useT();
  const warnings = skillWarnings({ ...skill, unbound });
  const link = sourceLink(skill.source);
  const licence = skill.licence.spdx && skill.licence.spdx !== "NONE" ? t("skills.licence", { spdx: skill.licence.spdx }) : t("skills.noLicence");
  return (
    <div className="flex flex-col gap-4">
      {warnings.length > 0 ? (
        <div className="flex flex-col gap-2">
          {warnings.map((warning) => (
            <Notice key={warning} live={false}>
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
          <a href={link} target="_blank" rel="noreferrer noopener" className="text-paper underline decoration-line-strong underline-offset-4 transition-colors hover:decoration-lime" title={link} translate="no">
            {[skill.source.repository.replace(/^https?:\/\/(github\.com\/)?/, ""), skill.source.path].filter(Boolean).join("/")}@{shortCommit(skill.source.commit)}
          </a>
          <span className="sr-only"> {t("common.opensNewTab")}</span>
        </dd>
      </dl>
      <Disclosure
        className={WELL}
        summary={
          <>
            <span>{t("skills.files", { count: skill.files.length })}</span>
            <span className="font-mono text-xs text-muted">{formatBytes(totalBytes(skill.files))}</span>
          </>
        }
      >
        <ul className="border-t border-line px-4 py-2">
          {skill.files.map((file) => (
            <li key={file.path} className="flex items-center justify-between gap-4 py-1 font-mono text-xs">
              <code className={`min-w-0 truncate ${file.path.startsWith("scripts/") ? "text-amber-400" : "text-paper"}`} translate="no" title={file.path}>
                {file.path}
              </code>
              <span className="shrink-0 text-muted tabular-nums">{formatBytes(file.bytes)}</span>
            </li>
          ))}
        </ul>
      </Disclosure>
    </div>
  );
}
