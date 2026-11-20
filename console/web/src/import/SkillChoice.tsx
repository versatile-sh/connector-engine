import type { Access, SkillFacts } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { useT } from "../i18n/I18nProvider";
import { SkillFactsView } from "../skills/SkillFactsView";

/** The review of a skill before it is added: what it is and carries, then one choice for every access key made so far. */
export function SkillChoice({ skill, access, onChange }: { skill: SkillFacts; access: Access; onChange(next: Access): void }) {
  const t = useT();
  return (
    <div className="flex flex-col gap-5">
      <SkillFactsView skill={skill} />
      <div className="flex flex-col gap-3 border-t border-line pt-4 sm:flex-row sm:items-center sm:gap-6">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{skill.keys.length > 0 ? t("import.skillKeys", { count: skill.keys.length }) : t("import.skillNoKeys")}</p>
          <p className="text-sm text-muted">{t("import.skillLater")}</p>
        </div>
        <AccessPicker name={skill.name} of="skill" value={access} onChange={onChange} />
      </div>
    </div>
  );
}
