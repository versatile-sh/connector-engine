import { useQuery } from "@tanstack/react-query";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { notificationsQuery } from "../queries";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { SKELETON } from "../ui/styles";
import { ChannelsSection } from "./ChannelsSection";
import { DevicesSection } from "./DevicesSection";
import { ElicitSection } from "./ElicitSection";

/** Where held calls reach the owner besides this console: in the AI app itself, as desktop popups, and in chat apps. Nothing is sent until one is set up. */
export function NotificationsPage() {
  const t = useT();
  const data = useQuery(notificationsQuery);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8">
        <PageTitle section={t("nav.notifications")} title={t("notify.title")} />
        <p className="mt-3 max-w-2xl text-muted">{t("notify.intro")}</p>
        <p className="mt-2 max-w-2xl text-sm text-muted">{t("notify.privacy")}</p>
      </header>
      {data.isError ? <Notice>{errorText(data.error)}</Notice> : null}
      {data.isPending ? (
        <div aria-hidden="true" className="flex flex-col gap-6">
          {[0, 1, 2].map((row) => (
            <div key={row} className={`${SKELETON} h-40`} />
          ))}
        </div>
      ) : data.data ? (
        <div className="flex flex-col gap-6">
          <ElicitSection apps={data.data.apps} />
          <DevicesSection devices={data.data.devices} />
          <ChannelsSection channels={data.data.channels} />
        </div>
      ) : null}
    </section>
  );
}
