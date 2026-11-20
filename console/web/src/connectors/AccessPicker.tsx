import { ACCESS, type Access } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";

type AccessPickerProps = { name: string; value: Access; disabled?: boolean; onChange(next: Access): void };

/** The three-way choice of one function, off, on or ask, with each choice explained on hover. */
export function AccessPicker({ name, value, disabled = false, onChange }: AccessPickerProps) {
  const t = useT();
  return (
    <Segmented<Access>
      label={t("access.label", { name })}
      size="sm"
      value={value}
      disabled={disabled}
      onChange={onChange}
      className="w-full sm:w-auto"
      options={ACCESS.map((access) => ({ value: access, text: t(`access.${access}`), hint: t(`access.hint.${access}`) }))}
    />
  );
}
