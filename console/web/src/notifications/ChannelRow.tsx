import { useMutation } from "@tanstack/react-query";
import { errorText, type ChannelView } from "../api";
import { useT } from "../i18n/I18nProvider";
import { testChannel } from "../mutations";
import { Bubble } from "../ui/Bubble";
import { button } from "../ui/styles";
import { InlineConfirm } from "./InlineConfirm";
import { SETUP } from "./setup";
import type { useRemove } from "./useRemove";

const TONE = { connected: "accent", connecting: "neutral", error: "amber", notify_only: "outline" } as const;

type ChannelRowProps = { channel: ChannelView; remove: ReturnType<typeof useRemove> };

/** One channel: its label and kind, its status, the owner id and the last characters of its secret, a Test message, and an inline Remove. */
export function ChannelRow({ channel, remove }: ChannelRowProps) {
  const t = useT();
  const test = useMutation(testChannel);
  const asking = remove.asking(channel.id);
  const failed = test.isError ? errorText(test.error) : test.data && !test.data.ok ? (test.data.error ?? t("notify.channels.testFailed")) : undefined;
  return (
    <li className={`border-b border-line py-3 transition-opacity duration-300 last:border-b-0 ${remove.busy && asking ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <div className="min-w-0 grow basis-full @md:basis-0">
          <p title={channel.label} className="truncate font-medium">{channel.label}</p>
          <p className="flex flex-wrap gap-x-3 font-mono text-xs text-muted [overflow-wrap:anywhere]">
            <span>{t(SETUP[channel.kind].title)}</span>
            {channel.owner ? <span translate="no">{t("notify.channels.owner", { id: channel.owner })}</span> : null}
            {channel.secret_hint ? <span translate="no">••••{channel.secret_hint}</span> : null}
          </p>
        </div>
        <Bubble tone={TONE[channel.status]} title={channel.error}>
          {t(`notify.status.${channel.status}`)}
        </Bubble>
        {asking ? null : (
          <div className="flex shrink-0 gap-2">
            <button type="button" disabled={test.isPending} className={button({ variant: "secondary", size: "xs" })} onClick={() => test.mutate(channel.id)}>
              {test.isPending ? t("notify.channels.testing") : t("notify.channels.test")}
            </button>
            <button type="button" autoFocus={remove.returning(channel.id)} aria-label={t("notify.channels.removeLabel", { name: channel.label })} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.ask(channel.id)}>
              {t("notify.remove")}
            </button>
          </div>
        )}
      </div>
      {channel.status === "error" && channel.error ? <p className="mt-1.5 text-sm text-amber [overflow-wrap:anywhere]">{channel.error}</p> : null}
      <p aria-live="polite" className="text-sm [overflow-wrap:anywhere] empty:hidden">
        {failed ? <span className="text-amber">{failed}</span> : test.data?.ok ? <span className="text-lime">{t("notify.channels.testSent")}</span> : ""}
      </p>
      {asking ? (
        <InlineConfirm
          label={t("notify.channels.removeLabel", { name: channel.label })}
          question={t("notify.channels.removeAsk", { name: channel.label })}
          confirm={t("notify.remove")}
          working={t("notify.removing")}
          busy={remove.busy}
          error={remove.error}
          onCancel={remove.cancel}
          onConfirm={remove.confirm}
        />
      ) : null}
    </li>
  );
}
