import type { ChannelView, NotificationsData } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { kindTitle } from "../notifications/setup";
import { openLink } from "../router";
import { Arrow, Icon, type IconName } from "../ui/Icon";
import { CAPTION, QUIET_LINK, SKELETON_LINE, SURFACE, TITLE } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useNow } from "../ui/useNow";
import { reachOf, relativeOf } from "./recent";

const SHOWN = 4;

const STATUS: Record<ChannelView["status"], { tone: "lime" | "amber" | "muted"; dot: boolean }> = {
  connected: { tone: "lime", dot: true },
  connecting: { tone: "muted", dot: false },
  error: { tone: "amber", dot: true },
  notify_only: { tone: "muted", dot: false },
};

type RowProps = { icon: IconName; label: string; note: string; tag: { tone: "lime" | "amber" | "muted"; dot: boolean; text: string } | undefined };

function Row({ icon, label, note, tag }: RowProps) {
  return (
    <li className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 py-2.5">
      <span aria-hidden="true" className="grid size-8 place-items-center rounded-full border border-line-strong text-muted">
        <Icon name={icon} className="size-4" />
      </span>
      <span className="min-w-0">
        <span className="block truncate text-sm text-paper">{label}</span>
        <span className={`${CAPTION} block truncate`}>{note}</span>
      </span>
      {tag ? (
        <Tag tone={tag.tone} dot={tag.dot}>
          {tag.text}
        </Tag>
      ) : null}
    </li>
  );
}

/** Where a held call reaches the viewer: this console, always, the AI app when an app is set to ask there, then the computers paired for popups and the chat channels with their status, the first few and a count of the rest, and the way to Notifications; with nothing but the console, a line on how to add more. */
export function ReachYou({ data, failed }: { data: NotificationsData | undefined; failed: boolean }) {
  const t = useT();
  const locale = useLocale();
  const reach = data ? reachOf(data) : undefined;
  const now = useNow();
  const rows = reach ? [...reach.devices.map((device) => ({ kind: "device" as const, device })), ...reach.channels.map((channel) => ({ kind: "channel" as const, channel }))] : [];
  return (
    <section aria-labelledby="reach-title" className={`${SURFACE} min-w-0 p-5`}>
      <h2 id="reach-title" className={TITLE}>
        {t("approvals.reach.title")}
      </h2>
      {!reach && failed ? (
        <p className="mt-2 text-sm text-pretty text-muted">{t("approvals.unavailable")}</p>
      ) : !reach ? (
        <div aria-hidden="true" className="mt-4 flex flex-col gap-3">
          {[0, 1, 2].map((row) => (
            <div key={row} className={`${SKELETON_LINE} h-9 w-full`} />
          ))}
        </div>
      ) : (
        <>
          <ul className="mt-2 flex flex-col divide-y divide-line">
            <Row icon="shield" label={t("approvals.reach.console")} note={t("approvals.reach.console.note")} tag={{ tone: "lime", dot: true, text: t("approvals.reach.on") }} />
            <Row
              icon="chat"
              label={t("approvals.reach.app")}
              note={reach.elicit > 0 ? t("approvals.reach.app.on", { count: reach.elicit }) : t("approvals.reach.app.off")}
              tag={reach.elicit > 0 ? { tone: "lime", dot: true, text: t("approvals.reach.on") } : { tone: "muted", dot: false, text: t("approvals.reach.off") }}
            />
            {rows.slice(0, SHOWN).map((row) =>
              row.kind === "device" ? (
                <Row key={`d:${row.device.id}`} icon="terminal" label={row.device.name} note={row.device.last_seen ? t("approvals.reach.seen", { when: relativeOf(row.device.last_seen, now, locale) }) : t("approvals.reach.never")} tag={{ tone: "muted", dot: false, text: t("approvals.reach.popup") }} />
              ) : (
                <Row key={`c:${row.channel.id}`} icon="bell" label={row.channel.label} note={kindTitle(t, data?.kinds ?? [], row.channel.kind)} tag={{ ...STATUS[row.channel.status], text: t(`notify.status.${row.channel.status}`) }} />
              ),
            )}
          </ul>
          {rows.length > SHOWN ? <p className={`${CAPTION} mt-1`}>{t("approvals.reach.more", { count: rows.length - SHOWN })}</p> : null}
          {reach.extra ? null : <p className="mt-3 text-sm text-pretty text-muted">{t("approvals.reach.empty")}</p>}
          <a href="/notifications" onClick={openLink("/notifications")} className={`${QUIET_LINK} mt-4 inline-flex items-center gap-1.5 text-sm text-muted`}>
            {t("approvals.reach.manage")}
            <Arrow />
          </a>
        </>
      )}
    </section>
  );
}
