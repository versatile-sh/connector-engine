import { useT } from "./i18n/I18nProvider";
import { chooseTheme, THEME_CHOICES, useThemeChoice } from "./theme";
import { Segmented } from "./ui/Segmented";

/** The console's theme as one segmented control: system, light or dark. */
export function ThemeSwitch({ className = "" }: { className?: string }) {
  const t = useT();
  const choice = useThemeChoice();
  return <Segmented label={t("theme.label")} size="sm" value={choice} onChange={chooseTheme} className={className} options={THEME_CHOICES.map((value) => ({ value, text: t(`theme.${value}`) }))} />;
}
