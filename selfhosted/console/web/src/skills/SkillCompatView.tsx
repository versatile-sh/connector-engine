import type { CompatStatus, SkillFacts } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Notice } from "../ui/Notice";
import { CAPTION, SUBTITLE, WELL } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { reasonText, requireTags, warningText } from "./compat";

const TONE = { full: "lime", instructions: "amber", no: "amber" } as const satisfies Record<CompatStatus, "lime" | "amber">;

/** The app warnings of a skill: one amber note per AI app behind this gateway that cannot fully use it. Never blocks anything. */
export function CompatWarnings({ skill }: { skill: Pick<SkillFacts, "warnings"> }) {
  const t = useT();
  if (skill.warnings.length === 0) return null;
  return (
    <div className="flex flex-col gap-2">
      {skill.warnings.map((warning) => (
        <Notice key={warning.client} live={false}>
          {warningText(warning, t)}
        </Notice>
      ))}
    </div>
  );
}

/** What a skill needs and how each AI app fares with it: a Requires row of tags, then one row per app with a status dot, its state and a one-line reason; every value is derived from the skill's files, never tested. */
export function SkillCompatView({ skill }: { skill: Pick<SkillFacts, "needs" | "compat"> }) {
  const t = useT();
  const tags = requireTags(skill.needs);
  return (
    <section aria-labelledby="compat-title" className="flex flex-col gap-3">
      <p id="compat-title" className={SUBTITLE}>
        {t("compat.title")}
      </p>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-sm text-muted">{t("compat.requires")}</span>
        {tags.length > 0 ? tags.map((tag) => <Tag key={tag} tone="paper">{tag}</Tag>) : <Tag>{t("compat.requiresNothing")}</Tag>}
      </div>
      <table className={`${WELL} w-full border-collapse text-left text-sm`}>
        <caption className="sr-only">{t("compat.title")}</caption>
        <thead className="sr-only">
          <tr>
            <th scope="col">{t("compat.app")}</th>
            <th scope="col">{t("compat.status")}</th>
            <th scope="col">{t("compat.why")}</th>
          </tr>
        </thead>
        <tbody>
          {skill.compat.map((entry) => (
            <tr key={entry.client} className="border-t border-line align-top first:border-t-0">
              <th scope="row" className="px-4 py-2 font-normal">
                {entry.name}
              </th>
              <td className="px-2 py-2 whitespace-nowrap">
                <Tag tone={TONE[entry.status]} dot>
                  {t(`compat.status.${entry.status}`)}
                </Tag>
              </td>
              <td className="px-4 py-2 text-muted">{reasonText(entry, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className={CAPTION}>{t("compat.derived")}</p>
    </section>
  );
}
