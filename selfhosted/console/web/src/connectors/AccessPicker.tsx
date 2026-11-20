import { ACCESS, type Access } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";
import { Select } from "../ui/Select";
import { wider } from "./profiles";

type AccessPickerProps = { name: string; value: Access | undefined; disabled?: boolean; of?: "function" | "skill" | "item"; choices?: readonly Access[]; most?: Access; onChange(next: Access): void };

const HINT = { function: "access.hint", skill: "access.skillHint", item: "access.itemHint" } as const;

/** The choice of one function, skill, item or key, off, on or ask (or the given subset), each explained on hover: a small segmented control where its container has 22rem, a `Select` where it has less, so its row never stacks; with no value nothing is chosen, and choices wider than `most` show disabled in the segmented control and are left out of the `Select`. */
export function AccessPicker({ name, value, disabled = false, of = "function", choices = ACCESS, most, onChange }: AccessPickerProps) {
  const t = useT();
  const label = t("access.label", { name });
  const options = choices.map((access) => {
    const capped = most !== undefined && wider(access, most);
    return { value: access, text: t(`access.${access}`), hint: capped ? t("profile.capped", { access: t(`access.${most}`) }) : t(`${HINT[of]}.${access}`), disabled: capped };
  });
  return (
    <>
      <span className="hidden @min-[22rem]:block">
        <Segmented<Access> label={label} size="sm" value={value} disabled={disabled} onChange={onChange} options={options} />
      </span>
      <span className="@min-[22rem]:hidden">
        <Select<Access> label={label} value={value} disabled={disabled} options={options.filter((option) => !option.disabled)} onChange={onChange} />
      </span>
    </>
  );
}
