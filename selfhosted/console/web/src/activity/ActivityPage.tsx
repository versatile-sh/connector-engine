import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useRef } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { useViewer } from "../people/useViewer";
import { offersScope } from "../people/viewer";
import { activityQuery, activitySettingsQuery, connectorsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { CAPTION, SKELETON, SURFACE, TITLE, button } from "../ui/styles";
import { callOf, filterOf, hrefOf, inScope, isNarrowed, keptDaysOf, namedOf, clearedOf, type ActivityFilter } from "./activity";
import { ActivitySettings } from "./ActivitySettings";
import { CallDetail } from "./CallDetail";
import { CallList } from "./CallList";
import { ConnectorTotals } from "./ConnectorTotals";
import { FilterRow } from "./FilterRow";
import { HourlyChart } from "./HourlyChart";

/** Every function call the apps made through the console: filters and the open call in the address, the calls per hour, per connector, the newest calls and, on a click, one call in full that Back closes. Once a second user exists, a viewer who may see everyone's calls picks Mine or Everyone and one person among them. */
export function ActivityPage({ endpoint }: { endpoint: string }) {
  const t = useT();
  const { query } = useLocation();
  const viewer = useViewer();
  const offered = offersScope(viewer, "activity.any");
  const asked = filterOf(query);
  const filter = offered || !asked.scope ? asked : inScope(asked, "mine");
  const open = callOf(query);
  const pushed = useRef(false);
  const keptDays = keptDaysOf(useQuery(activitySettingsQuery).data?.retention);
  const activity = useInfiniteQuery(activityQuery(filter, keptDays));
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
        <PageTitle section={t("nav.activity")} title={t("activity.title")} count={data?.total} scope={filter.scope ? t("scope.count") : undefined}>
          {t("activity.intro")}
        </PageTitle>
      </header>
      <FilterRow filter={filter} data={data} named={named} scoped={offered} keptDays={keptDays} onChange={show} />
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
              <div aria-busy={activity.isPlaceholderData} className={`mt-2 transition-opacity duration-250 ${activity.isPlaceholderData ? "opacity-60" : ""}`}>
                <CallList calls={data.calls} named={named} owners={filter.scope === "everyone"} scope={`${hrefOf(filter)}#${data.pages}`} onOpen={openCall} />
                {data.more ? (
                  <div className="flex justify-center pt-4">
                    <button type="button" disabled={activity.isFetchingNextPage} className={button({ variant: "secondary", size: "sm" })} onClick={() => void activity.fetchNextPage()}>
                      {t("activity.loadMore")}
                    </button>
                  </div>
                ) : null}
              </div>
            ) : isNarrowed(filter) ? (
              <div className="arrive flex flex-col items-start gap-4 px-1 py-8">
                <p className="text-muted">{t("activity.noMatch")}</p>
                <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => show(clearedOf(filter))}>
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
          {viewer.can("activity.settings") || viewer.can("activity.delete") ? <ActivitySettings /> : null}
        </aside>
      </div>
      {open ? <CallDetail id={open} named={named} onClose={closeCall} /> : null}
    </section>
  );
}
