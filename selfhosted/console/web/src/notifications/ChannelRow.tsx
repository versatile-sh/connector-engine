import { useMutation } from "@tanstack/react-query";
import { errorText, type ChannelKindView, type ChannelView } from "../api";
import { useT } from "../i18n/I18nProvider";
import { testChannel } from "../mutations";
import { InlineConfirm } from "../ui/InlineConfirm";
import { button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { setupsOf, textOf } from "./setup";
import { rowMotion, type RowPhase } from "../ui/kept";
import type { useRemove } from "./useRemove";

const TONE = { connected: "lime", connecting: "muted", error: "amber", notify_only: "muted" } as const;

type ChannelRowProps = { channel: ChannelView; kinds: readonly ChannelKindView[]; remove: ReturnType<typeof useRemove>; phase: RowPhase };

/** One channel: its label and kind, its status, the owner id and the last characters of its secret, a Test message, and an inline Remove. */
export function ChannelRow({ channel, kinds, remove, phase }: ChannelRowProps) {
  const t = useT();
  const setup = setupsOf(kinds).find((s) => s.kind === channel.kind);
  const test = useMutation(testChannel);
  const asking = remove.asking(channel.id);
  const failed = test.isError ? errorText(test.error) : test.data && !test.data.ok ? (test.data.error ?? t("notify.channels.testFailed")) : undefined;
  return (
    <li {...rowMotion(phase)} className={`border-b border-line px-4 py-3 transition-opacity duration-250 last:border-b-0 sm:px-5 ${remove.busy && asking ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <div className="min-w-0 grow basis-full @md:basis-0">
          <p title={channel.label} className="truncate font-medium">{channel.label}</p>
          <p className="flex flex-wrap gap-x-3 font-mono text-xs text-muted [overflow-wrap:anywhere]">
            <span>{setup ? textOf(t, setup.titleKey, setup.title) : channel.kind}</span>
            {channel.owner ? <span translate="no">{t("notify.channels.owner", { id: channel.owner })}</span> : null}
            {channel.secret_hint ? <span translate="no">••••{channel.secret_hint}</span> : null}
          </p>
        </div>
        <Tag tone={TONE[channel.status]} dot title={channel.error}>
          {t(`notify.status.${channel.status}`)}
        </Tag>
        {asking ? null : (
          <div className="flex shrink-0 gap-2">
            <button type="button" disabled={test.isPending} className={button({ variant: "secondary", size: "xs" })} onClick={() => test.mutate(channel.id)}>
              {test.isPending ? t("notify.channels.testing") : t("notify.channels.test")}
            </button>
            <button type="button" autoFocus={remove.returning(channel.id)} aria-label={t("notify.channels.removeLabel", { name: channel.label })} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.ask(channel.id)}>
              {t("common.remove")}
            </button>
          </div>
        )}
      </div>
      {channel.status === "error" && channel.error ? <p className="mt-1.5 text-sm text-amber-400">{channel.error}</p> : null}
      <p aria-live="polite" className="text-sm empty:hidden">
        {failed ? <span className="text-amber-400">{failed}</span> : test.data?.ok ? <span className="text-lime">{t("notify.channels.testSent")}</span> : ""}
      </p>
      {asking ? (
        <InlineConfirm
          label={t("notify.channels.removeLabel", { name: channel.label })}
          question={t("notify.channels.removeAsk", { name: channel.label })}
          confirm={t("common.remove")}
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
