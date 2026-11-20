import { ACCESS, type Access } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";
import { CONTROL } from "../ui/styles";

type AccessPickerProps = { name: string; value: Access | undefined; disabled?: boolean; of?: "function" | "skill" | "item"; choices?: readonly Access[]; onChange(next: Access): void };

const HINT = { function: "access.hint", skill: "access.skillHint", item: "access.itemHint" } as const;

/** The choice of one function, skill, item or key, off, on or ask (or the given subset), each explained on hover: a small segmented control where its container has 22rem, a select where it has less, so its row never stacks; with no value nothing is chosen. */
export function AccessPicker({ name, value, disabled = false, of = "function", choices = ACCESS, onChange }: AccessPickerProps) {
  const t = useT();
  const label = t("access.label", { name });
  const options = choices.map((access) => ({ value: access, text: t(`access.${access}`), hint: t(`${HINT[of]}.${access}`) }));
  return (
    <>
      <span className="hidden @min-[22rem]:block">
        <Segmented<Access> label={label} size="sm" value={value} disabled={disabled} onChange={onChange} options={options} />
      </span>
      <select
        aria-label={label}
        value={value ?? ""}
        disabled={disabled}
        onChange={(event) => {
          const next = choices.find((access) => access === event.target.value);
          if (next) onChange(next);
        }}
        className={`${CONTROL} h-8 pr-2 disabled:opacity-50 @min-[22rem]:hidden`}
      >
        {value === undefined ? <option value="">—</option> : null}
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.text}
          </option>
        ))}
      </select>
    </>
  );
}
