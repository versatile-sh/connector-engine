import { useT } from "./i18n/I18nProvider";
import { toggleTheme, useTheme } from "./theme";
import { Icon } from "./ui/Icon";
import { ICON_BUTTON } from "./ui/styles";

/** The console's theme as one icon button: a sun in dark (switches to light), a moon in light (switches to dark), the new theme opening from the click. */
export function ThemeSwitch({ className = "" }: { className?: string }) {
  const t = useT();
  const theme = useTheme();
  const label = t(theme === "dark" ? "theme.toLight" : "theme.toDark");
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={`${ICON_BUTTON} ${className}`}
      onClick={(event) => {
        const box = event.currentTarget.getBoundingClientRect();
        toggleTheme(event.detail === 0 ? { x: box.left + box.width / 2, y: box.top + box.height / 2 } : { x: event.clientX, y: event.clientY });
      }}
    >
      <Icon name={theme === "dark" ? "sun" : "moon"} className="size-4" />
    </button>
  );
}
