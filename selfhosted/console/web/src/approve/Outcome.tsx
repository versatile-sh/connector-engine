import type { ApprovalView } from "../api";
import type { MessageKey } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { ACCENT_BAR } from "../ui/styles";

const ALLOWED = new Set(["approved", "executing", "executed"]);

const STATE: Record<string, MessageKey> = { approved: "approve.state.approved", executing: "approve.state.approved", executed: "approve.state.approved", denied: "approve.state.denied", expired: "approve.state.expired", cancelled: "approve.state.cancelled" };

type OutcomeProps = { approval: ApprovalView; first: boolean };

/** How a held call ended, calmly: the answer, who gave it, through what and when, and what the app does next. `first` is false when someone else had answered before. */
export function Outcome({ approval, first }: OutcomeProps) {
  const t = useT();
  const locale = useLocale();
  const approved = ALLOWED.has(approval.state);
  const ended = approval.state === "expired" || approval.state === "cancelled";
  const key = STATE[approval.state];
  const when = approval.decided_at ? new Date(approval.decided_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) : "";
  const who = approval.decided_by ?? "";
  const decided = who && approval.channel ? t("approve.decidedByVia", { who, channel: approval.channel, when }) : who ? t("approve.decidedBy", { who, when }) : when ? t("approve.decidedAt", { when }) : "";
  return (
    <div role="status" className="swap-in bubble relative flex items-start gap-3 border border-line-strong bg-ink-sunken py-4 pr-4 pl-5">
      <span aria-hidden="true" className={`${ACCENT_BAR} ${approved ? "bg-lime" : "bg-line-strong"}`} />
      <span aria-hidden="true" className={`grid size-9 shrink-0 place-items-center rounded-full ${approved ? "bg-lime text-ink" : "border border-line-strong text-muted"}`}>
        <Icon name={approved ? "check" : "close"} className="size-4" />
      </span>
      <div className="min-w-0">
        {!first ? <p className="text-sm text-amber">{t("approve.first")}</p> : null}
        <p className="text-lg leading-snug font-medium">{key ? t(key) : approval.state}</p>
        {decided ? <p className="mt-0.5 font-mono text-xs text-muted [overflow-wrap:anywhere]">{decided}</p> : null}
        {approved || ended || approval.state === "denied" ? (
          <p className="mt-2 text-sm text-muted [overflow-wrap:anywhere]">{t(approved ? "approve.resumes" : ended ? "approve.ended" : "approve.refused", { app: approval.app })}</p>
        ) : null}
      </div>
    </div>
  );
}
