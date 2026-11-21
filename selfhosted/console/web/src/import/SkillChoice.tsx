import type { Access, SkillFacts } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { useT } from "../i18n/I18nProvider";
import { SkillFactsView } from "../skills/SkillFactsView";

type SkillAccessProps = { skill: SkillFacts; access: Access; onChange(next: Access): void };

/** Who may load a skill: one choice for every access key made so far, beside its note from 28rem of room and under it when there is less. */
export function SkillAccess({ skill, access, onChange }: SkillAccessProps) {
  const t = useT();
  return (
    <div className="@container">
      <div className="grid grid-cols-1 items-center gap-3 @md:grid-cols-[minmax(0,1fr)_auto] @md:gap-x-6">
        <div className="min-w-0">
          <p className="text-sm font-medium">{skill.keys.length > 0 ? t("import.skillKeys", { count: skill.keys.length }) : t("import.skillNoKeys")}</p>
          <p className="text-sm text-muted">{t("import.skillLater")}</p>
        </div>
        <AccessPicker name={skill.name} of="skill" value={access} onChange={onChange} />
      </div>
    </div>
  );
}

/** The review of a skill before it is added: what it is and carries, then one choice for every access key made so far. */
export function SkillChoice({ skill, access, onChange }: SkillAccessProps) {
  return (
    <div className="flex flex-col gap-5">
      <SkillFactsView skill={skill} />
      <div className="border-t border-line pt-4">
        <SkillAccess skill={skill} access={access} onChange={onChange} />
      </div>
    </div>
  );
}
