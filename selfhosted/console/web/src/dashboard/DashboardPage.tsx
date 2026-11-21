import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { hrefOf, namedOf } from "../activity/activity";
import { errorText } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { SetupGuide } from "../onboarding/SetupGuide";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { useViewer } from "../people/useViewer";
import { offersScope, scopedHref, scopeOf } from "../people/viewer";
import { connectorsQuery, dashboardKeysQuery, dashboardQuery, scopedApprovalsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { PAGE, button } from "../ui/styles";
import { Toolbar } from "../ui/Toolbar";
import { CallsChart } from "./CallsChart";
import { activeConnectors, isQuiet, liveKeys } from "./dashboard";
import { DashboardSkeleton } from "./DashboardSkeleton";
import { Kpis } from "./Kpis";
import { Ranking, type RankRow } from "./Ranking";
import { RecentCalls } from "./RecentCalls";

const COLUMNS = "grid gap-6 @4xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]";

const TOP = 5;

/** The first page of the console: the numbers of the last day, a chart of the calls, the busiest connectors and access keys of the week, the newest calls and, until it is done or closed, the first-run guide. It reads the console once for the calls (and again every half minute) and shows its shape only on that first read; a viewer who may see everyone's picks Mine or Everyone in the toolbar and the last answer stays up until the other arrives. With no calls in the week it says so once and offers to connect an app, never a page of zeros. */
export function DashboardPage() {
  const t = useT();
  const viewer = useViewer();
  const { path, query } = useLocation();
  const offered = offersScope(viewer, "activity.any") && viewer.can("keys.any") && viewer.can("approvals.any");
  const scope = scopeOf(query, offered);
  const summary = useQuery(dashboardQuery(scope));
  const approvals = useQuery(scopedApprovalsQuery(scope));
  const keys = useQuery(dashboardKeysQuery(scope));
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const [importing, setImporting] = useState<{ initial?: string }>();
  const manage = viewer.can("connectors.manage");
  const named = namedOf(connectors.data ?? [], skills.data ?? []);
  const data = summary.data;
  const everyone = scope === "everyone";
  const connectorRows: RankRow[] = (data?.connectors ?? []).slice(0, TOP).map((count) => {
    const known = count.connector ? named.get(count.connector) : undefined;
    const title = count.connector ? (known?.title ?? count.alias ?? count.connector) : t("activity.connectors.unknown", { alias: count.alias || "?" });
    return {
      key: count.connector ?? `alias:${count.alias ?? ""}`,
      label: title,
      icon: <ConnectorIcon src={known && !known.skill ? known.icon : undefined} title={title} size="xs" />,
      total: count.total,
      failed: count.failed > 0 ? t("activity.connectors.failed", { count: count.failed }) : undefined,
      calls: t("activity.connectors.calls", { count: count.total }),
      href: count.connector ? hrefOf({ range: "7d", connector: count.connector, ...(everyone ? { scope: "everyone" as const } : {}) }) : undefined,
    };
  });
  const keyRows: RankRow[] = (data?.keys ?? []).slice(0, TOP).map((count) => ({
    key: count.agent,
    label: (
      <span className="font-mono" translate="no">
        {count.agent}
      </span>
    ),
    total: count.total,
    failed: count.failed > 0 ? t("activity.connectors.failed", { count: count.failed }) : undefined,
    calls: t("activity.connectors.calls", { count: count.total }),
    href: hrefOf({ range: "7d", agent: count.agent, ...(everyone ? { scope: "everyone" as const } : {}) }),
  }));
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("dashboard.title")}>{t("dashboard.intro")}</PageHeader>
      {offered ? (
        <Toolbar>
          <ScopeSwitch scope={scope} onChange={(next) => navigate(scopedHref(path, query, next), { replace: true, keepScroll: true })} />
        </Toolbar>
      ) : null}
      {manage ? <SetupGuide onPaste={(initial) => setImporting({ initial })} /> : null}
      {summary.isError ? (
        <Notice
          action={
            <button type="button" className={button({ variant: "secondary", size: "xs" })} onClick={() => void summary.refetch()}>
              {t("common.retry")}
            </button>
          }
        >
          {errorText(summary.error)}
        </Notice>
      ) : null}
      {!data ? (
        summary.isError ? null : (
          <DashboardSkeleton />
        )
      ) : isQuiet(data) ? (
        <EmptyState title={t("dashboard.empty.title")} body={t("dashboard.empty.body")}>
          <button type="button" className={button()} onClick={() => navigate("/access")}>
            {t("dashboard.empty.action")}
          </button>
        </EmptyState>
      ) : (
        <div aria-busy={summary.isPlaceholderData} className={`@container flex flex-col gap-6 transition-opacity duration-250 ${summary.isPlaceholderData ? "opacity-60" : ""}`}>
          <Kpis summary={data} waiting={approvals.data?.length} connectors={connectors.data ? activeConnectors(connectors.data) : undefined} keys={keys.data ? liveKeys(keys.data.agents, Date.parse(data.at)) : undefined} />
          <div className={COLUMNS}>
            <CallsChart summary={data} />
            <Ranking id="top-connectors" title={t("dashboard.topConnectors")} caption={t("dashboard.top.period")} rows={connectorRows} />
          </div>
          <div className={COLUMNS}>
            <RecentCalls calls={data.recent} named={named} owners={everyone} scope={scope} />
            <Ranking id="top-keys" title={t("dashboard.topKeys")} caption={t("dashboard.top.period")} rows={keyRows} />
          </div>
        </div>
      )}
      {importing && manage ? <ImportDialog initial={importing.initial} onClose={() => setImporting(undefined)} /> : null}
    </section>
  );
}
