import { useT } from "../i18n/I18nProvider";
import { Arrow } from "../ui/Icon";
import { button } from "../ui/styles";

type LandingActionsProps = { label: string; pending: boolean; error: string | undefined; onInstall(): void; onCancel(): void };

/** What an install link ends in: a quiet Cancel and the one primary button that adds it, with the reason when adding failed above them. */
export function LandingActions({ label, pending, error, onInstall, onCancel }: LandingActionsProps) {
  const t = useT();
  return (
    <div className="flex flex-col gap-3">
      {error ? (
        <p role="alert" className="text-sm text-amber-400">
          {error}
        </p>
      ) : null}
      <div className="flex items-center gap-2">
        <button type="button" className={button({ variant: "quiet" })} onClick={onCancel}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={pending} className={`${button()} grow`} onClick={onInstall}>
          {pending ? t("import.installing") : label}
          <Arrow />
        </button>
      </div>
    </div>
  );
}
