import { LOCALES } from "./i18n";
import { useChooseLocale, useLocale, useT } from "./i18n/I18nProvider";
import { TOUCH_ROW } from "./ui/styles";

/** The console's language as three small mono choices; the pressed one is lit like the site's chips. */
export function LanguageSwitch({ className = "" }: { className?: string }) {
  const t = useT();
  const locale = useLocale();
  const choose = useChooseLocale();
  return (
    <div role="group" aria-label={t("lang.label")} className={`flex h-9 shrink-0 items-center rounded-full border border-line-strong p-0.5 font-mono text-xs ${className}`}>
      {LOCALES.map((option) => (
        <button
          key={option.value}
          type="button"
          lang={option.value}
          aria-pressed={locale === option.value}
          onClick={() => choose(option.value)}
          className={`relative h-full rounded-full px-2.5 text-muted transition-colors duration-200 hover:text-paper aria-pressed:bg-paper/10 aria-pressed:text-paper ${TOUCH_ROW}`}
        >
          {option.short}
        </button>
      ))}
    </div>
  );
}
