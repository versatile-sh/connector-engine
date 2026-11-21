import type { Agent, RowOwner } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { InlineConfirm } from "../ui/InlineConfirm";
import { rowMotion, type RowPhase } from "../ui/kept";
import { KeyConnectors } from "./KeyConnectors";
import { Tag } from "../ui/Tag";
import { button } from "../ui/styles";
import type { RevokeEvent, RevokeState } from "./revoke";

type KeyRowProps = { entry: Agent; owner?: RowOwner | undefined; flow: RevokeState; error?: string; phase?: RowPhase; dispatch(event: RevokeEvent): void; onConfirm(): void };

/** One access key: its name and expiry, whether an app connected it through the browser sign-in (new until the owner has seen it), its owner when the list shows everyone's keys, its mode on each connector, a visible Revoke, and in place of it the inline question that revokes the key for good. In a list narrower than 28rem the name takes its own line. */
export function KeyRow({ entry, owner, flow, error, phase, dispatch, onConfirm }: KeyRowProps) {
  const t = useT();
  const locale = useLocale();
  const expired = new Date(entry.expires_at) < new Date();
  const asking = (flow.kind === "confirming" || flow.kind === "revoking") && flow.name === entry.name;
  const revoking = flow.kind === "revoking" && flow.name === entry.name;
  return (
    <li {...rowMotion(phase)} className={`border-b border-line px-4 py-3 transition-opacity duration-250 last:border-b-0 sm:px-5 ${revoking ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span className={`size-1.5 shrink-0 rounded-full ${expired ? "bg-amber-400" : "bg-lime"}`} aria-hidden="true" />
        <code title={entry.name} className="min-w-0 grow basis-[calc(100%-1.5rem)] truncate font-mono text-sm font-medium @md:basis-0" translate="no">
          {entry.name}
        </code>
        <span className="font-mono text-xs text-muted @max-md:ml-5.5 @max-md:min-w-0 @max-md:grow @max-md:basis-0">{t("keys.until", { date: new Date(entry.expires_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) })}</span>
        {owner ? <Tag title={t("keys.owner")}>{owner.username || t("people.gone")}</Tag> : null}
        {expired ? <Tag tone="amber">{t("keys.expired")}</Tag> : null}
        {entry.signed_out_at ? <Tag tone="amber">{t("keys.signedOut")}</Tag> : null}
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
        <p className="mt-1 flex flex-wrap items-center gap-2 pl-5.5 font-mono text-xs text-muted">
          {entry.new ? <Tag tone="lime">{t("keys.newBadge")}</Tag> : null}
          {t("keys.viaBrowser")}
        </p>
      ) : null}
      <KeyConnectors agent={entry.name} />
      {asking ? (
        <InlineConfirm
          label={t("keys.revokeLabel", { name: entry.name })}
          question={t("keys.revokeAsk", { name: entry.name })}
          confirm={t("keys.revokeConfirm")}
          working={t("keys.revoking")}
          busy={revoking}
          error={error}
          onCancel={() => dispatch({ type: "cancel" })}
          onConfirm={onConfirm}
        />
      ) : null}
    </li>
  );
}
