import { useT } from "./i18n/I18nProvider";
import { toggleTheme } from "./theme";
import { Icon } from "./ui/Icon";
import { ICON_BUTTON } from "./ui/styles";

/** The console's theme as one icon button: a sun in dark, a moon in light; CSS picks which, so a swap renders nothing in React. */
export function ThemeSwitch({ className = "" }: { className?: string }) {
  const t = useT();
  return (
    <button
      type="button"
      className={`${ICON_BUTTON} ${className}`}
      onClick={(event) => {
        const box = event.currentTarget.getBoundingClientRect();
        toggleTheme(event.detail === 0 ? { x: box.left + box.width / 2, y: box.top + box.height / 2 } : { x: event.clientX, y: event.clientY });
      }}
    >
      <span className="dark-only contents">
        <Icon name="sun" className="size-4" />
        <span className="sr-only">{t("theme.toLight")}</span>
      </span>
      <span className="light-only contents">
        <Icon name="moon" className="size-4" />
        <span className="sr-only">{t("theme.toDark")}</span>
      </span>
    </button>
  );
}
