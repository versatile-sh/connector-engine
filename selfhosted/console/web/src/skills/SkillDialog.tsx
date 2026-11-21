import { useMutation } from "@tanstack/react-query";
import { useState, type RefObject } from "react";
import { errorText, type Skill } from "../api";
import { useT } from "../i18n/I18nProvider";
import { removeSkill } from "../mutations";
import { useViewer } from "../people/useViewer";
import { InlineConfirm } from "../ui/InlineConfirm";
import { Sheet } from "../ui/Sheet";
import { SUBTITLE, button } from "../ui/styles";
import { KeyChoices } from "./KeyChoices";
import { SkillFactsView } from "./SkillFactsView";
import { SkillOverview } from "./SkillOverview";

function RemoveSkill({ skill }: { skill: Skill }) {
  const t = useT();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation(removeSkill);
  return confirming ? (
    <InlineConfirm
      label={t("common.remove")}
      question={t("skills.removeAsk", { name: skill.alias })}
      confirm={t("skills.removeConfirm")}
      working={t("notify.removing")}
      busy={remove.isPending}
      error={remove.isError ? errorText(remove.error) : undefined}
      onCancel={() => setConfirming(false)}
      onConfirm={() => remove.mutate(skill.id)}
    />
  ) : (
    <div className="mt-4 flex justify-end">
      <button type="button" className={button({ variant: "danger", size: "xs" })} onClick={() => setConfirming(true)}>
        {t("common.remove")}
      </button>
    </div>
  );
}

type SkillDialogProps = { skill: Skill; origin: RefObject<HTMLElement | null>; onClose(): void };

/** One skill in a modal grown out of its card: the card's overview in full, then its warnings, licence, source and files, the off, on or ask of each key, and Remove for a viewer who manages skills. */
export function SkillDialog({ skill, origin, onClose }: SkillDialogProps) {
  const t = useT();
  const manage = useViewer().can("skills.manage");
  return (
    <Sheet title={skill.title} closeLabel={t("common.close")} onClose={onClose} origin={origin} wide>
      <SkillOverview skill={skill} />
      <div className="@container mt-5 border-t border-line pt-5">
        <div className="grid gap-x-10 gap-y-6 @3xl:grid-cols-2">
          <div className="min-w-0">
            <h3 className={`${SUBTITLE} mb-3`}>{t("skills.about")}</h3>
            <SkillFactsView skill={skill} unbound={skill.unbound} details />
          </div>
          <div className="min-w-0">
            <h3 className={SUBTITLE}>{t("skills.perKey")}</h3>
            <p className="text-sm text-muted">{t("skills.perKeyIntro")}</p>
            <KeyChoices skill={skill} />
          </div>
        </div>
        {manage ? <RemoveSkill skill={skill} /> : null}
      </div>
    </Sheet>
  );
}
