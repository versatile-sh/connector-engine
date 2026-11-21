import { useQuery } from "@tanstack/react-query";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { notificationsQuery } from "../queries";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { PAGE, SKELETON } from "../ui/styles";
import { ChannelsSection } from "./ChannelsSection";
import { DevicesSection } from "./DevicesSection";

/** Where held calls reach the owner besides this console: as desktop popups and in chat apps. Nothing is sent until one is set up. */
export function NotificationsPage() {
  const t = useT();
  const data = useQuery(notificationsQuery);
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("notify.title")}>
        <p>{t("notify.intro")}</p>
        <p className="mt-2 text-sm">{t("notify.privacy")}</p>
      </PageHeader>
      {data.isError ? <Notice>{errorText(data.error)}</Notice> : null}
      {data.isPending ? (
        <div aria-hidden="true" className="grid gap-6 lg:grid-cols-2">
          {[0, 1].map((row) => (
            <div key={row} className={`${SKELETON} h-40`} />
          ))}
        </div>
      ) : data.data ? (
        <div className="@container">
          <div className="grid gap-6 @5xl:grid-cols-2">
            <DevicesSection devices={data.data.devices} />
            <ChannelsSection channels={data.data.channels} />
          </div>
        </div>
      ) : null}
    </section>
  );
}
