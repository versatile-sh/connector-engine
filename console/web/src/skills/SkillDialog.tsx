import { useMutation } from "@tanstack/react-query";
import { useState, type RefObject } from "react";
import { errorText, type Access, type Skill } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { useT } from "../i18n/I18nProvider";
import { removeSkill, setSkillAccess } from "../mutations";
import { Sheet } from "../ui/Sheet";
import { button } from "../ui/styles";
import { SkillFactsView } from "./SkillFactsView";
import { SkillOverview } from "./SkillOverview";

function KeyChoice({ skill, agent, access }: { skill: Skill; agent: string; access: Access }) {
  const t = useT();
  const set = useMutation(setSkillAccess);
  return (
    <li className="flex flex-col gap-2 border-b border-line py-3 last:border-b-0 @lg:flex-row @lg:items-center @lg:gap-6">
      <div className="min-w-0 flex-1">
        <code className="font-mono text-sm font-semibold break-all text-paper" translate="no">
          {agent}
        </code>
        {set.isError ? (
          <p role="alert" className="mt-1 text-sm text-amber">
            {errorText(set.error)}
          </p>
        ) : null}
      </div>
      <AccessPicker name={t("skills.keyName", { skill: skill.alias, key: agent })} of="skill" value={set.isPending ? set.variables.access : access} disabled={set.isPending} onChange={(next) => set.mutate({ skill: skill.id, agent, access: next })} />
    </li>
  );
}

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
            {skill.access.length === 0 ? (
              <p className="py-4 text-sm text-muted">{t("skills.noKeys")}</p>
            ) : (
              <ul className="@container mt-1">
                {skill.access.map((key) => (
                  <KeyChoice key={key.agent} skill={skill} agent={key.agent} access={key.access} />
                ))}
              </ul>
            )}
          </div>
        </div>
        <RemoveSkill skill={skill} />
      </div>
    </Sheet>
  );
}
