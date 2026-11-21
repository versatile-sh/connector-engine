import { useT } from "../i18n/I18nProvider";
import { AppMark } from "../ui/AppMark";
import { Select } from "../ui/Select";
import { APPS } from "../skills/compat";

type AppPickerProps = { value: string; onChange(app: string): void };

/** Which AI app a new access key is for, `""` when not stated, so the console can warn about skills that app cannot run. */
export function AppPicker({ value, onChange }: AppPickerProps) {
  const t = useT();
  const options = [{ value: "", text: t("compat.appNone") }, ...APPS.map((app) => ({ value: app.id, text: app.name, icon: <AppMark client={app.id} name={app.name} size="xs" /> }))];
  return (
    <div className="mb-5">
      <p className="text-sm font-medium">{t("compat.appLabel")}</p>
      <Select<string> label={t("compat.appLabel")} value={value} options={options} onChange={onChange} className="mt-1.5 w-full" />
      <p className="mt-1.5 text-sm text-pretty text-muted">{t("compat.appHint")}</p>
    </div>
  );
}
