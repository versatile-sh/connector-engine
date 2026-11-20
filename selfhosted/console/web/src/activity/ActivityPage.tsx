import { useQuery } from "@tanstack/react-query";
import { useRef } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { activityQuery, connectorsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { CAPTION, SKELETON, SURFACE, TITLE, button } from "../ui/styles";
import { callOf, filterOf, hrefOf, isNarrowed, namedOf, periodOf, type ActivityFilter } from "./activity";
import { ActivitySettings } from "./ActivitySettings";
import { CallDetail } from "./CallDetail";
import { CallList } from "./CallList";
import { ConnectorTotals } from "./ConnectorTotals";
import { FilterRow } from "./FilterRow";
import { HourlyChart } from "./HourlyChart";

/** Every function call the apps made through the console: filters and the open call in the address, the calls per hour, per connector, the newest calls and, on a click, one call in full that Back closes. */
export function ActivityPage({ endpoint }: { endpoint: string }) {
  const t = useT();
  const { query } = useLocation();
  const filter = filterOf(query);
  const open = callOf(query);
  const pushed = useRef(false);
  const activity = useQuery(activityQuery(filter));
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const show = (next: ActivityFilter) => navigate(hrefOf(next), { replace: true, keepScroll: true });
  const openCall = (id: string) => {
    pushed.current = true;
    navigate(hrefOf(filter, id), { keepScroll: true });
  };
  const closeCall = () => {
    if (!pushed.current) return show(filter);
    pushed.current = false;
    window.history.back();
  };
  const data = activity.data;
  const named = namedOf(connectors.data ?? [], skills.data ?? []);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-10">
        <PageTitle section={t("nav.activity")} title={t("activity.title")} count={data?.total}>
          {t("activity.intro")}
        </PageTitle>
      </header>
      <FilterRow filter={filter} data={data} named={named} onChange={show} />
      {activity.isError ? (
        <Notice className="mb-6" action={<button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => void activity.refetch()}>{t("common.retry")}</button>}>
          {errorText(activity.error)}
        </Notice>
      ) : null}
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem] lg:gap-10">
        <div className="flex min-w-0 flex-col gap-6">
          {data ? <HourlyChart hours={data.hours} /> : <div aria-hidden="true" className={`${SKELETON} h-60`} />}
          <section aria-labelledby="calls-title" className={`${SURFACE} min-w-0 p-4 sm:p-5`}>
            <div className="flex flex-wrap items-baseline justify-between gap-2 px-1">
              <h2 id="calls-title" className={TITLE}>
                {t("activity.calls")}
              </h2>
              {data && data.total > data.calls.length ? <span className={CAPTION}>{t("activity.calls.shown", { shown: data.calls.length, total: data.total })}</span> : null}
            </div>
            {!data ? (
              <ul aria-hidden="true" className="mt-3 flex flex-col gap-2">
                {[0, 1, 2].map((row) => (
                  <li key={row} className={`${SKELETON} h-14`} />
                ))}
              </ul>
            ) : data.calls.length > 0 ? (
              <div className="mt-2">
                <CallList calls={data.calls} named={named} onOpen={openCall} />
              </div>
            ) : isNarrowed(filter) ? (
              <div className="arrive flex flex-col items-start gap-4 px-1 py-8">
                <p className="text-muted">{t("activity.noMatch")}</p>
                <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => show(periodOf(filter))}>
                  {t("activity.clearFilters")}
                </button>
              </div>
            ) : (
              <div className="px-1">
                <EmptyState heading="h3" title={t("activity.empty.title")} body={t("activity.empty.body", { endpoint })} />
              </div>
            )}
          </section>
        </div>
        <aside className="flex flex-col gap-6">
          {data ? <ConnectorTotals counts={data.connectors} named={named} selected={filter.connector} onSelect={(connector) => show({ ...filter, connector })} /> : <div aria-hidden="true" className={`${SKELETON} h-48`} />}
          <ActivitySettings />
        </aside>
      </div>
      {open ? <CallDetail id={open} named={named} onClose={closeCall} /> : null}
    </section>
  );
}
