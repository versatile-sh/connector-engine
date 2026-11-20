import { useMutation } from "@tanstack/react-query";
import { useState, type RefObject } from "react";
import { errorText, type Skill } from "../api";
import { useT } from "../i18n/I18nProvider";
import { removeSkill } from "../mutations";
import { Sheet } from "../ui/Sheet";
import { button } from "../ui/styles";
import { KeyChoices } from "./KeyChoices";
import { SkillFactsView } from "./SkillFactsView";
import { SkillOverview } from "./SkillOverview";

function RemoveSkill({ skill }: { skill: Skill }) {
  const t = useT();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation(removeSkill);
  return (
    <>
      <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
        {confirming ? (
          <div role="group" aria-label={t("common.remove")} className="swap-in bubble flex flex-wrap items-center gap-3 border border-amber/40 bg-amber/5 py-2 pr-2 pl-4">
            <p className="min-w-0 text-sm [overflow-wrap:anywhere]">{t("skills.removeAsk", { name: skill.alias })}</p>
            <button type="button" autoFocus disabled={remove.isPending} className={button({ variant: "quiet", size: "xs" })} onClick={() => setConfirming(false)}>
              {t("common.cancel")}
            </button>
            <button type="button" disabled={remove.isPending} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.mutate(skill.id)}>
              {t("skills.removeConfirm")}
            </button>
          </div>
        ) : (
          <button type="button" className={button({ variant: "danger", size: "xs" })} onClick={() => setConfirming(true)}>
            {t("common.remove")}
          </button>
        )}
      </div>
      {remove.isError ? (
        <p role="alert" className="mt-2 text-right text-sm text-amber">
          {errorText(remove.error)}
        </p>
      ) : null}
    </>
  );
}

type SkillDialogProps = { skill: Skill; origin: RefObject<HTMLElement | null>; onClose(): void };

/** One skill in a modal grown out of its card: the card's overview in full, then its warnings, licence, source and files, the off, on or ask of each key, and Remove. */
export function SkillDialog({ skill, origin, onClose }: SkillDialogProps) {
  const t = useT();
  return (
    <Sheet title={skill.title} closeLabel={t("common.close")} onClose={onClose} origin={origin} wide>
      <SkillOverview skill={skill} />
      <div className="@container mt-5 border-t border-line pt-5">
        <div className="grid gap-x-10 gap-y-6 @3xl:grid-cols-2">
          <SkillFactsView skill={skill} unbound={skill.unbound} />
          <div className="min-w-0">
            <h3 className="text-base font-medium">{t("skills.perKey")}</h3>
            <p className="text-sm text-muted">{t("skills.perKeyIntro")}</p>
            <KeyChoices skill={skill} />
          </div>
        </div>
        <RemoveSkill skill={skill} />
      </div>
    </Sheet>
  );
}
