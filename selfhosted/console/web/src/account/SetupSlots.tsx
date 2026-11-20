import { useQuery } from "@tanstack/react-query";
import type { SlotChoice, SlotView } from "../api";
import { SlotTags } from "../access/SlotRow";
import { useT } from "../i18n/I18nProvider";
import { keysQuery } from "../queries";
import { Choice } from "../ui/Choice";
import { Select } from "../ui/Select";
import { Switch } from "../ui/Switch";
import { SUBTITLE, WELL } from "../ui/styles";

type SetupSlotsProps = {
  sets: readonly SlotView[];
  choices: Readonly<Record<string, SlotChoice>>;
  onChoose(set: string, choice: SlotChoice): void;
  existingKeys: boolean;
  onExistingKeys(on: boolean): void;
};

const ACTIONS = ["keep", "apply", "discard"] as const;

/** The review's answers for a setup's permission sets: per set keep it, apply it to a key made already, picked here with no key chosen for the owner, or discard it, and whether the keys made already get the new connectors. */
export function SetupSlots({ sets, choices, onChoose, existingKeys, onExistingKeys }: SetupSlotsProps) {
  const t = useT();
  const keys = (useQuery(keysQuery).data?.agents ?? []).map((agent) => ({ value: agent.name, text: agent.name }));
  const pick = (set: string, action: SlotChoice["action"]) => onChoose(set, action === "apply" ? { action, key: "" } : { action });
  return (
    <section className="mt-5">
      {sets.length > 0 ? (
        <>
          <h3 className={SUBTITLE}>{t("backup.sets")}</h3>
          <p className="mt-1 text-[0.8125rem] leading-snug text-pretty text-muted">{t("backup.setsHint")}</p>
          <ul className={`${WELL} @container mt-2`}>
            {sets.map((set) => {
              const choice = choices[set.name] ?? { action: "keep" };
              return (
                <li key={set.name} className="flex flex-col gap-2 border-b border-line px-4 py-3 last:border-b-0">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
                    <code className="min-w-0 font-mono text-sm font-medium [overflow-wrap:anywhere]" translate="no">
                      {set.name}
                    </code>
                    <SlotTags profiles={set.connectors} />
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Choice<SlotChoice["action"]>
                      label={t("backup.setAction", { set: set.name })}
                      value={choice.action}
                      options={ACTIONS.map((action) => ({ value: action, text: t(`backup.set.${action}`), disabled: action === "apply" && keys.length === 0 }))}
                      onChange={(action) => pick(set.name, action)}
                    />
                    {choice.action === "apply" ? (
                      <Select<string> label={t("slots.applyLabel", { set: set.name })} value={choice.key || undefined} options={keys} placeholder={t("slots.applyTo")} onChange={(key) => onChoose(set.name, { action: "apply", key })} className="w-48 max-w-full" />
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </>
      ) : null}
      <div className="mt-4">
        <Switch checked={existingKeys} onChange={onExistingKeys} hint={t("backup.existingKeysHint")}>
          {t("backup.existingKeys")}
        </Switch>
      </div>
    </section>
  );
}
