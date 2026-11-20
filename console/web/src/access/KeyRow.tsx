import type { Agent } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { button } from "../ui/styles";
import type { RevokeEvent, RevokeState } from "./revoke";

type KeyRowProps = { entry: Agent; flow: RevokeState; error?: string; dispatch(event: RevokeEvent): void; onConfirm(): void };

/** One access key: its name and expiry, whether an app connected it through the browser sign-in (new until the owner has seen it), a visible Revoke, and in place of it the inline question that revokes the key for good. In a list narrower than 28rem the name takes its own line. */
export function KeyRow({ entry, flow, error, dispatch, onConfirm }: KeyRowProps) {
  const t = useT();
  const locale = useLocale();
  const expired = new Date(entry.expires_at) < new Date();
  const asking = (flow.kind === "confirming" || flow.kind === "revoking") && flow.name === entry.name;
  const revoking = flow.kind === "revoking" && flow.name === entry.name;
  return (
    <li className={`border-b border-line py-3 last:border-b-0 transition-opacity duration-300 ${revoking ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span className={`size-2 shrink-0 rounded-full ${expired ? "bg-amber" : "bg-lime"}`} aria-hidden="true" />
        <code title={entry.name} className="min-w-0 grow basis-[calc(100%-1.5rem)] truncate font-mono text-sm font-semibold @md:basis-0" translate="no">
          {entry.name}
        </code>
        <span className="font-mono text-xs text-muted @max-md:ml-6 @max-md:min-w-0 @max-md:grow @max-md:basis-0">{t("keys.until", { date: new Date(entry.expires_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) })}</span>
        {expired ? <Bubble tone="amber">{t("keys.expired")}</Bubble> : null}
        {asking ? null : (
          <button
            type="button"
            autoFocus={flow.kind === "idle" && flow.returnTo === entry.name}
            aria-label={t("keys.revokeLabel", { name: entry.name })}
            className={button({ variant: "danger", size: "xs" })}
            onClick={() => dispatch({ type: "ask", name: entry.name })}
          >
            {t("common.revoke")}
          </button>
        )}
      </div>
      {entry.via_browser ? (
        <p className="mt-1 flex flex-wrap items-center gap-2 pl-6 font-mono text-xs text-muted">
          {entry.new ? <Bubble tone="accent">{t("keys.newBadge")}</Bubble> : null}
          {t("keys.viaBrowser")}
        </p>
      ) : null}
      {asking ? (
        <div
          role="group"
          aria-label={t("keys.revokeLabel", { name: entry.name })}
          onKeyDown={(event) => {
            if (event.key === "Escape") dispatch({ type: "cancel" });
          }}
          className="swap-in bubble mt-3 flex flex-wrap items-center justify-between gap-3 border border-amber/40 bg-amber/5 py-2.5 pr-2.5 pl-4"
        >
          <p className="min-w-0 grow basis-56 text-sm">{t("keys.revokeAsk", { name: entry.name })}</p>
          <div className="ml-auto flex shrink-0 gap-2">
            <button type="button" autoFocus disabled={revoking} className={button({ variant: "quiet", size: "xs" })} onClick={() => dispatch({ type: "cancel" })}>
              {t("common.cancel")}
            </button>
            <button type="button" disabled={revoking} className={`${button({ variant: "danger", size: "xs" })} min-w-28`} onClick={onConfirm}>
              {revoking ? t("keys.revoking") : t("keys.revokeConfirm")}
            </button>
          </div>
          <p role="alert" className="w-full text-sm text-amber empty:hidden">
            {error ?? ""}
          </p>
        </div>
      ) : null}
    </li>
  );
}
