import type { Preview } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { PRESS, TOUCH } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useCopy } from "../ui/useCopy";
import { hasLicence, shortCommit } from "../skills/summary";

const PILL = "max-w-full min-w-0 rounded-full bg-paper/10 px-2.5 py-0.5";

type ChipProps = { text: string; tone?: "muted" | "paper" | "amber"; title?: string };

function Chip({ text, tone = "muted", title }: ChipProps) {
  return (
    <li className={PILL}>
      <Tag tone={tone} title={title} className="max-w-full">
        <span className="truncate">{text}</span>
      </Tag>
    </li>
  );
}

function CommitChip({ commit }: { commit: string }) {
  const t = useT();
  const { result, copy } = useCopy();
  const short = shortCommit(commit);
  return (
    <li className={PILL}>
      <button
        type="button"
        title={commit}
        aria-label={t("import.copyCommit", { commit: short })}
        onClick={() => void copy(commit)}
        className={`relative inline-flex max-w-full items-center gap-1.5 font-mono text-xs leading-5 text-muted hover:text-paper ${TOUCH} ${PRESS}`}
      >
        <span className="truncate" translate="no">
          {short}
        </span>
        <Icon name={result === "copied" ? "check" : "copy"} className={`size-3 ${result === "copied" ? "text-lime" : ""}`} />
      </button>
      <span aria-live="polite" className="sr-only">
        {result === "copied" ? t("common.copied") : result === "failed" ? t("common.copyFailed") : ""}
      </span>
    </li>
  );
}

/** The quiet row of facts under an install link's title: what it is, who made it, then a skill's licence and the short commit it is pinned to (the full hash in its tooltip and its copy button) or a connector's version. */
export function MetaChips({ preview }: { preview: Preview }) {
  const t = useT();
  const { skill } = preview;
  return (
    <ul className="flex max-w-full flex-wrap items-center justify-center gap-2 lg:justify-start">
      <Chip tone="paper" text={t(skill ? "import.badge.skill" : "import.badge.connector")} />
      {preview.publisher ? <Chip text={t("import.by", { publisher: preview.publisher })} title={preview.publisher} /> : null}
      {skill ? (
        <>
          <Chip tone={hasLicence(skill.licence) ? "muted" : "amber"} text={`${t("skills.licenceLabel")} ${hasLicence(skill.licence) ? t("skills.licence", { spdx: skill.licence.spdx }) : t("skills.noLicence")}`} />
          <CommitChip commit={skill.source.commit} />
        </>
      ) : preview.version ? (
        <Chip text={preview.version} title={`${preview.name}@${preview.version}`} />
      ) : null}
    </ul>
  );
}
