import { useQuery } from "@tanstack/react-query";
import { useT } from "../i18n/I18nProvider";
import { slotsQuery } from "../queries";
import { Select } from "../ui/Select";

type SlotPickerProps = { value: string; onChange(slot: string): void; className?: string };

/** Which permission set a new access key starts with, the default first as `""`; nothing when no set waits to be claimed. */
export function SlotPicker({ value, onChange, className = "" }: SlotPickerProps) {
  const t = useT();
  const slots = useQuery(slotsQuery).data ?? [];
  if (slots.length === 0) return null;
  const options = [{ value: "", text: t("slots.default") }, ...slots.map((slot) => ({ value: slot.name, text: slot.name }))];
  return (
    <div className={className}>
      <p className="text-sm font-medium">{t("slots.useFrom")}</p>
      <Select<string> label={t("slots.useFrom")} value={value} options={options} onChange={onChange} className="mt-1.5 w-full" />
      <p className="mt-1.5 text-sm text-pretty text-muted">{t("slots.useFromHint")}</p>
    </div>
  );
}
