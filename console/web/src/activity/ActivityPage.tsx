import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type CallOutcome } from "../api";
import { useT } from "../i18n/I18nProvider";
import { activityQuery, connectorsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { Marked } from "../ui/Marked";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { Segmented } from "../ui/Segmented";
import { CAPTION, CHIP, SKELETON, SURFACE, button } from "../ui/styles";
import { filterOf, hrefOf, namedOf, RANGES, type ActivityFilter } from "./activity";
import { ActivitySettings } from "./ActivitySettings";
import { CallDetail } from "./CallDetail";
import { CallList } from "./CallList";
import { ConnectorTotals } from "./ConnectorTotals";
import { HourlyChart } from "./HourlyChart";

const FILTER_OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending"];

const CHIP_SIZE = `${CHIP} h-8 px-3 text-xs`;

function Chips<T extends string>({ label, all, options, value, onChange }: { label: string; all: string; options: readonly { value: T; text: string }[]; value: T | undefined; onChange(value: T | undefined): void }) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap items-center gap-1.5">
      <span className={`${CAPTION} mr-1`}>{label}</span>
      <button type="button" aria-pressed={value === undefined} className={CHIP_SIZE} onClick={() => onChange(undefined)}>
        {all}
      </button>
      {options.map((option) => (
        <button key={option.value} type="button" aria-pressed={value === option.value} className={CHIP_SIZE} onClick={() => onChange(value === option.value ? undefined : option.value)}>
          {option.text}
        </button>
      ))}
    </div>
  );
}

/** Every function call the apps made through the console: filters in the address, the calls per hour, per connector, the newest calls and, on a click, one call in full. */
export function ActivityPage({ endpoint }: { endpoint: string }) {
  const t = useT();
  const { query } = useLocation();
  const filter = filterOf(query);
  const activity = useQuery(activityQuery(filter));
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const [open, setOpen] = useState<string>();
  const change = (next: Partial<ActivityFilter>) => {
    const merged: ActivityFilter = { ...filter, ...next };
    navigate(hrefOf(merged), { replace: true, keepScroll: true });
  };
  const data = activity.data;
  const named = namedOf(connectors.data ?? [], skills.data ?? []);
  const filtered = Boolean(filter.connector || filter.agent || filter.outcome);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8">
        <PageTitle section={t("nav.activity")} title={t("activity.title")}>
          {data ? <Bubble tone={data.total > 0 ? "accent" : "neutral"}>{data.total}</Bubble> : null}
        </PageTitle>
        <p className="mt-3 max-w-2xl text-muted">{t("activity.intro")}</p>
      </header>
      <div className="mb-6 flex flex-col gap-3">
        <Segmented
          label={t("activity.range")}
          size="sm"
          className="self-start"
          value={filter.range}
          onChange={(range) => change({ range })}
          options={RANGES.map((range) => ({ value: range.value, text: t(`activity.range.${range.value}`) }))}
        />
        <Chips<CallOutcome>
          label={t("activity.outcome")}
          all={t("activity.outcome.all")}
          value={filter.outcome}
          onChange={(outcome) => change({ outcome })}
          options={FILTER_OUTCOMES.map((outcome) => ({ value: outcome, text: t(`outcome.${outcome}`) }))}
        />
        {data && (data.agents.length > 1 || filter.agent) ? (
          <Chips<string> label={t("activity.app")} all={t("activity.app.all")} value={filter.agent} onChange={(agent) => change({ agent })} options={data.agents.map((agent) => ({ value: agent, text: agent }))} />
        ) : null}
      </div>
      {activity.isError ? (
        <div className="mb-6">
          <Notice action={<button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => void activity.refetch()}>{t("common.retry")}</button>}>
            {errorText(activity.error)}
          </Notice>
        </div>
      ) : null}
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="flex min-w-0 flex-col gap-6">
          {data ? <HourlyChart hours={data.hours} range={filter.range} /> : <div aria-hidden="true" className={`${SKELETON} h-60`} />}
          <section aria-labelledby="calls-title" className={`${SURFACE} min-w-0 p-4 sm:p-5`}>
            <div className="flex flex-wrap items-baseline justify-between gap-2 px-1">
              <h2 id="calls-title" className="text-lg leading-snug font-medium">
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
                <CallList calls={data.calls} named={named} onOpen={setOpen} />
              </div>
            ) : filtered ? (
              <div className="arrive flex flex-col items-center px-4 py-10 text-center">
                <p className="text-muted">{t("activity.noMatch")}</p>
                <button type="button" className={`${button({ variant: "secondary", size: "sm" })} mt-4`} onClick={() => navigate(hrefOf({ range: filter.range }), { replace: true, keepScroll: true })}>
                  {t("activity.clearFilters")}
                </button>
              </div>
            ) : (
              <div className="arrive flex flex-col items-center px-4 py-12 text-center">
                <span aria-hidden="true" className="grid size-14 place-items-center rounded-full border border-line-strong text-muted">
                  <Icon name="activity" className="size-6" />
                </span>
                <h3 className="serif-room mt-4 font-serif text-3xl leading-none font-normal tracking-[-0.02em]"><Marked text={t("activity.empty.title")} /></h3>
                <p className="mt-2 max-w-md text-muted">{t("activity.empty.body", { endpoint })}</p>
              </div>
            )}
          </section>
        </div>
        <aside className="flex flex-col gap-6">
          {data ? <ConnectorTotals counts={data.connectors} named={named} selected={filter.connector} onSelect={(connector) => change({ connector })} /> : <div aria-hidden="true" className={`${SKELETON} h-48`} />}
          <ActivitySettings />
        </aside>
      </div>
      {open ? <CallDetail id={open} named={named} onClose={() => setOpen(undefined)} /> : null}
    </section>
  );
}
