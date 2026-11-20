import { ACCESS, type Access } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Choice } from "../ui/Choice";
import { wider } from "./profiles";

type AccessPickerProps = { name: string; value: Access | undefined; disabled?: boolean; of?: "function" | "skill" | "item"; choices?: readonly Access[]; most?: Access; onChange(next: Access): void };

const HINT = { function: "access.hint", skill: "access.skillHint", item: "access.itemHint" } as const;

/** The choice of one function, skill, item or key, off, on or ask (or the given subset), each explained on hover, as a `Choice` that fits its container; with no value nothing is chosen, and choices wider than `most` show but cannot be taken. */
export function AccessPicker({ name, value, disabled = false, of = "function", choices = ACCESS, most, onChange }: AccessPickerProps) {
  const t = useT();
  const options = choices.map((access) => {
    const capped = most !== undefined && wider(access, most);
    return { value: access, text: t(`access.${access}`), hint: capped ? t("profile.capped", { access: t(`access.${most}`) }) : t(`${HINT[of]}.${access}`), disabled: capped };
  });
  return <Choice<Access> label={t("access.label", { name })} value={value} disabled={disabled} options={options} onChange={onChange} />;
}
