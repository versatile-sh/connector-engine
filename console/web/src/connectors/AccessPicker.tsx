import { ACCESS, type Access } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";

type AccessPickerProps = { name: string; value: Access; disabled?: boolean; of?: "function" | "skill" | "item"; choices?: readonly Access[]; onChange(next: Access): void };

const HINT = { function: "access.hint", skill: "access.skillHint", item: "access.itemHint" } as const;

/** The choice of one function, skill or item, off, on or ask (or the given subset), with each choice explained on hover. */
export function AccessPicker({ name, value, disabled = false, of = "function", choices = ACCESS, onChange }: AccessPickerProps) {
  const t = useT();
  return (
    <Segmented<Access>
      label={t("access.label", { name })}
      size="sm"
      value={value}
      disabled={disabled}
      onChange={onChange}
      className="w-full @lg:w-auto"
      options={choices.map((access) => ({ value: access, text: t(`access.${access}`), hint: t(`${HINT[of]}.${access}`) }))}
    />
  );
}
