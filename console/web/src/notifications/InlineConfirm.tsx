import { useT } from "../i18n/I18nProvider";
import { button } from "../ui/styles";

type InlineConfirmProps = { label: string; question: string; confirm: string; working: string; busy: boolean; error?: string | undefined; onCancel(): void; onConfirm(): void };

/** The inline question that removes something for good, in place of its Remove button; Escape cancels. */
export function InlineConfirm({ label, question, confirm, working, busy, error, onCancel, onConfirm }: InlineConfirmProps) {
  const t = useT();
  return (
    <div
      role="group"
      aria-label={label}
      onKeyDown={(event) => {
        if (event.key === "Escape") onCancel();
      }}
      className="swap-in bubble mt-3 flex flex-wrap items-center justify-between gap-3 border border-amber/40 bg-amber/5 py-2.5 pr-2.5 pl-4"
    >
      <p className="min-w-0 flex-1 basis-48 text-sm [overflow-wrap:anywhere]">{question}</p>
      <div className="flex shrink-0 gap-2">
        <button type="button" autoFocus disabled={busy} className={button({ variant: "quiet", size: "xs" })} onClick={onCancel}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={busy} className={`${button({ variant: "danger", size: "xs" })} min-w-28`} onClick={onConfirm}>
          {busy ? working : confirm}
        </button>
      </div>
      <p role="alert" className="w-full text-sm text-amber empty:hidden">
        {error ?? ""}
      </p>
    </div>
  );
}
