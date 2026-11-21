import { useMutation } from "@tanstack/react-query";
import { errorText, type Access, type Skill } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { ChoiceRow } from "../connectors/ChoiceRow";
import { commonAccess } from "../connectors/summary";
import { useT } from "../i18n/I18nProvider";
import { setSkillAccess, setSkillAccessAll } from "../mutations";
import { useViewer } from "../people/useViewer";
import { LongList } from "../ui/LongList";
import { WELL } from "../ui/styles";
import type { RowPhase } from "../ui/kept";

function KeyChoice({ skill, agent, access, phase }: { skill: Skill; agent: string; access: Access; phase: RowPhase }) {
  const t = useT();
  const set = useMutation(setSkillAccess);
  return (
    <ChoiceRow
      phase={phase}
      name={agent}
      description={
        set.isError ? (
          <p role="alert" className="text-sm text-amber-400">
            {errorText(set.error)}
          </p>
        ) : null
      }
    >
      <AccessPicker name={t("skills.keyName", { skill: skill.alias, key: agent })} of="skill" value={set.isPending ? set.variables.access : access} disabled={set.isPending} onChange={(next) => set.mutate({ skill: skill.id, agent, access: next })} />
    </ChoiceRow>
  );
}

/** Every access key's off, on or ask on a skill as compact rows, under one choice that sets all of them in one request for a viewer who manages skills; past a few keys, a filter and Show all. */
export function KeyChoices({ skill }: { skill: Skill }) {
  const t = useT();
  const manage = useViewer().can("skills.manage");
  const all = useMutation(setSkillAccessAll);
  const kept = all.data?.keys.filter((key) => key.error) ?? [];
  if (skill.access.length === 0) return <p className="py-4 text-sm text-muted">{t("skills.noKeys")}</p>;
  return (
    <>
      {manage && skill.access.length > 1 ? (
        <div className="@container mt-3 grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 px-4 pb-3 @md:gap-x-6 sm:px-5">
          <span className="text-sm font-medium">{t("skills.allKeys", { count: skill.access.length })}</span>
          <AccessPicker
            name={t("skills.allKeysName", { skill: skill.alias })}
            of="skill"
            value={all.isPending ? all.variables.access : commonAccess(skill.access)}
            disabled={all.isPending}
            onChange={(access) => all.mutate({ skill: skill.id, access })}
          />
        </div>
      ) : null}
      <p role="alert" className="mt-2 text-sm text-amber-400 empty:hidden">
        {all.isError ? errorText(all.error) : kept.length > 0 ? t("skills.allKeysKept", { keys: kept.map((key) => key.agent).join(", ") }) : ""}
      </p>
      <LongList rows={skill.access} textOf={(key) => key.agent} keyOf={(key) => key.agent} label={t("keys.filter")}>
        {(shown) => (
          <ul className={WELL}>
            {shown.map(({ row: key, phase }) => (
              <KeyChoice key={key.agent} skill={skill} agent={key.agent} access={key.access} phase={phase} />
            ))}
          </ul>
        )}
      </LongList>
    </>
  );
}
