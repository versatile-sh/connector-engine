import type { Skill } from "../api";
import { AccessBar } from "../connectors/AccessBar";
import { accessCounts } from "../connectors/summary";
import { useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { shortCommit } from "./summary";

type SkillOverviewProps = { skill: Skill; compact?: boolean };

/** What a skill card and its dialog both show: its alias at its commit and who made it, what it does (two lines when `compact`) and how its keys are set. */
export function SkillOverview({ skill, compact = false }: SkillOverviewProps) {
  const t = useT();
  const counts = accessCounts(skill.access);
  return (
    <>
      <p className="truncate font-mono text-xs text-muted" translate="no">
        {skill.alias}@{shortCommit(skill.version)}
        {skill.publisher ? ` · ${t("import.by", { publisher: skill.publisher })}` : ""}
      </p>
      <p className={`mt-3 text-sm text-muted [overflow-wrap:anywhere] ${compact ? "line-clamp-2 min-h-10" : "max-w-2xl"}`}>{skill.description}</p>
      <AccessBar counts={counts} className="mt-4" />
      <div className="mt-3 flex flex-wrap gap-1.5">
        {skill.category ? <Bubble>{skill.category}</Bubble> : null}
        <Bubble tone={counts.on + counts.ask > 0 ? "accent" : "neutral"}>{t("skills.keysOn", { count: counts.on + counts.ask, total: skill.access.length })}</Bubble>
        {counts.on > 0 ? <Bubble>{t("access.count.on", { count: counts.on })}</Bubble> : null}
        {counts.ask > 0 ? <Bubble tone="outline">{t("access.count.ask", { count: counts.ask })}</Bubble> : null}
        {skill.state === "quarantined" ? <Bubble tone="amber">{t("connectors.changed", { count: 1 })}</Bubble> : null}
      </div>
    </>
  );
}
