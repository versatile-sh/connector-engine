import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";
import type { Scope } from "./viewer";

/** Mine or Everyone: whose rows a list shows, for a viewer who may see everyone's. */
export function ScopeSwitch({ scope, onChange, className = "" }: { scope: Scope; onChange(scope: Scope): void; className?: string }) {
  const t = useT();
  return (
    <Segmented<Scope>
      label={t("scope.label")}
      size="sm"
      value={scope}
      options={[
        { value: "mine", text: t("scope.mine") },
        { value: "everyone", text: t("scope.everyone") },
      ]}
      onChange={onChange}
      className={`w-fit ${className}`}
    />
  );
}
