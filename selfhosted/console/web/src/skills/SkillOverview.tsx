import type { Skill } from "../api";
import { AccessBar } from "../connectors/AccessBar";
import { accessCounts } from "../connectors/summary";
import { useT } from "../i18n/I18nProvider";
import { CAPTION } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { shortCommit } from "./summary";

type SkillOverviewProps = { skill: Skill; compact?: boolean };

/** What a skill card and its dialog both show: its alias at its commit and who made it, what it does (two lines when `compact`) and how its keys are set. */
export function SkillOverview({ skill, compact = false }: SkillOverviewProps) {
  const t = useT();
  const counts = accessCounts(skill.access);
  return (
    <>
      <p className={`${CAPTION} truncate`} translate="no">
        {skill.alias}@{shortCommit(skill.version)}
        {skill.publisher ? ` · ${t("import.by", { publisher: skill.publisher })}` : ""}
      </p>
      <p className={`mt-3 text-sm text-pretty text-muted ${compact ? "line-clamp-2 min-h-10" : "max-w-2xl"}`}>{skill.description}</p>
      <AccessBar counts={counts} className="mt-4" />
      <div className="mt-2.5 flex flex-wrap gap-x-3 gap-y-0.5">
        {skill.category ? <Tag>{skill.category}</Tag> : null}
        <Tag>{t("skills.keysOn", { count: counts.on + counts.ask, total: skill.access.length })}</Tag>
        {counts.on > 0 ? <Tag tone="paper">{t("access.count.on", { count: counts.on })}</Tag> : null}
        {counts.ask > 0 ? <Tag tone="lime">{t("access.count.ask", { count: counts.ask })}</Tag> : null}
        {skill.state === "quarantined" ? <Tag tone="amber" dot>{t("connectors.changed", { count: 1 })}</Tag> : null}
      </div>
    </>
  );
}
