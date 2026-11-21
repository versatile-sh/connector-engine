import type { CompatStatus, SkillFacts } from "../api";
import { useT } from "../i18n/I18nProvider";
import { AppMark } from "../ui/AppMark";
import { Disclosure } from "../ui/Disclosure";
import { Notice } from "../ui/Notice";
import { CAPTION, WELL } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { reasonText, requireTags, warningText } from "./compat";

const TONE = { full: "lime", instructions: "amber", no: "amber" } as const satisfies Record<CompatStatus, "lime" | "amber">;

const ORDER = ["full", "instructions", "no"] as const satisfies readonly CompatStatus[];

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

type CompatFacts = Pick<SkillFacts, "needs" | "compat">;

function CompatTable({ skill }: { skill: CompatFacts }) {
  const t = useT();
  return (
    <div className="flex flex-col gap-2 border-t border-line p-3">
      <table className="w-full border-collapse text-left text-sm">
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
              <th scope="row" className="px-2 py-2 font-normal [overflow-wrap:anywhere]">
                {entry.name}
              </th>
              <td className="px-2 py-2">
                <Tag tone={TONE[entry.status]} dot>
                  {t(`compat.status.${entry.status}`)}
                </Tag>
              </td>
              <td className="px-2 py-2 text-muted [overflow-wrap:anywhere]">{reasonText(entry, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className={`${CAPTION} px-2`}>{t("compat.derived")}</p>
    </div>
  );
}

function AppGroups({ compat }: { compat: CompatFacts["compat"] }) {
  const t = useT();
  const groups = ORDER.map((status) => ({ status, entries: compat.filter((entry) => entry.status === status) })).filter((group) => group.entries.length > 0);
  const everywhere = groups.length === 1 && groups[0]?.status === "full";
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
      {groups.map(({ status, entries }) => (
        <div key={status} className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
          <span className="text-sm text-muted">{everywhere ? t("compat.all") : t(status === "full" ? "compat.worksIn" : `compat.status.${status}`)}</span>
          <ul className="flex flex-wrap gap-1.5">
            {entries.map((entry) => (
              <li key={entry.client} className="flex">
                <AppMark client={entry.client} name={entry.name} status={status} label={`${entry.name}: ${t(`compat.status.${status}`)}. ${reasonText(entry, t)}`} />
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

type SkillCompatViewProps = { skill: CompatFacts; details?: boolean };

/** What a skill needs and how each AI app fares with it, in one compact block: a Requires row of tags only when something is required, then the apps as small marks grouped by status (Works in, Instructions only, Does not run), or one "Works in every AI app" line when all work; each mark says its app and why in its tooltip and accessible name. With `details`, the full table of apps and reasons waits behind a Details disclosure. Every value is derived from the skill's files, never tested. */
export function SkillCompatView({ skill, details = false }: SkillCompatViewProps) {
  const t = useT();
  const tags = requireTags(skill.needs);
  if (tags.length === 0 && skill.compat.length === 0) return null;
  return (
    <section aria-label={t("compat.title")} className="flex flex-col gap-2.5">
      {tags.length > 0 ? (
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="text-sm text-muted">{t("compat.requires")}</span>
          {tags.map((tag) => (
            <Tag key={tag} tone="paper" title={tag} className="max-w-full">
              <span className="truncate">{tag}</span>
            </Tag>
          ))}
        </div>
      ) : null}
      <AppGroups compat={skill.compat} />
      {details && skill.compat.length > 0 ? (
        <Disclosure className={WELL} summary={<span>{t("compat.details")}</span>}>
          <CompatTable skill={skill} />
        </Disclosure>
      ) : null}
    </section>
  );
}
