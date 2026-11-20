import { LOCALES } from "./i18n";
import { useChooseLocale, useLocale, useT } from "./i18n/I18nProvider";
import { Segmented } from "./ui/Segmented";

/** The console's language as the one segmented control, in mono. */
export function LanguageSwitch({ className = "" }: { className?: string }) {
  const t = useT();
  const locale = useLocale();
  const choose = useChooseLocale();
  return (
    <Segmented
      label={t("lang.label")}
      size="sm"
      value={locale}
      onChange={choose}
      className={`font-mono ${className}`}
      options={LOCALES.map((option) => ({ value: option.value, text: option.short, lang: option.value }))}
    />
  );
}
