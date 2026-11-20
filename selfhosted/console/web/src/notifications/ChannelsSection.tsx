import { useState } from "react";
import type { ChannelView } from "../api";
import { useT } from "../i18n/I18nProvider";
import { removeChannel } from "../mutations";
import { Icon } from "../ui/Icon";
import { WELL, button } from "../ui/styles";
import { useKept } from "../ui/useKept";
import { AddChannelDialog } from "./AddChannelDialog";
import { ChannelRow } from "./ChannelRow";
import { Section } from "./Section";
import { useRemove } from "./useRemove";

/** Chat apps: the channels held calls are sent to, each with its status, a test and an inline remove, and the window that adds one. */
export function ChannelsSection({ channels }: { channels: ChannelView[] }) {
  const t = useT();
  const [adding, setAdding] = useState(false);
  const remove = useRemove(removeChannel);
  const drawn = useKept(channels, (channel) => channel.id);
  return (
    <Section
      id="channels-title"
      icon="chat"
      title={t("notify.channels.title")}
      intro={t("notify.channels.intro")}
      action={
        <button type="button" className={button({ variant: "primary", size: "sm" })} onClick={() => setAdding(true)}>
          <Icon name="plus" className="size-4" />
          {t("notify.channels.add")}
        </button>
      }
    >
      {channels.length === 0 ? (
        <p className="text-sm text-muted">{t("notify.channels.empty")}</p>
      ) : (
        <ul className={WELL}>
          {drawn.map(({ row: channel, phase }) => (
            <ChannelRow key={channel.id} phase={phase} channel={channel} remove={remove} />
          ))}
        </ul>
      )}
      {adding ? <AddChannelDialog onClose={() => setAdding(false)} /> : null}
    </Section>
  );
}
